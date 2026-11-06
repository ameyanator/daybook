package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	srv := httptest.NewServer(NewRouter(store))
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path, device, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if device != "" {
		req.Header.Set(ownerHeader, device)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func listTitles(t *testing.T, srv *httptest.Server, device string) []string {
	t.Helper()
	resp := do(t, srv, "GET", "/events", device, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: status %d", resp.StatusCode)
	}
	var events []Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	titles := make([]string, len(events))
	for i, e := range events {
		titles[i] = e.Title
	}
	return titles
}

func TestEventsAreChronological(t *testing.T) {
	srv := newTestServer(t)
	// Inserted out of order, and with mixed time zones: 09:00 PST == 17:00 UTC.
	for _, body := range []string{
		`{"title":"dinner","starts_at":"2026-11-06T19:00:00Z"}`,
		`{"title":"standup","starts_at":"2026-11-06T09:00:00-08:00"}`,
		`{"title":"gym","starts_at":"2026-11-06T07:00:00Z"}`,
		`{"title":"tomorrow","starts_at":"2026-11-07T06:00:00Z"}`,
	} {
		if resp := do(t, srv, "POST", "/events", "dev-a", body); resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %s: status %d", body, resp.StatusCode)
		}
	}
	got := strings.Join(listTitles(t, srv, "dev-a"), ",")
	if want := "gym,standup,dinner,tomorrow"; got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
}

func TestDevicesAreIsolated(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, "POST", "/events", "dev-a", `{"title":"mine","starts_at":"2026-11-06T09:00:00Z"}`)
	var e Event
	json.NewDecoder(resp.Body).Decode(&e)

	if got := listTitles(t, srv, "dev-b"); len(got) != 0 {
		t.Errorf("dev-b sees %v", got)
	}
	if resp := do(t, srv, "DELETE", "/events/"+itoa(e.ID), "dev-b", ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("dev-b delete: status %d, want 404", resp.StatusCode)
	}
	if resp := do(t, srv, "GET", "/events", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no device: status %d, want 401", resp.StatusCode)
	}
}

func TestUpdateAndDelete(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, "POST", "/events", "d", `{"title":"a","starts_at":"2026-11-06T09:00:00Z"}`)
	var e Event
	json.NewDecoder(resp.Body).Decode(&e)

	if resp := do(t, srv, "PUT", "/events/"+itoa(e.ID), "d", `{"title":"b","starts_at":"2026-11-06T10:00:00Z"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("update: status %d", resp.StatusCode)
	}
	if got := listTitles(t, srv, "d"); len(got) != 1 || got[0] != "b" {
		t.Errorf("after update: %v", got)
	}
	if resp := do(t, srv, "DELETE", "/events/"+itoa(e.ID), "d", ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", resp.StatusCode)
	}
	if got := listTitles(t, srv, "d"); len(got) != 0 {
		t.Errorf("after delete: %v", got)
	}
}

func TestValidation(t *testing.T) {
	srv := newTestServer(t)
	for _, body := range []string{
		`{"title":"   ","starts_at":"2026-11-06T09:00:00Z"}`,
		`{"title":"x"}`,
		`{"title":"x","starts_at":"not a time"}`,
		`{"title":"` + strings.Repeat("a", 201) + `","starts_at":"2026-11-06T09:00:00Z"}`,
		`not json`,
	} {
		if resp := do(t, srv, "POST", "/events", "d", body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %.40s: status %d, want 400", body, resp.StatusCode)
		}
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }
