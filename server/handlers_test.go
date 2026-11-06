package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeVerifier treats the raw token as a key into a table of identities.
type fakeVerifier map[string]Identity

func (f fakeVerifier) Verify(_ context.Context, raw string) (Identity, error) {
	id, ok := f[raw]
	if !ok {
		return Identity{}, errors.New("bad token")
	}
	return id, nil
}

type testEnv struct {
	t     *testing.T
	srv   *httptest.Server
	store *Store
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	store := testStore(t)
	google := fakeVerifier{
		"g-alice": {Provider: "google", Subject: "1", Email: "Alice@Example.com", Name: "Alice"},
		"g-bob":   {Provider: "google", Subject: "2", Email: "bob@example.com", Name: "Bob"},
		"g-carol": {Provider: "google", Subject: "3", Email: "carol@example.com", Name: "Carol"},
	}
	apple := fakeVerifier{
		"a-bob": {Provider: "apple", Subject: "apple-bob", Email: "bob@example.com"},
	}
	srv := httptest.NewServer(NewRouter(store, Config{Google: google, Apple: apple, PublicURL: "https://api.test"}))
	t.Cleanup(srv.Close)
	return &testEnv{t: t, srv: srv, store: store}
}

func (e *testEnv) do(method, path, token, body string) (int, []byte) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func (e *testEnv) must(want int, method, path, token, body string, out any) {
	e.t.Helper()
	status, b := e.do(method, path, token, body)
	if status != want {
		e.t.Fatalf("%s %s: status %d, want %d; body %s", method, path, status, want, b)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			e.t.Fatalf("%s %s: decode %s: %v", method, path, b, err)
		}
	}
}

func (e *testEnv) signIn(provider, idToken string) (string, User) {
	e.t.Helper()
	var resp sessionResponse
	e.must(200, "POST", "/auth/"+provider, "", `{"id_token":"`+idToken+`"}`, &resp)
	return resp.Token, resp.User
}

func (e *testEnv) titles(path, token string) string {
	e.t.Helper()
	var events []Event
	e.must(200, "GET", path, token, "", &events)
	t := make([]string, len(events))
	for i, ev := range events {
		t[i] = ev.Title
	}
	return strings.Join(t, ",")
}

