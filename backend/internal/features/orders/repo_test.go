package orders

// Integration test against a real PostgreSQL. Run with:
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/features/orders/ -count=1

import (
	"context"
	"errors"
	"testing"
	"time"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/engine"
	"pu1/backend/internal/features/assignments"
	"pu1/backend/internal/features/requests"
	"pu1/backend/internal/platform/db"
	"pu1/backend/internal/platform/dbtest"
)

func TestCrewCannotAdvanceOrderWithoutAssignment(t *testing.T) {
	store := dbtest.New(t)
	ctx := context.Background()
	day := db.OperatingDay

	// Create a pending order via the requests feature.
	rr := requests.NewRepo(store.Pool)
	created, err := rr.CreateRequest(ctx, domain.Request{
		Customer: "E", ServiceType: "cabling", Priority: 1,
		LocationX: 3, LocationY: 7,
		WindowStart: day.Add(12 * time.Hour), WindowEnd: day.Add(14 * time.Hour),
		DurationMin: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	rv, err := rr.EvaluateRequest(ctx, created.ID, false)
	if err != nil || rv.Order == nil {
		t.Fatalf("evaluate: %v", err)
	}
	orderID := rv.Order.ID

	// Assign to crew 3 (Gamma, the only one with cabling) via the assignments service.
	asvc := assignments.NewService(store.Pool, day, engine.DefaultWeights(), engine.DefaultRefs())
	if _, err := asvc.Confirm(ctx, assignments.ConfirmInput{
		OrderID: orderID, CrewID: 3, Justification: "initial",
		Start: day.Add(12 * time.Hour), End: day.Add(13 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	repo := NewRepo(store.Pool)

	// Crew 5 tries to start the order: rejected (no active assignment).
	crew5 := int64(5)
	if _, err := repo.UpdateStatus(ctx, orderID, domain.OrderInProgress, &crew5); !errors.Is(err, db.ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for foreign crew, got %v", err)
	}

	// Crew 3 starts and completes it: allowed, assignment mirrors the status.
	crew3 := int64(3)
	if _, err := repo.UpdateStatus(ctx, orderID, domain.OrderInProgress, &crew3); err != nil {
		t.Fatalf("crew 3 start: %v", err)
	}
	if _, err := repo.UpdateStatus(ctx, orderID, domain.OrderCompleted, &crew3); err != nil {
		t.Fatalf("crew 3 complete: %v", err)
	}
	var astatus domain.AssignmentStatus
	if err := store.Pool.QueryRow(ctx,
		`SELECT status FROM assignments WHERE order_id = $1`, orderID).Scan(&astatus); err != nil {
		t.Fatal(err)
	}
	if astatus != domain.AssignmentCompleted {
		t.Fatalf("assignment should mirror completed, got %s", astatus)
	}

	// Invalid transition: completed -> pending is not allowed.
	if _, err := repo.UpdateStatus(ctx, orderID, domain.OrderPending, nil); !errors.Is(err, db.ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState for completed->pending, got %v", err)
	}
}

func TestListOrdersFilterByStatus(t *testing.T) {
	store := dbtest.New(t)
	repo := NewRepo(store.Pool)
	ctx := context.Background()

	pending := domain.OrderPending
	orders, err := repo.ListOrders(ctx, &pending)
	if err != nil {
		t.Fatal(err)
	}
	// The seed leaves all 12 dataset orders pending (4 maintenance orders are assigned).
	if len(orders) != 12 {
		t.Fatalf("expected 12 pending orders, got %d", len(orders))
	}
	for _, o := range orders {
		if o.Status != domain.OrderPending {
			t.Fatalf("filter failed: got %s", o.Status)
		}
	}

	all, err := repo.ListOrders(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) <= len(orders) {
		t.Fatalf("unfiltered list should include maintenance orders too")
	}
}
