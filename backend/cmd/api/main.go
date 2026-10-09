// Command api runs the assignment prototype HTTP server.
//
// Environment:
//
//	DATABASE_URL  PostgreSQL DSN (default: postgres://pu1:pu1@localhost:5432/pu1?sslmode=disable)
//	HTTP_ADDR     listen address (default: :8080)
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pu1/backend/internal/httpapi"
	"pu1/backend/internal/store"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://pu1:pu1@localhost:5432/pu1?sslmode=disable"
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer s.Close()

	if err := s.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := s.SeedIfEmpty(ctx); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Printf("database ready (operating day %s)", store.OperatingDay.Format("2006-01-02"))

	api := httpapi.NewAPI(s)
	server := &http.Server{
		Addr:              addr,
		Handler:           api.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
