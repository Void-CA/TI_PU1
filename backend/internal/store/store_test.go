package store

// Integration tests against a real PostgreSQL.
// Run with:
//   TEST_DATABASE_URL=postgres://user:pass@host:port/db?sslmode=disable go test ./internal/store/ -count=1
// Skipped when TEST_DATABASE_URL is not set.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/rules"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// Fresh schema for every test.
	if _, err := s.Pool.Exec(ctx, `
		DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := s.SeedIfEmpty(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return s
}

func createPendingOrder(t *testing.T, s *Store, req domain.Request) int64 {
	t.Helper()
	ctx := context.Background()
	r, err := s.CreateRequest(ctx, req)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	rv, err := s.EvaluateRequest(ctx, r.ID, false)
	if err != nil {
		t.Fatalf("evaluate request: %v", err)
	}
	if rv.Order == nil {
		t.Fatalf("expected work order after on-site evaluation")
	}
	return rv.Order.ID
}

var w = rules.DefaultWeights()
var refs = rules.DefaultRefs()

func TestSeedMirrorsSyntheticDataset(t *testing.T) {
	s := newTestStore(t)
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

func TestConfirmRejectsOverlappingSlot(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := OperatingDay

	// Two router orders; both only fit crews with the router skill.
	o1 := createPendingOrder(t, s, domain.Request{
		Customer: "A", ServiceType: "router", Priority: 2,
		LocationX: 3, LocationY: 3,
		WindowStart: day.Add(14 * time.Hour), WindowEnd: day.Add(17 * time.Hour),
		DurationMin: 60,
	})
	o2 := createPendingOrder(t, s, domain.Request{
		Customer: "B", ServiceType: "router", Priority: 2,
		LocationX: 3, LocationY: 3,
		WindowStart: day.Add(14 * time.Hour), WindowEnd: day.Add(17 * time.Hour),
		DurationMin: 60,
	})

	// Explicitly reserve 15:00-16:00 on crew 3 (Gamma) for o1.
	a1, err := s.Confirm(ctx, ConfirmInput{
		OrderID: o1, CrewID: 3, Justification: "test",
		Start: day.Add(15 * time.Hour), End: day.Add(16 * time.Hour),
	}, day, w, refs)
	if err != nil {
		t.Fatalf("first confirm must succeed: %v", err)
	}
	if a1.Status != domain.AssignmentConfirmed {
		t.Fatalf("expected confirmed, got %s", a1.Status)
	}

	// o2 tries the same crew and slot: must be rejected by the DB constraint.
	_, err = s.Confirm(ctx, ConfirmInput{
		OrderID: o2, CrewID: 3, Justification: "test",
		Start: day.Add(15 * time.Hour + 30*time.Minute), End: day.Add(16 * time.Hour + 30*time.Minute),
	}, day, w, refs)
	if !errors.Is(err, ErrSlotTaken) {
		t.Fatalf("expected ErrSlotTaken, got %v", err)
	}

	// The database must still hold exactly one blocking assignment on that slot.
	var n int
	if err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM assignments
		WHERE crew_id = 3 AND status = 'confirmed'
		  AND tstzrange(start_at, end_at) && tstzrange($1::timestamptz, $2::timestamptz)`,
		day.Add(15*time.Hour), day.Add(16*time.Hour)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 assignment in the slot, got %d", n)
	}
}

func TestConcurrentConfirmExactlyOneWins(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for run := 1; run <= 2; run++ {
		res, err := s.DemoConcurrency(ctx, 3, OperatingDay, 3)
		if err != nil {
			t.Fatalf("demo run %d: %v", run, err)
		}
		if res.Succeeded != 1 {
			t.Fatalf("demo run %d: expected exactly 1 success, got %d (rejected %d, errors %v)",
				run, res.Succeeded, res.Rejected, res.Errors)
		}
		if res.Rejected != res.Attempts-1 {
			t.Fatalf("demo run %d: expected %d rejections, got %d", run, res.Attempts-1, res.Rejected)
		}
	}

	// After two demo runs (each cleaning up), crew 3 must have no demo leftovers
	// except exactly the successful assignment of the last run.
	var n int
	if err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM assignments WHERE justification = 'concurrency demo'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 demo assignment after last run, got %d", n)
	}
}

