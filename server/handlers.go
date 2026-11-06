package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxTitleChars = 200

type Config struct {
	Google    IdentityVerifier // nil if Google sign-in isn't configured
	Apple     IdentityVerifier // nil if Sign in with Apple isn't configured
	DevLogin  bool             // allow POST /auth/dev — local development only
	PublicURL string           // e.g. https://api.daybook.app, used to build invite links
}

type server struct {
	store *Store
	cfg   Config
}

func NewRouter(store *Store, cfg Config) http.Handler {
	s := &server{store: store, cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	// Sign-in
	mux.HandleFunc("GET /auth/providers", s.providers)
	mux.HandleFunc("POST /auth/google", s.signIn(func() IdentityVerifier { return s.cfg.Google }))
	mux.HandleFunc("POST /auth/apple", s.signIn(func() IdentityVerifier { return s.cfg.Apple }))
	mux.HandleFunc("POST /auth/dev", s.devSignIn)
	mux.HandleFunc("POST /auth/logout", s.authed(s.logout))
	mux.HandleFunc("GET /me", s.authed(func(w http.ResponseWriter, r *http.Request, u User) {
		writeJSON(w, http.StatusOK, u)
	}))
	mux.HandleFunc("DELETE /me", s.authed(s.deleteAccount))

	// My events
	mux.HandleFunc("GET /events", s.authed(s.listMyEvents))
	mux.HandleFunc("POST /events", s.authed(s.createEvent))
	mux.HandleFunc("PUT /events/{id}", s.authed(s.updateEvent))
	mux.HandleFunc("DELETE /events/{id}", s.authed(s.deleteEvent))

	// Sharing (read-only)
	mux.HandleFunc("GET /shares", s.authed(s.listShares))
	mux.HandleFunc("POST /shares", s.authed(s.shareByEmail))
	mux.HandleFunc("DELETE /shares/{userId}", s.authed(s.stopSharing))
	mux.HandleFunc("DELETE /shared-with-me/{userId}", s.authed(s.leaveCalendar))
	mux.HandleFunc("GET /calendars/{userId}/events", s.authed(s.listSharedEvents))
	mux.HandleFunc("POST /invites", s.authed(s.createInvite))
	mux.HandleFunc("POST /invites/accept", s.authed(s.acceptInvite))
	mux.HandleFunc("GET /invite/{token}", s.invitePage)
	return mux
}

// ---- auth ----

func (s *server) authed(h func(http.ResponseWriter, *http.Request, User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		u, err := s.store.UserForSession(r.Context(), token)
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "session expired, please sign in again")
			return
		} else if err != nil {
			serverError(w, err)
			return
		}
		h(w, r, u)
	}
}

func (s *server) providers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"google": s.cfg.Google != nil,
		"apple":  s.cfg.Apple != nil,
		"dev":    s.cfg.DevLogin,
	})
}

type sessionResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

func (s *server) signIn(verifier func() IdentityVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := verifier()
		if v == nil {
			writeError(w, http.StatusServiceUnavailable, "this sign-in method isn't configured on the server")
			return
		}
		var in struct {
			IDToken string `json:"id_token"`
			// Apple only shares the user's name with the app, once, on first
			// sign-in — it isn't in the token — so the app passes it along.
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		id, err := v.Verify(r.Context(), in.IDToken)
		if err != nil {
			log.Printf("sign-in rejected: %v", err)
			writeError(w, http.StatusUnauthorized, "couldn't verify sign-in, please try again")
			return
		}
		if id.Name == "" {
			id.Name = truncate(in.Name, 100)
		}
		s.startSession(w, r, id)
	}
}

// devSignIn lets you sign in as any email without Google/Apple. Only enabled
// when the server runs with DEV_LOGIN=1, for local testing in Expo Go.
func (s *server) devSignIn(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.DevLogin {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var in struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "enter an email address")
		return
	}
	s.startSession(w, r, Identity{Provider: "dev", Subject: email, Email: email, Name: in.Name})
}

func (s *server) startSession(w http.ResponseWriter, r *http.Request, id Identity) {
	u, err := s.store.UpsertUser(r.Context(), id)
	if err != nil {
		serverError(w, err)
		return
	}
	token, err := s.store.CreateSession(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{Token: token, User: u})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request, _ User) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := s.store.DeleteSession(r.Context(), token); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteAccount(w http.ResponseWriter, r *http.Request, u User) {
	if err := s.store.DeleteUser(r.Context(), u.ID); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- events ----

// eventInput is the JSON body for creating/editing an event. Send either
// starts_at (RFC 3339, e.g. 2026-11-06T09:30:00-08:00) for a timed item, or
// date ("2026-11-12") for an all-day item.
type eventInput struct {
	Title    string     `json:"title"`
	StartsAt *time.Time `json:"starts_at"`
	Date     string     `json:"date"`
}

func (in *eventInput) fields() (EventFields, error) {
	f := EventFields{Title: strings.TrimSpace(in.Title), StartsAt: in.StartsAt, Date: strings.TrimSpace(in.Date)}
	switch {
	case f.Title == "":
		return f, errors.New("title is required")
	case utf8.RuneCountInString(f.Title) > maxTitleChars:
		return f, errors.New("title must be at most 200 characters")
	case f.StartsAt == nil && f.Date == "":
		return f, errors.New("either starts_at or date is required")
	case f.StartsAt != nil && f.Date != "":
		return f, errors.New("send starts_at or date, not both")
	case f.StartsAt != nil && f.StartsAt.IsZero():
		return f, errors.New("starts_at is invalid")
	}
	if f.Date != "" {
		if _, err := time.Parse(time.DateOnly, f.Date); err != nil {
			return f, errors.New("date must look like 2026-11-12")
		}
	}
	return f, nil
}

func (s *server) decodeEvent(w http.ResponseWriter, r *http.Request) (EventFields, bool) {
	var in eventInput
	if !decodeJSON(w, r, &in) {
		return EventFields{}, false
	}
	f, err := in.fields()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return f, false
	}
	return f, true
}

