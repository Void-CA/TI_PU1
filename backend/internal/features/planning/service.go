// Package planning runs the hybrid initial planning: it processes pending
// orders in a deterministic order and coordinates the assignment service to
// reserve each one atomically. No SQL and no HTTP of its own: it orchestrates
// application services (orders + assignments).
package planning

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/engine"
	"pu1/backend/internal/features/assignments"
	"pu1/backend/internal/features/orders"
	"pu1/backend/internal/platform/db"
)

// Service coordinates batch planning.
type Service struct {
	orders      *orders.Repo
	assignments *assignments.Service
	day         time.Time
}

func NewService(or *orders.Repo, as *assignments.Service, day time.Time) *Service {
	return &Service{orders: or, assignments: as, day: day}
}

// SortForPlanning: deterministic order by priority desc, window start asc, id asc.
// This is a planning policy, not part of the shared engine.
func SortForPlanning(list []domain.WorkOrder) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if !a.WindowStart.Equal(b.WindowStart) {
			return a.WindowStart.Before(b.WindowStart)
		}
		return a.ID < b.ID
	})
}

// PlanResult is the per-order outcome of a planning run.
type PlanResult struct {
	OrderID  int64      `json:"order_id"`
	CrewID   int64      `json:"crew_id,omitempty"`
	Crew     string     `json:"crew,omitempty"`
	Start    *time.Time `json:"start,omitempty"`
	End      *time.Time `json:"end,omitempty"`
	Cost     float64    `json:"cost,omitempty"`
	Assigned bool       `json:"assigned"`
	Reason   string     `json:"reason,omitempty"`
	// Retries counts confirmations rejected by a concurrent change (recalculated).
	Retries int `json:"retries,omitempty"`
}

// PlanDay runs the initial planning for the operating day. Candidate crews are
// evaluated in parallel per order; the chosen reservation is confirmed
// atomically. If the reservation is rejected because the schedule changed, the
// best remaining candidate is recalculated (bounded retries).
func (s *Service) PlanDay(ctx context.Context) ([]PlanResult, error) {
	status := domain.OrderPending
	orders, err := s.orders.ListOrders(ctx, &status)
	if err != nil {
		return nil, err
	}
	SortForPlanning(orders)

	var results []PlanResult
	for _, order := range orders {
		res := PlanResult{OrderID: order.ID}
		for attempt := 0; attempt < 3; attempt++ {
			cands, err := s.assignments.Candidates(ctx, order.ID)
			if err != nil {
				return nil, err
			}
			chosen, ok := engine.SelectProposed(cands)
			if !ok {
				res.Reason = "no feasible crew"
				break
			}
			_, err = s.assignments.Confirm(ctx, assignments.ConfirmInput{
				OrderID:       order.ID,
				CrewID:        chosen.CrewID,
				Justification: fmt.Sprintf("batch planning (attempt %d)", attempt+1),
				Start:         chosen.Start,
				End:           chosen.End,
			})
			if errors.Is(err, db.ErrSlotTaken) || errors.Is(err, db.ErrOrderNotPending) {
				res.Retries++
				continue // state changed: recalculate candidates
			}
			if err != nil {
				return nil, err
			}
			res.Assigned = true
			res.CrewID = chosen.CrewID
			res.Crew = chosen.Name
			start, end := chosen.Start, chosen.End
			res.Start, res.End = &start, &end
			res.Cost = chosen.Cost
			res.Reason = ""
			break
		}
		if !res.Assigned && res.Reason == "" {
			res.Reason = "confirmation rejected after retries"
		}
		results = append(results, res)
	}
	return results, nil
}
