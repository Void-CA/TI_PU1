package assignments

// Integration tests against a real PostgreSQL. Run with:
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/features/assignments/ -count=1
//
// Skipped when TEST_DATABASE_URL is not set.

import (
	"context"
	"errors"
	"testing"
	"time"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/engine"
	"pu1/backend/internal/features/requests"
	"pu1/backend/internal/platform/db"
	"pu1/backend/internal/platform/dbtest"
)

func newService(t *testing.T) (*Service, *db.Store) {
	t.Helper()
	s := dbtest.New(t)
	return NewService(s.Pool, db.OperatingDay, engine.DefaultWeights(), engine.DefaultRefs()), s
}

// pendingOrder registers a request and evaluates it on-site, returning the new
// pending work order id.
func pendingOrder(t *testing.T, svc *Service, req domain.Request) int64 {
	t.Helper()
	ctx := context.Background()
	rr := requests.NewRepo(svc.pool)
	created, err := rr.CreateRequest(ctx, req)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	rv, err := rr.EvaluateRequest(ctx, created.ID, false)
	if err != nil {
		t.Fatalf("evaluate request: %v", err)
	}
	if rv.Order == nil {
		t.Fatalf("expected work order after on-site evaluation")
	}
	return rv.Order.ID
}

var w = engine.DefaultWeights()
var refs = engine.DefaultRefs()