func (s *server) listMyEvents(w http.ResponseWriter, r *http.Request, u User) {
	events, err := s.store.ListEvents(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *server) createEvent(w http.ResponseWriter, r *http.Request, u User) {
	in, ok := s.decodeEvent(w, r)
	if !ok {
		return
	}
	e, err := s.store.CreateEvent(r.Context(), u.ID, in)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *server) updateEvent(w http.ResponseWriter, r *http.Request, u User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in, ok := s.decodeEvent(w, r)
	if !ok {
		return
	}
	e, err := s.store.UpdateEvent(r.Context(), u.ID, id, in)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *server) deleteEvent(w http.ResponseWriter, r *http.Request, u User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	err := s.store.DeleteEvent(r.Context(), u.ID, id)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- sharing ----

func (s *server) listShares(w http.ResponseWriter, r *http.Request, u User) {
	sharingWith, err := s.store.SharingWith(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	sharedWithMe, err := s.store.SharedWithMe(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]User{
		"sharing_with":   sharingWith,
		"shared_with_me": sharedWithMe,
	})
}

func (s *server) shareByEmail(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	users, err := s.store.ShareByEmail(r.Context(), u.ID, in.Email)
	switch {
	case errors.Is(err, ErrSelfShare):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound,
			"no one has signed in to Daybook with that email yet. Send them an invite link instead.")
	case err != nil:
		serverError(w, err)
	default:
		writeJSON(w, http.StatusCreated, users)
	}
}

func (s *server) stopSharing(w http.ResponseWriter, r *http.Request, u User) {
	viewerID, ok := pathID(w, r, "userId")
	if !ok {
		return
	}
	s.removeShare(w, r, u.ID, viewerID)
}

func (s *server) leaveCalendar(w http.ResponseWriter, r *http.Request, u User) {
	ownerID, ok := pathID(w, r, "userId")
	if !ok {
		return
	}
	s.removeShare(w, r, ownerID, u.ID)
}

func (s *server) removeShare(w http.ResponseWriter, r *http.Request, ownerID, viewerID int64) {
	err := s.store.RemoveShare(r.Context(), ownerID, viewerID)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not shared")
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listSharedEvents(w http.ResponseWriter, r *http.Request, u User) {
	ownerID, ok := pathID(w, r, "userId")
	if !ok {
		return
	}
	allowed, err := s.store.CanView(r.Context(), u.ID, ownerID)
	if err != nil {
		serverError(w, err)
		return
	}
	if !allowed {
		// 404 rather than 403 so this can't be used to probe which users exist.
		writeError(w, http.StatusNotFound, "calendar not found")
		return
	}
	events, err := s.store.ListEvents(r.Context(), ownerID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *server) createInvite(w http.ResponseWriter, r *http.Request, u User) {
	token, expires, err := s.store.CreateInvite(r.Context(), u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      token,
		"url":        strings.TrimRight(s.cfg.PublicURL, "/") + "/invite/" + token,
		"expires_at": expires.UTC(),
	})
}

func (s *server) acceptInvite(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	owner, err := s.store.AcceptInvite(r.Context(), u.ID, in.Token)
	switch {
	case errors.Is(err, ErrInviteInvalid):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSelfShare):
		writeError(w, http.StatusBadRequest, "this is your own invite link. Send it to someone else.")
	case err != nil:
		serverError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]User{"owner": owner})
	}
}

var (
	tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{20,100}$`)
	inviteHTML   = template.Must(template.New("invite").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Daybook invite</title>
<style>body{font-family:-apple-system,system-ui,sans-serif;background:#FAF8F5;color:#1F1B16;
max-width:28rem;margin:4rem auto;padding:0 1rem;text-align:center}
a.btn{display:inline-block;background:#C2552B;color:#fff;padding:.8rem 1.6rem;border-radius:.5rem;
text-decoration:none;font-weight:600;margin-top:1rem}p{color:#5c554e}</style></head>
<body><h1>You're invited to a Daybook calendar</h1>
<p>Open this link on your phone with Daybook installed. After you sign in, you'll be able to see their schedule.</p>
<a class="btn" href="daybook://invite/{{.}}">Open in Daybook</a>
</body></html>`))
)

// invitePage is what an invite link opens in a browser: a button that hands
// the token to the app. (Later: universal/app links can open the app directly.)
func (s *server) invitePage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !tokenPattern.MatchString(token) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	inviteHTML.Execute(w, token)
}

// ---- helpers ----

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