func TestReassignPreservesOldAssignmentWhenNewSlotImpossible(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := OperatingDay

	// Fiber order, assign to crew 1 (Alpha).
	orderID := createPendingOrder(t, s, domain.Request{
		Customer: "C", ServiceType: "fiber", Priority: 2,
		LocationX: 2, LocationY: 2,
		WindowStart: day.Add(13 * time.Hour), WindowEnd: day.Add(15 * time.Hour),
		DurationMin: 60,
	})
	a, err := s.Confirm(ctx, ConfirmInput{
		OrderID: orderID, CrewID: 1, Justification: "initial",
		Start: day.Add(13 * time.Hour), End: day.Add(14 * time.Hour),
	}, day, w, refs)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Reassign to crew 5 (Epsilon): only knows coax → no feasible slot.
	_, err = s.Reassign(ctx, a.ID, 5, "prueba", day)
	if err == nil {
		t.Fatalf("expected reassignment to Epsilon to fail (missing skill)")
	}

	// The original assignment must be intact (transactional rollback).
	var status domain.AssignmentStatus
	var crewID int64
	if err := s.Pool.QueryRow(ctx,
		`SELECT status, crew_id FROM assignments WHERE id = $1`, a.ID).Scan(&status, &crewID); err != nil {
		t.Fatal(err)
	}
	if status != domain.AssignmentConfirmed || crewID != 1 {
		t.Fatalf("original assignment must be preserved: status=%s crew=%d", status, crewID)
	}

	// Now reassign to crew 2 (Beta, has fiber): must succeed.
	na, err := s.Reassign(ctx, a.ID, 2, "mejor cobertura oriente", day)
	if err != nil {
		t.Fatalf("reassign to Beta: %v", err)
	}
	if na.CrewID != 2 {
		t.Fatalf("expected crew 2, got %d", na.CrewID)
	}

	// Old assignment is marked replaced and frees the slot.
	if err := s.Pool.QueryRow(ctx,
		`SELECT status FROM assignments WHERE id = $1`, a.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != domain.AssignmentReplaced {
		t.Fatalf("expected old assignment replaced, got %s", status)
	}
}

func TestReassignRequiresJustification(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := OperatingDay

	orderID := createPendingOrder(t, s, domain.Request{
		Customer: "D", ServiceType: "router", Priority: 2,
		LocationX: 2, LocationY: 2,
		WindowStart: day.Add(14 * time.Hour), WindowEnd: day.Add(16 * time.Hour),
		DurationMin: 60,
	})
	a, err := s.Confirm(ctx, ConfirmInput{
		OrderID: orderID, CrewID: 1, Justification: "initial",
		Start: day.Add(14 * time.Hour), End: day.Add(15 * time.Hour),
	}, day, w, refs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reassign(ctx, a.ID, 3, "", day); !errors.Is(err, ErrNoJustification) {
		t.Fatalf("expected ErrNoJustification, got %v", err)
	}
}

func TestCrewCannotAdvanceOrderWithoutAssignment(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := OperatingDay

	orderID := createPendingOrder(t, s, domain.Request{
		Customer: "E", ServiceType: "cabling", Priority: 1,
		LocationX: 3, LocationY: 7,
		WindowStart: day.Add(12 * time.Hour), WindowEnd: day.Add(14 * time.Hour),
		DurationMin: 60,
	})
	// Assign to crew 3 (Gamma, the only one with cabling).
	if _, err := s.Confirm(ctx, ConfirmInput{
		OrderID: orderID, CrewID: 3, Justification: "initial",
		Start: day.Add(12 * time.Hour), End: day.Add(13 * time.Hour),
	}, day, w, refs); err != nil {
		t.Fatal(err)
	}

	// Crew 5 tries to start the order: rejected (no active assignment).
	crew5 := int64(5)
	if _, err := s.UpdateOrderStatus(ctx, orderID, domain.OrderInProgress, &crew5); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for foreign crew, got %v", err)
	}

	// Crew 3 starts and completes it: allowed, assignment mirrors the status.
	crew3 := int64(3)
	if _, err := s.UpdateOrderStatus(ctx, orderID, domain.OrderInProgress, &crew3); err != nil {
		t.Fatalf("crew 3 start: %v", err)
	}
	if _, err := s.UpdateOrderStatus(ctx, orderID, domain.OrderCompleted, &crew3); err != nil {
		t.Fatalf("crew 3 complete: %v", err)
	}
	var astatus domain.AssignmentStatus
	if err := s.Pool.QueryRow(ctx,
		`SELECT status FROM assignments WHERE order_id = $1`, orderID).Scan(&astatus); err != nil {
		t.Fatal(err)
	}
	if astatus != domain.AssignmentCompleted {
		t.Fatalf("assignment should mirror completed, got %s", astatus)
	}
}

func TestPlanDayAssignsWhatIsFeasible(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := OperatingDay

	results, err := s.PlanDay(ctx, day, w, refs)
	if err != nil {
		t.Fatalf("plan day: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected pending orders to plan")
	}
	assigned := 0
	for _, r := range results {
		if r.Assigned {
			assigned++
			if r.CrewID == 0 || r.Start == nil {
				t.Fatalf("assigned result missing crew/slot: %+v", r)
			}
		} else if r.Reason == "" {
			t.Fatalf("unassigned result must carry a reason: %+v", r)
		}
	}
	if assigned == 0 {
		t.Fatalf("expected at least one planned assignment")
	}

	// A second planning run must not double-assign (all remaining are unfeasible or none pending).
	results2, err := s.PlanDay(ctx, day, w, refs)
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	for _, r := range results2 {
		if r.Assigned {
			t.Fatalf("second run should not assign pending leftovers blindly: %+v", r)
		}
	}
}