func (e *testEnv) addEvent(token, title, at string) Event {
	e.t.Helper()
	var ev Event
	e.must(201, "POST", "/events", token, `{"title":"`+title+`","starts_at":"`+at+`"}`, &ev)
	return ev
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

func TestSignIn(t *testing.T) {
	e := newEnv(t)
	if status, _ := e.do("GET", "/events", "", ""); status != 401 {
		t.Errorf("no token: %d, want 401", status)
	}
	if status, _ := e.do("POST", "/auth/google", "", `{"id_token":"forged"}`); status != 401 {
		t.Errorf("bad id token: %d, want 401", status)
	}
	if status, _ := e.do("POST", "/auth/dev", "", `{"email":"x@y.z"}`); status != 404 {
		t.Errorf("dev login while disabled: %d, want 404", status)
	}

	token, u := e.signIn("google", "g-alice")
	if u.Email != "alice@example.com" || u.Name != "Alice" {
		t.Errorf("user = %+v; email should be lowercased", u)
	}
	var me User
	e.must(200, "GET", "/me", token, "", &me)
	if me.ID != u.ID {
		t.Errorf("/me = %+v, want %+v", me, u)
	}
	// Signing in again maps to the same account.
	if _, again := e.signIn("google", "g-alice"); again.ID != u.ID {
		t.Errorf("second sign-in created a new user")
	}
	// Apple's name arrives from the app, not the token.
	var resp sessionResponse
	e.must(200, "POST", "/auth/apple", "", `{"id_token":"a-bob","name":"Bobby"}`, &resp)
	if resp.User.Name != "Bobby" {
		t.Errorf("apple name = %q", resp.User.Name)
	}

	e.must(204, "POST", "/auth/logout", token, "", nil)
	if status, _ := e.do("GET", "/me", token, ""); status != 401 {
		t.Errorf("after logout: %d, want 401", status)
	}
}

func TestEventsChronologicalAndPrivate(t *testing.T) {
	e := newEnv(t)
	alice, _ := e.signIn("google", "g-alice")
	bob, _ := e.signIn("google", "g-bob")
	// Out of order, mixed time zones: 09:00 PST == 17:00 UTC.
	e.addEvent(alice, "dinner", "2026-11-06T19:00:00Z")
	e.addEvent(alice, "standup", "2026-11-06T09:00:00-08:00")
	gym := e.addEvent(alice, "gym", "2026-11-06T07:00:00Z")
	e.addEvent(alice, "tomorrow", "2026-11-07T06:00:00Z")

	if got, want := e.titles("/events", alice), "gym,standup,dinner,tomorrow"; got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
	if got := e.titles("/events", bob); got != "" {
		t.Errorf("bob sees alice's events: %s", got)
	}
	if status, _ := e.do("DELETE", "/events/"+id(gym.ID), bob, ""); status != 404 {
		t.Errorf("bob deleting alice's event: %d, want 404", status)
	}
	e.must(200, "PUT", "/events/"+id(gym.ID), alice, `{"title":"run","starts_at":"2026-11-06T07:00:00Z"}`, nil)
	e.must(204, "DELETE", "/events/"+id(gym.ID), alice, "", nil)
	if got := e.titles("/events", alice); got != "standup,dinner,tomorrow" {
		t.Errorf("after update+delete: %s", got)
	}
}

func TestShareByEmail(t *testing.T) {
	e := newEnv(t)
	alice, a := e.signIn("google", "g-alice")
	bob, b := e.signIn("google", "g-bob")
	carol, _ := e.signIn("google", "g-carol")
	e.addEvent(alice, "lunch", "2026-11-06T20:00:00Z")
	cal := "/calendars/" + id(a.ID) + "/events"

	if status, _ := e.do("GET", cal, bob, ""); status != 404 {
		t.Errorf("before sharing: %d, want 404", status)
	}
	if status, _ := e.do("POST", "/shares", alice, `{"email":"nobody@example.com"}`); status != 404 {
		t.Errorf("unknown email: %d, want 404", status)
	}
	if status, _ := e.do("POST", "/shares", alice, `{"email":"alice@example.com"}`); status != 400 {
		t.Errorf("self share: %d, want 400", status)
	}

	var shared []User
	e.must(201, "POST", "/shares", alice, `{"email":"  BOB@example.com "}`, &shared)
	if len(shared) != 1 || shared[0].ID != b.ID {
		t.Fatalf("shared with %+v", shared)
	}
	e.must(201, "POST", "/shares", alice, `{"email":"bob@example.com"}`, nil) // idempotent

	if got := e.titles(cal, bob); got != "lunch" {
		t.Errorf("bob sees %q", got)
	}
	if status, _ := e.do("GET", cal, carol, ""); status != 404 {
		t.Errorf("carol (not shared): %d, want 404", status)
	}
	// Read-only: bob can't touch alice's events through his own endpoints.
	var aliceEvents []Event
	e.must(200, "GET", cal, bob, "", &aliceEvents)
	if status, _ := e.do("DELETE", "/events/"+id(aliceEvents[0].ID), bob, ""); status != 404 {
		t.Errorf("bob deleting shared event: %d, want 404", status)
	}

	var lists map[string][]User
	e.must(200, "GET", "/shares", alice, "", &lists)
	if len(lists["sharing_with"]) != 1 || len(lists["shared_with_me"]) != 0 {
		t.Errorf("alice lists = %+v", lists)
	}
	e.must(200, "GET", "/shares", bob, "", &lists)
	if len(lists["shared_with_me"]) != 1 || lists["shared_with_me"][0].ID != a.ID {
		t.Errorf("bob lists = %+v", lists)
	}

	// Alice stops sharing.
	e.must(204, "DELETE", "/shares/"+id(b.ID), alice, "", nil)
	if status, _ := e.do("GET", cal, bob, ""); status != 404 {
		t.Errorf("after unshare: %d, want 404", status)
	}
	// Bob can also leave a calendar shared with him.
	e.must(201, "POST", "/shares", alice, `{"email":"bob@example.com"}`, nil)
	e.must(204, "DELETE", "/shared-with-me/"+id(a.ID), bob, "", nil)
	if status, _ := e.do("GET", cal, bob, ""); status != 404 {
		t.Errorf("after leaving: %d, want 404", status)
	}
}

func TestShareByEmailReachesGoogleAndAppleAccounts(t *testing.T) {
	e := newEnv(t)
	alice, _ := e.signIn("google", "g-alice")
	e.signIn("google", "g-bob")
	e.signIn("apple", "a-bob")
	var shared []User
	e.must(201, "POST", "/shares", alice, `{"email":"bob@example.com"}`, &shared)
	if len(shared) != 2 {
		t.Errorf("shared with %d accounts, want 2 (Google + Apple)", len(shared))
	}
}

func TestInviteLinks(t *testing.T) {
	e := newEnv(t)
	alice, a := e.signIn("google", "g-alice")
	bob, _ := e.signIn("google", "g-bob")
	carol, _ := e.signIn("google", "g-carol")
	e.addEvent(alice, "lunch", "2026-11-06T20:00:00Z")

	var inv struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	e.must(201, "POST", "/invites", alice, "", &inv)
	if inv.URL != "https://api.test/invite/"+inv.Token {
		t.Errorf("invite url = %s", inv.URL)
	}
	body := `{"token":"` + inv.Token + `"}`

	// The owner tapping their own link doesn't use it up.
	if status, _ := e.do("POST", "/invites/accept", alice, body); status != 400 {
		t.Errorf("owner accepting own invite: %d, want 400", status)
	}
	var accepted map[string]User
	e.must(200, "POST", "/invites/accept", bob, body, &accepted)
	if accepted["owner"].ID != a.ID {
		t.Errorf("accepted = %+v", accepted)
	}
	if got := e.titles("/calendars/"+id(a.ID)+"/events", bob); got != "lunch" {
		t.Errorf("bob sees %q", got)
	}
	// Single use.
	if status, _ := e.do("POST", "/invites/accept", carol, body); status != 404 {
		t.Errorf("reused invite: %d, want 404", status)
	}

	// Expired.
	e.must(201, "POST", "/invites", alice, "", &inv)
	e.store.now = func() time.Time { return time.Now().Add(inviteTTL + time.Minute) }
	if status, _ := e.do("POST", "/invites/accept", carol, `{"token":"`+inv.Token+`"}`); status != 404 {
		t.Errorf("expired invite: %d, want 404", status)
	}
}

func TestInvitePage(t *testing.T) {
	e := newEnv(t)
	status, b := e.do("GET", "/invite/abcdefghijklmnopqrstuvwxyz_-0123", "", "")
	if status != 200 || !strings.Contains(string(b), `href="daybook://invite/abcdefghijklmnopqrstuvwxyz_-0123"`) {
		t.Errorf("invite page: %d %s", status, b)
	}
	if status, _ := e.do("GET", "/invite/%3Cscript%3E", "", ""); status != 404 {
		t.Errorf("bad token page: %d, want 404", status)
	}
}

func TestDeleteAccount(t *testing.T) {
	e := newEnv(t)
	alice, a := e.signIn("google", "g-alice")
	bob, _ := e.signIn("google", "g-bob")
	e.addEvent(alice, "lunch", "2026-11-06T20:00:00Z")
	e.must(201, "POST", "/shares", alice, `{"email":"bob@example.com"}`, nil)
	e.must(201, "POST", "/shares", bob, `{"email":"alice@example.com"}`, nil)

	e.must(204, "DELETE", "/me", alice, "", nil)

	if status, _ := e.do("GET", "/me", alice, ""); status != 401 {
		t.Errorf("deleted user's session: %d, want 401", status)
	}
	var lists map[string][]User
	e.must(200, "GET", "/shares", bob, "", &lists)
	if len(lists["shared_with_me"]) != 0 || len(lists["sharing_with"]) != 0 {
		t.Errorf("shares survived deletion: %+v", lists)
	}
	var n int
	e.store.db.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE user_id = $1`, a.ID).Scan(&n)
	if n != 0 {
		t.Errorf("%d events survived deletion", n)
	}
	// Signing in again starts a fresh, empty account.
	alice, _ = e.signIn("google", "g-alice")
	if got := e.titles("/events", alice); got != "" {
		t.Errorf("new account has events: %s", got)
	}
}

func TestValidation(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.signIn("google", "g-alice")
	for _, body := range []string{
		`{"title":"   ","starts_at":"2026-11-06T09:00:00Z"}`,
		`{"title":"x"}`,
		`{"title":"x","starts_at":"not a time"}`,
		`{"title":"` + strings.Repeat("a", 201) + `","starts_at":"2026-11-06T09:00:00Z"}`,
		`not json`,
	} {
		if status, _ := e.do("POST", "/events", tok, body); status != 400 {
			t.Errorf("body %.40s: status %d, want 400", body, status)
		}
	}
}

// testStore connects to TEST_DATABASE_URL (default: local daybook_test),
// migrates it and empties every table. Skips if Postgres isn't reachable.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost:5432/daybook_test?sslmode=disable"
	}
	ctx := context.Background()
	store, err := OpenStore(ctx, url)
	if err != nil {
		t.Skipf("no test database (%v); start Postgres or set TEST_DATABASE_URL", err)
	}
	t.Cleanup(store.Close)
	if _, err := store.db.Exec(ctx, `TRUNCATE users, sessions, events, shares, invites RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return store
}

func TestMigrateIsIdempotent(t *testing.T) {
	store := testStore(t)
	if err := migrate(context.Background(), store.db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestDefaultPublicURL(t *testing.T) {
	for addr, want := range map[string]string{":8080": "http://localhost:8080", "127.0.0.1:9000": "http://127.0.0.1:9000"} {
		if got := defaultPublicURL(addr); got != want {
			t.Errorf("defaultPublicURL(%q) = %q, want %q", addr, got, want)
		}
	}
}
