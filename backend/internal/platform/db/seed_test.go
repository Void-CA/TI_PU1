package db_test

import (
	"context"
	"testing"

	"pu1/backend/internal/platform/dbtest"
)

func TestSeedMirrorsSyntheticDataset(t *testing.T) {
	s := dbtest.New(t)
	ctx := context.Background()

	var crews, requests, orders, assignments int
	for _, q := range []struct {
		sql  string
		dest *int
	}{
		{`SELECT COUNT(*) FROM crews`, &crews},
		{`SELECT COUNT(*) FROM service_requests`, &requests},
		{`SELECT COUNT(*) FROM work_orders`, &orders},
		{`SELECT COUNT(*) FROM assignments`, &assignments},
	} {
		if err := s.Pool.QueryRow(ctx, q.sql).Scan(q.dest); err != nil {
			t.Fatal(err)
		}
	}
	if crews != 5 {
		t.Fatalf("expected 5 crews, got %d", crews)
	}
	// 12 dataset orders + 4 maintenance orders (crews with initial busy intervals).
	if orders != 16 {
		t.Fatalf("expected 16 orders, got %d", orders)
	}
	// 4 initial busy intervals (Alpha, Beta, Delta, Epsilon); Gamma starts free.
	if assignments != 4 {
		t.Fatalf("expected 4 initial assignments, got %d", assignments)
	}
	if requests != orders {
		t.Fatalf("every order must have a request")
	}
}
