// Package dbtest provides a shared setup helper for feature integration tests:
// a private database per test run (so feature packages can run in parallel),
// migrations applied and fixture seed loaded.
package dbtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/platform/db"
)

// New returns a migrated and seeded store backed by a private database created
// from TEST_DATABASE_URL (which must point at a maintenance database the test
// user can create databases in). Skips the test when the variable is unset.
func New(t *testing.T) *db.Store {
	t.Helper()
	baseDSN := os.Getenv("TEST_DATABASE_URL")
	if baseDSN == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer admin.Close()

	dbName := fmt.Sprintf("pu1_t_%d_%d", time.Now().UnixNano(), os.Getpid())
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+dbName); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	testDSN, err := withDatabase(baseDSN, dbName)
	if err != nil {
		t.Fatalf("rewrite dsn: %v", err)
	}

	s, err := db.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
		// FORCE terminates leftover connections (PG 13+).
		if _, err := admin.Exec(ctx, `DROP DATABASE `+dbName+` WITH (FORCE)`); err != nil {
			t.Logf("drop test database: %v", err)
		}
	})

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := s.SeedIfEmpty(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return s
}

// withDatabase replaces the database name in a postgres:// DSN.
func withDatabase(dsn, dbName string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + dbName
	return u.String(), nil
}
