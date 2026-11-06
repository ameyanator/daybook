package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrInviteInvalid = errors.New("invite link is invalid or has expired")
	ErrSelfShare     = errors.New("you can't share your calendar with yourself")
)

const (
	sessionTTL = 180 * 24 * time.Hour
	inviteTTL  = 7 * 24 * time.Hour
)

// User is a signed-in person. Identity comes from Google or Apple; we never
// store passwords.
type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Event is a single scheduled item. Exactly one of StartsAt (a moment in
// time) or Date (an all-day item, "YYYY-MM-DD" in the user's calendar) is set.
type Event struct {
	ID       int64      `json:"id"`
	Title    string     `json:"title"`
	StartsAt *time.Time `json:"starts_at,omitempty"`
	Date     string     `json:"date,omitempty"`
}

// EventFields is what a user provides when creating or editing an event.
type EventFields struct {
	Title    string
	StartsAt *time.Time
	Date     string
}

// Store keeps everything in Postgres.
type Store struct {
	db  *pgxpool.Pool
	now func() time.Time // replaceable in tests
}

// OpenStore connects to Postgres and applies any pending migrations.
func OpenStore(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := connectDB(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{db: pool, now: time.Now}, nil
}

func (s *Store) Close() { s.db.Close() }

// ---- users & sessions ----

// UpsertUser finds or creates the user for a verified identity, refreshing
// the stored email and (if provided) name.
func (s *Store) UpsertUser(ctx context.Context, id Identity) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		INSERT INTO users (provider, subject, email, name) VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider, subject) DO UPDATE SET
			email = CASE WHEN excluded.email <> '' THEN excluded.email ELSE users.email END,
			name  = CASE WHEN excluded.name  <> '' THEN excluded.name  ELSE users.name  END
		RETURNING id, name, email`,
		id.Provider, id.Subject, strings.ToLower(strings.TrimSpace(id.Email)), strings.TrimSpace(id.Name),
	).Scan(&u.ID, &u.Name, &u.Email)
	return u, err
}

func (s *Store) CreateSession(ctx context.Context, userID int64) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(token), userID, s.now().Add(sessionTTL))
	return token, err
}

func (s *Store) UserForSession(ctx context.Context, token string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT u.id, u.name, u.email FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2`, hashToken(token), s.now(),
	).Scan(&u.ID, &u.Name, &u.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

// DeleteUser removes the account and, via ON DELETE CASCADE, all of its
// events, sessions, shares and invites.
func (s *Store) DeleteUser(ctx context.Context, userID int64) error {
	_, err := s.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

// ---- events ----

const eventColumns = `id, title, starts_at, day`

func scanEvent(row pgx.Row) (Event, error) {
	var e Event
	var startsAt, day *time.Time
	if err := row.Scan(&e.ID, &e.Title, &startsAt, &day); err != nil {
		return e, err
	}
	if startsAt != nil {
		t := startsAt.UTC()
		e.StartsAt = &t
	}
	if day != nil {
		e.Date = day.Format(time.DateOnly)
	}
	return e, nil
}

// dayParam converts "" to NULL for the day column.
func dayParam(date string) any {
	if date == "" {
		return nil
	}
	return date
}

// ListEvents returns a user's events in rough chronological order. All-day
// items sort as midnight UTC of their date; the app re-sorts in local time.
func (s *Store) ListEvents(ctx context.Context, userID int64) ([]Event, error) {
	rows, err := s.db.Query(ctx, `SELECT `+eventColumns+` FROM events WHERE user_id = $1
		ORDER BY coalesce(starts_at, day::timestamp AT TIME ZONE 'UTC'), id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) CreateEvent(ctx context.Context, userID int64, f EventFields) (Event, error) {
	return scanEvent(s.db.QueryRow(ctx, `
		INSERT INTO events (user_id, title, starts_at, day) VALUES ($1, $2, $3, $4)
		RETURNING `+eventColumns, userID, f.Title, f.StartsAt, dayParam(f.Date)))
}

func (s *Store) UpdateEvent(ctx context.Context, userID, id int64, f EventFields) (Event, error) {
	e, err := scanEvent(s.db.QueryRow(ctx, `
		UPDATE events SET title = $1, starts_at = $2, day = $3 WHERE id = $4 AND user_id = $5
		RETURNING `+eventColumns, f.Title, f.StartsAt, dayParam(f.Date), id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

func (s *Store) DeleteEvent(ctx context.Context, userID, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM events WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- sharing ----

// ShareByEmail shares owner's calendar with every account that signed in with
// that email (normally one; possibly a Google and an Apple account).
func (s *Store) ShareByEmail(ctx context.Context, ownerID int64, email string) ([]User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrNotFound
	}
	users, err := s.queryUsers(ctx,
		`SELECT id, name, email FROM users WHERE email = $1 AND id <> $2 ORDER BY id`, email, ownerID)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		var self bool
		err := s.db.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND email = $2)`, ownerID, email).Scan(&self)
		if err != nil {
			return nil, err
		}
		if self {
			return nil, ErrSelfShare
		}
		return nil, ErrNotFound
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO shares (owner_id, viewer_id)
		SELECT $1, id FROM users WHERE email = $2 AND id <> $1
		ON CONFLICT DO NOTHING`, ownerID, email)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Store) RemoveShare(ctx context.Context, ownerID, viewerID int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM shares WHERE owner_id = $1 AND viewer_id = $2`, ownerID, viewerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CanView(ctx context.Context, viewerID, ownerID int64) (bool, error) {
	if viewerID == ownerID {
		return true, nil
	}
	var ok bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM shares WHERE owner_id = $1 AND viewer_id = $2)`, ownerID, viewerID).Scan(&ok)
	return ok, err
}

// SharingWith lists who can see userID's calendar.
func (s *Store) SharingWith(ctx context.Context, userID int64) ([]User, error) {
	return s.queryUsers(ctx, `
		SELECT u.id, u.name, u.email FROM shares s JOIN users u ON u.id = s.viewer_id
		WHERE s.owner_id = $1 ORDER BY s.created_at, u.id`, userID)
}

// SharedWithMe lists whose calendars userID can see.
func (s *Store) SharedWithMe(ctx context.Context, userID int64) ([]User, error) {
	return s.queryUsers(ctx, `
		SELECT u.id, u.name, u.email FROM shares s JOIN users u ON u.id = s.owner_id
		WHERE s.viewer_id = $1 ORDER BY s.created_at, u.id`, userID)
}

func (s *Store) queryUsers(ctx context.Context, query string, args ...any) ([]User, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	users, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		var u User
		return u, row.Scan(&u.ID, &u.Name, &u.Email)
	})
	if users == nil {
		users = []User{} // encode as [] rather than null
	}
	return users, err
}

// ---- invite links ----

// CreateInvite makes a single-use token that, when accepted, shares
// ownerID's calendar with whoever accepts it.
func (s *Store) CreateInvite(ctx context.Context, ownerID int64) (string, time.Time, error) {
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(inviteTTL)
	_, err = s.db.Exec(ctx, `INSERT INTO invites (token_hash, owner_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(token), ownerID, expires)
	return token, expires, err
}

// AcceptInvite consumes the invite and shares its owner's calendar with viewerID.
func (s *Store) AcceptInvite(ctx context.Context, viewerID int64, token string) (User, error) {
	var owner User
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var expires time.Time
		err := tx.QueryRow(ctx, `
			SELECT u.id, u.name, u.email, i.expires_at FROM invites i JOIN users u ON u.id = i.owner_id
			WHERE i.token_hash = $1 FOR UPDATE OF i`, hashToken(token),
		).Scan(&owner.ID, &owner.Name, &owner.Email, &expires)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !expires.After(s.now())) {
			return ErrInviteInvalid
		} else if err != nil {
			return err
		}
		if owner.ID == viewerID {
			return ErrSelfShare // don't burn the link if the owner taps it themselves
		}
		if _, err := tx.Exec(ctx, `DELETE FROM invites WHERE token_hash = $1`, hashToken(token)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO shares (owner_id, viewer_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			owner.ID, viewerID)
		return err
	})
	return owner, err
}

// ---- tokens ----

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Only hashes of tokens are stored, so a leaked database can't be used to
// impersonate users or redeem invites.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
