// Package engine implements the assignment algorithm as pure functions.
// No database, no HTTP: verifiable with isolated tests.
//
// Decision levels (PLAN.md):
//  1. Feasibility -> mandatory filter (skills, availability, overlap, window)
//  2. Efficiency  -> cost C = w_d·D + w_t·W + w_l·L
//  3. Equity      -> load component L of the cost function
//
// Use-case policies (batch ordering, the naive baseline selector) are NOT
// here: they belong to the features that own those decisions.
package engine

import (
	"math"
	"sort"
	"time"

	"pu1/backend/internal/domain"
)

// Weights of the cost function. No weight can override a mandatory constraint.
type Weights struct {
	WD float64 `json:"w_d"`
	WT float64 `json:"w_t"`
	WL float64 `json:"w_l"`
}

// Normalization references (explicit assumptions).
type Refs struct {
	// DistanceRef: diagonal of the coordinate grid (10x10) ≈ 14.14.
	DistanceRef float64
	// WindowRef: reference duration to normalize waiting time (minutes).
	WindowRef float64
	// ShiftRef: shift length in minutes (08:00–17:00 = 540).
	ShiftRef float64
}

func DefaultWeights() Weights { return Weights{WD: 0.5, WT: 0.2, WL: 0.3} }
func DefaultRefs() Refs       { return Refs{DistanceRef: 14.14, WindowRef: 240, ShiftRef: 540} }

// Rejection reason codes (stable for the UI).
const (
	ReasonInactiveCrew    = "crew_inactive"
	ReasonMissingSkill    = "missing_skill"
	ReasonNoFreeSlot      = "no_free_slot_in_window"
	ReasonAlreadyAssigned = "order_already_assigned"
)

// Candidate is the result of evaluating one crew for one order.
type Candidate struct {
	CrewID   int64     `json:"crew_id"`
	Name     string    `json:"name"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Feasible bool      `json:"feasible"`
	Reasons  []string  `json:"reasons,omitempty"`
	// Metrics (only meaningful when feasible).
	Distance  float64 `json:"distance"`
	WaitMin   float64 `json:"wait_min"`
	LoadHours float64 `json:"load_hours"` // hours after assigning this order
	MeanLoad  float64 `json:"mean_load"`  // mean load after assigning (same for all crews)
	Cost      float64 `json:"cost"`
}

// EvaluateCrew applies level 1 (feasibility) and, if it passes, computes the cost.
// busy: intervals the crew already occupies that day. loadBefore/totalLoadBefore in hours.
// The crew's BaseX/BaseY must already be adjusted (base location or last order of the day).
func EvaluateCrew(
	crew domain.Crew,
	order domain.WorkOrder,
	busy []domain.Interval,
	loadBefore float64,
	totalLoadBefore float64,
	nCrews int,
	w Weights,
	r Refs,
	day time.Time,
) Candidate {
	cand := Candidate{
		CrewID:  crew.ID,
		Name:    crew.Name,
		Reasons: []string{},
	}
	if !crew.Active {
		cand.Reasons = append(cand.Reasons, ReasonInactiveCrew)
		return cand
	}
	if !hasSkills(crew, order.Requirements) {
		cand.Reasons = append(cand.Reasons, ReasonMissingSkill)
		return cand
	}
	slot, ok := earliestFreeSlot(crew, order, busy, day)
	if !ok {
		cand.Reasons = append(cand.Reasons, ReasonNoFreeSlot)
		return cand
	}
	cand.Start, cand.End = slot.Start, slot.End
	cand.Feasible = true

	// Levels 2 and 3: efficiency and equity.
	newLoad := loadBefore + float64(order.DurationMin)/60.0
	newTotal := totalLoadBefore + float64(order.DurationMin)/60.0
	meanLoad := newTotal / float64(max(nCrews, 1))

	cand.Distance = math.Hypot(order.LocationX-crew.BaseX, order.LocationY-crew.BaseY)
	cand.WaitMin = math.Max(0, cand.Start.Sub(order.WindowStart).Minutes())
	cand.LoadHours = newLoad
	cand.MeanLoad = meanLoad
	excessLoad := math.Max(0, newLoad-meanLoad)

	cand.Cost = w.WD*(cand.Distance/r.DistanceRef) +
		w.WT*(cand.WaitMin/r.WindowRef) +
		w.WL*(excessLoad/r.ShiftRef)
	return cand
}

func hasSkills(crew domain.Crew, requirements []string) bool {
	if len(requirements) == 0 {
		return true
	}
	set := make(map[string]bool, len(crew.Skills))
	for _, s := range crew.Skills {
		set[s] = true
	}
	for _, req := range requirements {
		if !set[req] {
			return false
		}
	}
	return true
}

// earliestFreeSlot: first start in the intersection of the request window and the
// crew's availability that does not overlap any busy interval.
func earliestFreeSlot(crew domain.Crew, order domain.WorkOrder, busy []domain.Interval, day time.Time) (domain.Interval, bool) {
	availFrom := minuteOfDayToDay(day, crew.AvailableFromMin)
	availTo := minuteOfDayToDay(day, crew.AvailableToMin)
	start := order.WindowStart
	if availFrom.After(start) {
		start = availFrom
	}
	limit := order.WindowEnd
	if availTo.Before(limit) {
		limit = availTo
	}
	dur := time.Duration(order.DurationMin) * time.Minute
	if limit.Sub(start) < dur {
		return domain.Interval{}, false
	}

	sorted := make([]domain.Interval, len(busy))
	copy(sorted, busy)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start.Before(sorted[j].Start) })

	end := start.Add(dur)
	for _, iv := range sorted {
		cand := domain.Interval{Start: start, End: end}
		if cand.Overlaps(iv) {
			start = iv.End
			end = start.Add(dur)
		}
	}
	if end.After(limit) {
		return domain.Interval{}, false
	}
	return domain.Interval{Start: start, End: end}, true
}

// DistanceBetween is the Euclidean distance between two points
// (for the "from last order" variant).
func DistanceBetween(x1, y1, x2, y2 float64) float64 {
	return math.Hypot(x2-x1, y2-y1)
}

func minuteOfDayToDay(day time.Time, mins int) time.Time {
	h := mins / 60
	m := mins % 60
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, time.UTC)
}

// SelectProposed: lowest cost among feasible candidates; tie-break by crew id (determinism).
func SelectProposed(cands []Candidate) (Candidate, bool) {
	var best Candidate
	found := false
	for _, c := range cands {
		if !c.Feasible {
			continue
		}
		if !found || c.Cost < best.Cost ||
			(c.Cost == best.Cost && c.CrewID < best.CrewID) {
			best = c
			found = true
		}
	}
	return best, found
}