func TestConfirmRejectsOverlappingSlot(t *testing.T) {
	svc, store := newService(t)
	ctx := context.Background()
	day := db.OperatingDay

	// Two router orders; both only fit crews with the router skill.
	o1 := pendingOrder(t, svc, domain.Request{
		Customer: "A", ServiceType: "router", Priority: 2,
		LocationX: 3, LocationY: 3,
		WindowStart: day.Add(14 * time.Hour), WindowEnd: day.Add(17 * time.Hour),
		DurationMin: 60,
	})
	o2 := pendingOrder(t, svc, domain.Request{
		Customer: "B", ServiceType: "router", Priority: 2,
		LocationX: 3, LocationY: 3,
		WindowStart: day.Add(14 * time.Hour), WindowEnd: day.Add(17 * time.Hour),
		DurationMin: 60,
	})

	// Explicitly reserve 15:00-16:00 on crew 3 (Gamma) for o1.
	a1, err := svc.Confirm(ctx, ConfirmInput{
		OrderID: o1, CrewID: 3, Justification: "test",
		Start: day.Add(15 * time.Hour), End: day.Add(16 * time.Hour),
	})
	if err != nil {
		t.Fatalf("first confirm must succeed: %v", err)
	}
	if a1.Status != domain.AssignmentConfirmed {
		t.Fatalf("expected confirmed, got %s", a1.Status)
	}

	// o2 tries the same crew and slot: must be rejected by the DB constraint.
	_, err = svc.Confirm(ctx, ConfirmInput{
		OrderID: o2, CrewID: 3, Justification: "test",
		Start: day.Add(15*time.Hour + 30*time.Minute), End: day.Add(16*time.Hour + 30*time.Minute),
	})
	if !errors.Is(err, db.ErrSlotTaken) {
		t.Fatalf("expected ErrSlotTaken, got %v", err)
	}

	// The database must still hold exactly one blocking assignment on that slot.
	var n int
	if err := store.Pool.QueryRow(ctx, `
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
	svc, store := newService(t)
	ctx := context.Background()

	for run := 1; run <= 2; run++ {
		res, err := svc.DemoConcurrency(ctx, 3, 3)
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

	// After two demo runs (each cleaning up), exactly the last success remains.
	var n int
	if err := store.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM assignments WHERE justification = 'concurrency demo'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 demo assignment after last run, got %d", n)
	}
}

func TestReassignPreservesOldAssignmentWhenNewSlotImpossible(t *testing.T) {
	svc, store := newService(t)
	ctx := context.Background()
	day := db.OperatingDay

	// Fiber order, assign to crew 1 (Alpha).
	orderID := pendingOrder(t, svc, domain.Request{
		Customer: "C", ServiceType: "fiber", Priority: 2,
		LocationX: 2, LocationY: 2,
		WindowStart: day.Add(13 * time.Hour), WindowEnd: day.Add(15 * time.Hour),
		DurationMin: 60,
	})
	a, err := svc.Confirm(ctx, ConfirmInput{
		OrderID: orderID, CrewID: 1, Justification: "initial",
		Start: day.Add(13 * time.Hour), End: day.Add(14 * time.Hour),
	})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Reassign to crew 5 (Epsilon): only knows coax → no feasible slot.
	_, err = svc.Reassign(ctx, a.ID, 5, "prueba")
	if err == nil {
		t.Fatalf("expected reassignment to Epsilon to fail (missing skill)")
	}

	// The original assignment must be intact (transactional rollback).
	var status domain.AssignmentStatus
	var crewID int64
	if err := store.Pool.QueryRow(ctx,
		`SELECT status, crew_id FROM assignments WHERE id = $1`, a.ID).Scan(&status, &crewID); err != nil {
		t.Fatal(err)
	}
	if status != domain.AssignmentConfirmed || crewID != 1 {
		t.Fatalf("original assignment must be preserved: status=%s crew=%d", status, crewID)
	}

	// Now reassign to crew 2 (Beta, has fiber): must succeed.
	na, err := svc.Reassign(ctx, a.ID, 2, "mejor cobertura oriente")
	if err != nil {
		t.Fatalf("reassign to Beta: %v", err)
	}
	if na.CrewID != 2 {
		t.Fatalf("expected crew 2, got %d", na.CrewID)
	}

	// Old assignment is marked replaced and frees the slot.
	if err := store.Pool.QueryRow(ctx,
		`SELECT status FROM assignments WHERE id = $1`, a.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != domain.AssignmentReplaced {
		t.Fatalf("expected old assignment replaced, got %s", status)
	}
}

func TestReassignRequiresJustification(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	day := db.OperatingDay

	orderID := pendingOrder(t, svc, domain.Request{
		Customer: "D", ServiceType: "router", Priority: 2,
		LocationX: 2, LocationY: 2,
		WindowStart: day.Add(14 * time.Hour), WindowEnd: day.Add(16 * time.Hour),
		DurationMin: 60,
	})
	a, err := svc.Confirm(ctx, ConfirmInput{
		OrderID: orderID, CrewID: 1, Justification: "initial",
		Start: day.Add(14 * time.Hour), End: day.Add(15 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Reassign(ctx, a.ID, 3, ""); !errors.Is(err, db.ErrNoJustification) {
		t.Fatalf("expected ErrNoJustification, got %v", err)
	}
}

func TestCandidatesOrderedAndLabeled(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	day := db.OperatingDay

	orderID := pendingOrder(t, svc, domain.Request{
		Customer: "F", ServiceType: "fiber", Priority: 2,
		LocationX: 2, LocationY: 2,
		WindowStart: day.Add(13 * time.Hour), WindowEnd: day.Add(16 * time.Hour),
		DurationMin: 60,
	})
	cands, err := svc.Candidates(ctx, orderID)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	if len(cands) != 5 {
		t.Fatalf("expected 5 candidates, got %d", len(cands))
	}
	prev := int64(0)
	for _, c := range cands {
		if c.CrewID <= prev {
			t.Fatalf("candidates must be ordered by crew id: %d after %d", c.CrewID, prev)
		}
		prev = c.CrewID
	}
	// Crew 5 (Epsilon) only knows coax: must be rejected with missing_skill.
	for _, c := range cands {
		if c.CrewID == 5 {
			if c.Feasible {
				t.Fatalf("Epsilon must be infeasible for a fiber order")
			}
			if len(c.Reasons) == 0 || c.Reasons[0] != engine.ReasonMissingSkill {
				t.Fatalf("expected %s, got %v", engine.ReasonMissingSkill, c.Reasons)
			}
		}
	}
}
