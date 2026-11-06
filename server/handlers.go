package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ownerHeader   = "X-Device-ID"
	maxTitleChars = 200
)

type eventInput struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"` // RFC 3339, e.g. 2026-11-06T09:30:00-08:00
}

func (in *eventInput) validate() error {
	in.Title = strings.TrimSpace(in.Title)
	switch {
	case in.Title == "":
		return errors.New("title is required")
	case utf8.RuneCountInString(in.Title) > maxTitleChars:
		return errors.New("title must be at most 200 characters")
	case in.StartsAt.IsZero():
		return errors.New("starts_at is required")
	}
	return nil
}

func NewRouter(store *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /events", withOwner(func(w http.ResponseWriter, r *http.Request, owner string) {
		events, err := store.List(r.Context(), owner)
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, events)
	}))
	mux.HandleFunc("POST /events", withOwner(func(w http.ResponseWriter, r *http.Request, owner string) {
		in, ok := decodeInput(w, r)
		if !ok {
			return
		}
		e, err := store.Create(r.Context(), owner, in.Title, in.StartsAt)
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, e)
	}))
	mux.HandleFunc("PUT /events/{id}", withOwner(func(w http.ResponseWriter, r *http.Request, owner string) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		in, ok := decodeInput(w, r)
		if !ok {
			return
		}
		e, err := store.Update(r.Context(), owner, id, in.Title, in.StartsAt)
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		} else if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, e)
	}))
	mux.HandleFunc("DELETE /events/{id}", withOwner(func(w http.ResponseWriter, r *http.Request, owner string) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		err := store.Delete(r.Context(), owner, id)
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		} else if err != nil {
			serverError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	return mux
}

// withOwner requires the device ID header that scopes all data access.
// This is a placeholder for real authentication (see README roadmap).
func withOwner(h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner := strings.TrimSpace(r.Header.Get(ownerHeader))
		if owner == "" || len(owner) > 128 {
			writeError(w, http.StatusUnauthorized, "missing or invalid "+ownerHeader+" header")
			return
		}
		h(w, r, owner)
	}
}

func decodeInput(w http.ResponseWriter, r *http.Request) (eventInput, bool) {
	var in eventInput
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return in, false
	}
	if err := in.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return in, false
	}
	return in, true
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid event id")
		return 0, false
	}
	return id, true
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
