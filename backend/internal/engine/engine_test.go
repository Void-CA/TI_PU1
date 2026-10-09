package engine

import (
	"testing"
	"time"

	"pu1/backend/internal/domain"
)

var testDay = time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)

func at(h, m int) time.Time {
	return time.Date(2026, 10, 9, h, m, 0, 0, time.UTC)
}

func baseCrew() domain.Crew {
	return domain.Crew{
		ID: 1, Name: "Alpha", Active: true,
		Skills: []string{"fiber", "router"},
		BaseX:  2, BaseY: 2,
		AvailableFromMin: 8 * 60,
		AvailableToMin:   17 * 60,
	}
}

func baseOrder() domain.WorkOrder {
	return domain.WorkOrder{
		ID: 1, DurationMin: 60,
		Requirements: []string{"fiber"},
		LocationX:    5, LocationY: 5,
		WindowStart: at(9, 0), WindowEnd: at(12, 0),
	}
}

func TestFeasibleRejectsMissingSkill(t *testing.T) {
	crew := baseCrew()
	crew.Skills = []string{"router"}
	cand := EvaluateCrew(crew, baseOrder(), nil, 0, 0, 1, DefaultWeights(), DefaultRefs(), testDay)
	if cand.Feasible {
		t.Fatalf("expected rejection for missing skill")
	}
	if len(cand.Reasons) == 0 || cand.Reasons[0] != ReasonMissingSkill {
		t.Fatalf("expected %s, got %v", ReasonMissingSkill, cand.Reasons)
	}
}

func TestFeasibleRejectsOverlap(t *testing.T) {
	// Crew already busy 09:00-11:00; order 09:00-10:00 with a 2h window that ends at 10:30
	// so the remaining window cannot fit 60 minutes after the busy interval.
	order := baseOrder()
	order.WindowEnd = at(10, 30)
	busy := []domain.Interval{{Start: at(9, 0), End: at(11, 0)}}
	cand := EvaluateCrew(baseCrew(), order, busy, 1, 1, 1, DefaultWeights(), DefaultRefs(), testDay)
	if cand.Feasible {
		t.Fatalf("expected rejection due to no free slot")
	}
	if cand.Reasons[0] != ReasonNoFreeSlot {
		t.Fatalf("expected %s, got %v", ReasonNoFreeSlot, cand.Reasons)
	}
}

func TestFeasibleRejectsOutsideAvailability(t *testing.T) {
	// Window 06:00-07:30 is fully outside availability (08:00-17:00).
	order := baseOrder()
	order.WindowStart = at(6, 0)
	order.WindowEnd = at(7, 30)
	cand := EvaluateCrew(baseCrew(), order, nil, 0, 0, 1, DefaultWeights(), DefaultRefs(), testDay)
	if cand.Feasible {
		t.Fatalf("expected rejection outside availability")
	}
}

func TestEarliestSlotAfterBusyInterval(t *testing.T) {
	// Busy 09:00-10:00; order wants 09:00 with a window until 12:00 -> slot starts at 10:00.
	busy := []domain.Interval{{Start: at(9, 0), End: at(10, 0)}}
	cand := EvaluateCrew(baseCrew(), baseOrder(), busy, 1, 1, 1, DefaultWeights(), DefaultRefs(), testDay)
	if !cand.Feasible {
		t.Fatalf("expected feasible candidate: %v", cand.Reasons)
	}
	if !cand.Start.Equal(at(10, 0)) {
		t.Fatalf("expected start 10:00, got %s", cand.Start)
	}
	if cand.WaitMin != 60 {
		t.Fatalf("expected wait 60 min, got %v", cand.WaitMin)
	}
}

func TestCostPrefersNearerCrew(t *testing.T) {
	near := baseCrew()
	near.ID = 1
	near.BaseX, near.BaseY = 5, 5 // same point as the order: distance 0

	far := baseCrew()
	far.ID = 2
	far.BaseX, far.BaseY = 9, 9 // distance ~5.66

	order := baseOrder()
	w, r := DefaultWeights(), DefaultRefs()
	a := EvaluateCrew(near, order, nil, 0, 0, 2, w, r, testDay)
	b := EvaluateCrew(far, order, nil, 0, 0, 2, w, r, testDay)
	if !a.Feasible || !b.Feasible {
		t.Fatalf("both should be feasible")
	}
	if a.Cost >= b.Cost {
		t.Fatalf("nearer crew should have lower cost: %v vs %v", a.Cost, b.Cost)
	}
}

func TestCostPenalizesOverloading(t *testing.T) {
	// Same distance for both crews; one is already loaded, the other is not.
	order := baseOrder()
	w, r := DefaultWeights(), DefaultRefs()
	loaded := EvaluateCrew(baseCrew(), order, nil, 6, 12, 2, w, r, testDay)
	fresh := EvaluateCrew(baseCrew(), order, nil, 0, 6, 2, w, r, testDay)
	if loaded.Cost <= fresh.Cost {
		t.Fatalf("loaded crew should cost more: %v vs %v", loaded.Cost, fresh.Cost)
	}
}

func TestSelectProposedPicksLowestCostWithDeterministicTieBreak(t *testing.T) {
	cands := []Candidate{
		{CrewID: 5, Feasible: true, Cost: 0.5},
		{CrewID: 2, Feasible: true, Cost: 0.5},
		{CrewID: 9, Feasible: true, Cost: 0.9},
		{CrewID: 1, Feasible: false, Cost: 0.0},
	}
	best, ok := SelectProposed(cands)
	if !ok || best.CrewID != 2 {
		t.Fatalf("expected crew 2 (tie-break by id), got %+v (ok=%v)", best, ok)
	}
}
