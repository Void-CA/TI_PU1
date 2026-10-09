// Package evaluation runs the base-vs-proposed comparison on the fixed
// fixture dataset. Everything is deterministic: same inputs, same outputs.
//
// This is a use case, not a generic simulation library: it owns the naive
// baseline selector and the comparison report.
package evaluation

import (
	"sort"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/engine"
	"pu1/backend/internal/fixture"
)

// sortOrders mirrors the planning policy (priority desc, window start asc,
// id asc). Duplicated on purpose: the comparison must process orders in the
// same deterministic sequence as the planner, without coupling this feature
// to the planning package.
func sortOrders(list []domain.WorkOrder) {
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

// SelectBase: lowest-id feasible crew (naive comparison method).
// This policy exists only for the evaluation, so it lives here, not in engine.
func SelectBase(cands []engine.Candidate) (engine.Candidate, bool) {
	var best engine.Candidate
	found := false
	for _, c := range cands {
		if !c.Feasible {
			continue
		}
		if !found || c.CrewID < best.CrewID {
			best = c
			found = true
		}
	}
	return best, found
}

// crewState tracks the mutable simulation state of one crew.
type crewState struct {
	crew domain.Crew
	busy []domain.Interval
	load float64 // hours
}

// MethodResult holds the metrics of one assignment method.
type MethodResult struct {
	Method             string             `json:"method"`
	Label              string             `json:"label"` // Spanish label for the UI
	Assigned           int                `json:"assigned"`
	Unassigned         int                `json:"unassigned"`
	ValidPercent       float64            `json:"valid_percent"`
	TotalDistance      float64            `json:"total_distance"`
	TotalWaitMin       float64            `json:"total_wait_min"`
	Conflicts          int                `json:"conflicts"` // must always be 0
	HoursBefore        map[string]float64 `json:"hours_before"`
	HoursAfter         map[string]float64 `json:"hours_after"`
	UnassignedOrderIDs []int64            `json:"unassigned_order_ids"`
	Assignments        []AssignmentSim    `json:"assignments"`
}

// AssignmentSim is one simulated assignment (order -> crew, slot, cost).
type AssignmentSim struct {
	OrderID     int64   `json:"order_id"`
	CrewID      int64   `json:"crew_id"`
	Crew        string  `json:"crew"`
	Start       string  `json:"start"`
	End         string  `json:"end"`
	DurationMin int     `json:"duration_min"`
	Distance    float64 `json:"distance"`
	WaitMin     float64 `json:"wait_min"`
	Cost        float64 `json:"cost"`
}

// Evaluation is the full comparison report.
type Evaluation struct {
	OrdersTotal int            `json:"orders_total"`
	Day         string         `json:"day"`
	Weights     engine.Weights `json:"weights"`
	Refs        engine.Refs    `json:"refs"`
	Base        MethodResult   `json:"base"`
	Proposed    MethodResult   `json:"proposed"`
	Assumptions []string       `json:"assumptions"`
	Crews       []string       `json:"crews"`
}

// Run executes both methods over the same fixed fixture dataset.
// The proposed method evaluates crews in parallel (bounded), but the result is
// deterministic because the selection ties break by crew id.
func Run() Evaluation {
	ds := fixture.Build()
	w := engine.DefaultWeights()
	r := engine.DefaultRefs()

	hoursBefore := map[string]float64{}
	for _, cs := range ds.Crews {
		hoursBefore[cs.Crew.Name] = hoursOf(cs.Busy)
	}

	base := runMethod(ds, "base", "Método base (primera cuadrilla factible)", w, r, hoursBefore)
	proposed := runMethod(ds, "proposed", "Método propuesto (costo D+W+L)", w, r, hoursBefore)

	names := make([]string, 0, len(ds.Crews))
	for _, cs := range ds.Crews {
		names = append(names, cs.Crew.Name)
	}

	return Evaluation{
		OrdersTotal: len(ds.Orders),
		Day:         fixture.Day.Format("2006-01-02"),
		Weights:     w,
		Refs:        r,
		Base:        base,
		Proposed:    proposed,
		Crews:       names,
		Assumptions: []string{
			"Distancia euclidiana sobre malla ficticia 10x10 (aproximación, no tiempo real de viaje).",
			"Punto de partida de una cuadrilla: su base si no tiene órdenes asignadas ese día; si las tiene, la ubicación de su última orden.",
			"Heurística voraz secuencial por prioridad: no garantiza el óptimo global de la jornada.",
			"Datos sintéticos: demuestran comportamiento bajo estos supuestos, no mejoras reales de una empresa.",
		},
	}
}

func runMethod(ds fixture.Dataset, key, label string, w engine.Weights, r engine.Refs, hoursBefore map[string]float64) MethodResult {
	// Deterministic processing order for both methods.
	orders := make([]domain.WorkOrder, len(ds.Orders))
	copy(orders, ds.Orders)
	sortOrders(orders)

	states := make([]*crewState, len(ds.Crews))
	for i, cs := range ds.Crews {
		busy := make([]domain.Interval, len(cs.Busy))
		copy(busy, cs.Busy)
		states[i] = &crewState{crew: cs.Crew, busy: busy, load: hoursBefore[cs.Crew.Name]}
	}

	res := MethodResult{
		Method:             key,
		Label:              label,
		HoursBefore:        hoursBefore,
		HoursAfter:         map[string]float64{},
		UnassignedOrderIDs: []int64{},
		Assignments:        []AssignmentSim{},
	}

	for _, st := range states {
		res.HoursAfter[st.crew.Name] = st.load
	}

	totalLoad := 0.0
	for _, h := range hoursBefore {
		totalLoad += h
	}

	for _, order := range orders {
		cands := make([]engine.Candidate, len(states))

		// Parallel candidate evaluation (bounded by the number of crews).
		done := make(chan int)
		for i, st := range states {
			go func(i int, st *crewState) {
				cands[i] = engine.EvaluateCrew(st.crew, order, st.busy, st.load, totalLoad, len(states), w, r, fixture.Day)
				done <- i
			}(i, st)
		}
		for range states {
			<-done
		}

		var chosen engine.Candidate
		var ok bool
		if key == "base" {
			chosen, ok = SelectBase(cands)
		} else {
			chosen, ok = engine.SelectProposed(cands)
		}
		if !ok {
			res.Unassigned++
			res.UnassignedOrderIDs = append(res.UnassignedOrderIDs, order.ID)
			continue
		}

		// Safety check: a chosen candidate must never overlap an existing interval.
		iv := domain.Interval{Start: chosen.Start, End: chosen.End}
		for _, st := range states {
			if st.crew.ID != chosen.CrewID {
				continue
			}
			for _, busy := range st.busy {
				if iv.Overlaps(busy) {
					res.Conflicts++
				}
			}
			st.busy = append(st.busy, iv)
			st.load = chosen.LoadHours
			// The crew's next starting point is this order's location.
			st.crew.BaseX, st.crew.BaseY = order.LocationX, order.LocationY
		}
		totalLoad += float64(order.DurationMin) / 60.0

		res.Assigned++
		res.TotalDistance += chosen.Distance
		res.TotalWaitMin += chosen.WaitMin
		res.Assignments = append(res.Assignments, AssignmentSim{
			OrderID:     order.ID,
			CrewID:      chosen.CrewID,
			Crew:        chosen.Name,
			Start:       chosen.Start.Format("15:04"),
			End:         chosen.End.Format("15:04"),
			DurationMin: order.DurationMin,
			Distance:    chosen.Distance,
			WaitMin:     chosen.WaitMin,
			Cost:        chosen.Cost,
		})
	}

	for _, st := range states {
		res.HoursAfter[st.crew.Name] = st.load
	}
	total := len(ds.Orders)
	if total > 0 {
		res.ValidPercent = float64(res.Assigned) * 100 / float64(total)
	}
	return res
}

func hoursOf(ivs []domain.Interval) float64 {
	h := 0.0
	for _, iv := range ivs {
		h += iv.End.Sub(iv.Start).Hours()
	}
	return h
}
