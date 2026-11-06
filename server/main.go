package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := envOr("ADDR", ":8080")
	if port := os.Getenv("PORT"); port != "" { // set by hosts like Render
		addr = ":" + port
	}
	databaseURL := envOr("DATABASE_URL", "postgres://localhost:5432/daybook?sslmode=disable")

	ctx := context.Background()
	store, err := OpenStore(ctx, databaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()
	cfg := Config{
		DevLogin:  os.Getenv("DEV_LOGIN") == "1",
		PublicURL: envOr("PUBLIC_URL", defaultPublicURL(addr)),
	}
	if ids := splitList(os.Getenv("GOOGLE_CLIENT_IDS")); len(ids) > 0 {
		cfg.Google = NewGoogleVerifier(ctx, ids)
	} else {
		log.Print("GOOGLE_CLIENT_IDS not set: Google sign-in disabled")
	}
	if ids := splitList(os.Getenv("APPLE_BUNDLE_IDS")); len(ids) > 0 {
		cfg.Apple = NewAppleVerifier(ctx, ids)
	} else {
		log.Print("APPLE_BUNDLE_IDS not set: Sign in with Apple disabled")
	}
	if cfg.DevLogin {
		log.Print("WARNING: DEV_LOGIN=1 — anyone can sign in as any email. Never enable in production.")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(NewRouter(store, cfg)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	go func() {
		log.Printf("daybook server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}

// logRequests logs method, path and latency. Invite paths are redacted so
// tokens don't end up in logs.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		path := r.URL.Path
		if strings.HasPrefix(path, "/invite/") {
			path = "/invite/…"
		}
		log.Printf("%s %s %s", r.Method, path, time.Since(start))
	})
}

// defaultPublicURL turns a listen address (":8080" or "127.0.0.1:8080") into
// a URL for local development. Set PUBLIC_URL in production.
func defaultPublicURL(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
