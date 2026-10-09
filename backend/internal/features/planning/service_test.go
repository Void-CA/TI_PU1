package planning

// Integration test against a real PostgreSQL. Run with:
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/features/planning/ -count=1

import (
	"context"
	"testing"
	"time"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/engine"
	"pu1/backend/internal/features/assignments"
	"pu1/backend/internal/features/orders"
	"pu1/backend/internal/platform/db"
	"pu1/backend/internal/platform/dbtest"
)

func at(h, m int) time.Time {
	return time.Date(2026, 10, 9, h, m, 0, 0, time.UTC)
}

func newService(t *testing.T) (*Service, *db.Store) {
	t.Helper()
	store := dbtest.New(t)
	asvc := assignments.NewService(store.Pool, db.OperatingDay, engine.DefaultWeights(), engine.DefaultRefs())
	osvc := orders.NewRepo(store.Pool)
	return NewService(osvc, asvc, db.OperatingDay), store
}

func TestSortForPlanningDeterministic(t *testing.T) {
	orders := []domain.WorkOrder{
		{ID: 4, Priority: domain.PriorityMedium, WindowStart: at(9, 0)},
		{ID: 5, Priority: domain.PriorityHigh, WindowStart: at(10, 0)},
		{ID: 6, Priority: domain.PriorityHigh, WindowStart: at(10, 0)},
		{ID: 7, Priority: domain.PriorityLow, WindowStart: at(8, 0)},
	}
	SortForPlanning(orders)
	want := []int64{5, 6, 4, 7}
	for i, id := range want {
		if orders[i].ID != id {
			t.Fatalf("position %d: expected order %d, got %d", i, id, orders[i].ID)
		}
	}
}

func TestPlanDayAssignsWhatIsFeasible(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	results, err := svc.PlanDay(ctx)
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
	results2, err := svc.PlanDay(ctx)
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	for _, r := range results2 {
		if r.Assigned {
			t.Fatalf("second run should not assign pending leftovers blindly: %+v", r)
		}
	}
}
