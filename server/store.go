package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

// Event is a single scheduled item: something that happens at a point in time.
type Event struct {
	ID       int64     `json:"id"`
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
}

var ErrNotFound = errors.New("event not found")

// Store persists events in SQLite. Every event belongs to an owner (for now,
// a device ID sent by the app) so different installs never see each other's data.
type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite allows one writer; keep it simple and safe.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			owner     TEXT    NOT NULL,
			title     TEXT    NOT NULL,
			starts_at INTEGER NOT NULL -- unix seconds, UTC
		);
		CREATE INDEX IF NOT EXISTS events_owner_starts ON events(owner, starts_at);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// List returns the owner's events in chronological order.
func (s *Store) List(ctx context.Context, owner string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, starts_at FROM events WHERE owner = ? ORDER BY starts_at, id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&e.ID, &e.Title, &ts); err != nil {
			return nil, err
		}
		e.StartsAt = time.Unix(ts, 0).UTC()
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) Create(ctx context.Context, owner, title string, startsAt time.Time) (Event, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO events (owner, title, starts_at) VALUES (?, ?, ?)`, owner, title, startsAt.Unix())
	if err != nil {
		return Event{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Event{}, err
	}
	return Event{ID: id, Title: title, StartsAt: time.Unix(startsAt.Unix(), 0).UTC()}, nil
}

func (s *Store) Update(ctx context.Context, owner string, id int64, title string, startsAt time.Time) (Event, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE events SET title = ?, starts_at = ? WHERE id = ? AND owner = ?`, title, startsAt.Unix(), id, owner)
	if err != nil {
		return Event{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Event{}, ErrNotFound
	}
	return Event{ID: id, Title: title, StartsAt: time.Unix(startsAt.Unix(), 0).UTC()}, nil
}

func (s *Store) Delete(ctx context.Context, owner string, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ? AND owner = ?`, id, owner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
