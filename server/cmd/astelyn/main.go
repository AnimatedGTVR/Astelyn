package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/AnimatedGTVR/Astelyn/server/internal/auth"
	"github.com/AnimatedGTVR/Astelyn/server/internal/db"
)

func main() {
	ctx := context.Background()

	pool, err := db.Connect(ctx, env("DATABASE_URL", "postgres://astelyn:astelyn@localhost:5432/astelyn"))
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	authHandler, err := auth.NewHandler(pool)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	authHandler.Register(mux)

	addr := env("ADDR", ":8080")
	log.Printf("astelyn server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, cors(mux)))
}

// cors lets the Flutter web build (served from another origin) call the API.
// Auth uses bearer tokens rather than cookies, so a wildcard origin is safe here.
func cors(next http.Handler) http.Handler {
	origin := env("ALLOWED_ORIGIN", "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
