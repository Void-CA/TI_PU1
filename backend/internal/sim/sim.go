// Package sim runs the base-vs-proposed comparison on a fixed synthetic dataset.
// Everything is deterministic: same inputs, same outputs, no randomness.
package sim

import (
	"time"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/rules"
)

var Day = time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)

func at(h, m int) time.Time {
	return time.Date(2026, 10, 9, h, m, 0, 0, time.UTC)
}

// Dataset returns the fixed crews, orders and initial busy intervals used both
// by the evaluation endpoint and by the DB seed.
type CrewSeed struct {
	Crew domain.Crew
	// Initial busy intervals for that day (existing schedule).
	Busy []rules.Interval
}

type Dataset struct {
	Crews  []CrewSeed
	Orders []domain.WorkOrder
}

func Build() Dataset {
	crews := []CrewSeed{
		{Crew: domain.Crew{
			ID: 1, Name: "Alpha", Members: []string{"Ana", "Luis"},
			Skills: []string{"fiber", "router"}, Zone: "Norte",
			BaseX: 2, BaseY: 2, AvailableFromMin: 480, AvailableToMin: 1020, Active: true,
		}, Busy: []rules.Interval{{Start: at(9, 0), End: at(11, 0)}}},
		{Crew: domain.Crew{
			ID: 2, Name: "Beta", Members: []string{"Carla", "Diego"},
			Skills: []string{"fiber", "splicing"}, Zone: "Oriente",
			BaseX: 8, BaseY: 2, AvailableFromMin: 480, AvailableToMin: 1020, Active: true,
		}, Busy: []rules.Interval{{Start: at(10, 0), End: at(12, 0)}}},
		{Crew: domain.Crew{
			ID: 3, Name: "Gamma", Members: []string{"Elena", "Fernando"},
			Skills: []string{"router", "cabling"}, Zone: "Sur",
			BaseX: 2, BaseY: 8, AvailableFromMin: 480, AvailableToMin: 1020, Active: true,
		}, Busy: nil},
		{Crew: domain.Crew{
			ID: 4, Name: "Delta", Members: []string{"Gabriela", "Hugo"},
			Skills: []string{"fiber", "router", "splicing"}, Zone: "Occidente",
			BaseX: 9, BaseY: 9, AvailableFromMin: 540, AvailableToMin: 960, Active: true,
		}, Busy: []rules.Interval{{Start: at(13, 0), End: at(15, 0)}}},
		{Crew: domain.Crew{
			ID: 5, Name: "Epsilon", Members: []string{"Iván"},
			Skills: []string{"coax"}, Zone: "Centro",
			BaseX: 5, BaseY: 5, AvailableFromMin: 480, AvailableToMin: 1020, Active: true,
		}, Busy: []rules.Interval{{Start: at(8, 0), End: at(12, 0)}}},
	}

	orders := []domain.WorkOrder{
		{ID: 1, RequestID: 1, Customer: "Torres del Norte", ServiceType: "fiber", Requirements: []string{"fiber"},
			DurationMin: 60, Priority: domain.PriorityHigh, LocationX: 3, LocationY: 3, WindowStart: at(9, 0), WindowEnd: at(12, 0), Status: domain.OrderPending},
		{ID: 2, RequestID: 2, Customer: "Hotel Oriente", ServiceType: "router", Requirements: []string{"router"},
			DurationMin: 90, Priority: domain.PriorityMedium, LocationX: 7, LocationY: 3, WindowStart: at(9, 0), WindowEnd: at(13, 0), Status: domain.OrderPending},
		{ID: 3, RequestID: 3, Customer: "Plaza Sur", ServiceType: "splicing", Requirements: []string{"splicing"},
			DurationMin: 60, Priority: domain.PriorityMedium, LocationX: 8, LocationY: 8, WindowStart: at(10, 0), WindowEnd: at(14, 0), Status: domain.OrderPending},
		{ID: 4, RequestID: 4, Customer: "Clinica Las Palmas", ServiceType: "fiber", Requirements: []string{"fiber"},
			DurationMin: 45, Priority: domain.PriorityLow, LocationX: 1, LocationY: 9, WindowStart: at(8, 30), WindowEnd: at(11, 0), Status: domain.OrderPending},
		{ID: 5, RequestID: 5, Customer: "Radio Centro", ServiceType: "coax", Requirements: []string{"coax"},
			DurationMin: 120, Priority: domain.PriorityMedium, LocationX: 5, LocationY: 6, WindowStart: at(8, 0), WindowEnd: at(12, 0), Status: domain.OrderPending},
		{ID: 6, RequestID: 6, Customer: "Parque Occidental", ServiceType: "fiber_splice", Requirements: []string{"fiber", "splicing"},
			DurationMin: 60, Priority: domain.PriorityHigh, LocationX: 9, LocationY: 8, WindowStart: at(11, 0), WindowEnd: at(15, 0), Status: domain.OrderPending},
		{ID: 7, RequestID: 7, Customer: "Escuela Sur 12", ServiceType: "cabling", Requirements: []string{"cabling"},
			DurationMin: 60, Priority: domain.PriorityLow, LocationX: 3, LocationY: 7, WindowStart: at(9, 0), WindowEnd: at(12, 0), Status: domain.OrderPending},
		{ID: 8, RequestID: 8, Customer: "Oficinas Centro", ServiceType: "router", Requirements: []string{"router"},
			DurationMin: 30, Priority: domain.PriorityLow, LocationX: 6, LocationY: 5, WindowStart: at(14, 0), WindowEnd: at(17, 0), Status: domain.OrderPending},
		{ID: 9, RequestID: 9, Customer: "Conjunto Alameda", ServiceType: "fiber", Requirements: []string{"fiber"},
			DurationMin: 60, Priority: domain.PriorityMedium, LocationX: 4, LocationY: 4, WindowStart: at(15, 0), WindowEnd: at(17, 0), Status: domain.OrderPending},
		// Order 10 requires a skill no crew has: must stay unassigned under both methods.
		{ID: 10, RequestID: 10, Customer: "Laboratorio X", ServiceType: "experimental", Requirements: []string{"quantum"},
			DurationMin: 60, Priority: domain.PriorityHigh, LocationX: 5, LocationY: 5, WindowStart: at(9, 0), WindowEnd: at(12, 0), Status: domain.OrderPending},
		{ID: 11, RequestID: 11, Customer: "Mercado Norte", ServiceType: "router", Requirements: []string{"router"},
			DurationMin: 45, Priority: domain.PriorityMedium, LocationX: 2, LocationY: 1, WindowStart: at(8, 0), WindowEnd: at(10, 0), Status: domain.OrderPending},
		{ID: 12, RequestID: 12, Customer: "Bodega Oriente", ServiceType: "splicing", Requirements: []string{"splicing"},
			DurationMin: 90, Priority: domain.PriorityMedium, LocationX: 8, LocationY: 1, WindowStart: at(12, 0), WindowEnd: at(16, 0), Status: domain.OrderPending},
	}

	return Dataset{Crews: crews, Orders: orders}
}

// crewState tracks the mutable simulation state of one crew.
type crewState struct {
	crew  domain.Crew
	busy  []rules.Interval
	load  float64 // hours
}

// MethodResult holds the metrics of one assignment method.
type MethodResult struct {
	Method              string             `json:"method"`
	Label               string             `json:"label"` // Spanish label for the UI
	Assigned            int                `json:"assigned"`
	Unassigned          int                `json:"unassigned"`
	ValidPercent        float64            `json:"valid_percent"`
	TotalDistance       float64            `json:"total_distance"`
	TotalWaitMin        float64            `json:"total_wait_min"`
	Conflicts           int                `json:"conflicts"` // must always be 0
	HoursBefore         map[string]float64 `json:"hours_before"`
	HoursAfter          map[string]float64 `json:"hours_after"`
	UnassignedOrderIDs  []int64            `json:"unassigned_order_ids"`
	Assignments         []AssignmentSim    `json:"assignments"`
}

// AssignmentSim is one simulated assignment (order -> crew, slot, cost).
type AssignmentSim struct {
	OrderID    int64   `json:"order_id"`
	CrewID     int64   `json:"crew_id"`
	Crew       string  `json:"crew"`
	Start      string  `json:"start"`
	End        string  `json:"end"`
	DurationMin int    `json:"duration_min"`
	Distance   float64 `json:"distance"`
	WaitMin    float64 `json:"wait_min"`
	Cost       float64 `json:"cost"`
}

// Evaluation is the full comparison report.
type Evaluation struct {
	OrdersTotal int                `json:"orders_total"`
	Day         string             `json:"day"`
	Weights     rules.Weights      `json:"weights"`
	Refs        rules.Refs         `json:"refs"`
	Base        MethodResult       `json:"base"`
	Proposed    MethodResult       `json:"proposed"`
	Assumptions []string           `json:"assumptions"`
	Crews       []string           `json:"crews"`
}

// Run executes both methods over the same fixed dataset.
// The proposed method evaluates crews in parallel (bounded), but the result is
// deterministic because the selection ties break by crew id.
func Run() Evaluation {
	ds := Build()
	w := rules.DefaultWeights()
	r := rules.DefaultRefs()

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
		Day:         Day.Format("2006-01-02"),
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

func runMethod(ds Dataset, key, label string, w rules.Weights, r rules.Refs, hoursBefore map[string]float64) MethodResult {
	// Deterministic processing order for both methods.
	orders := make([]domain.WorkOrder, len(ds.Orders))
	copy(orders, ds.Orders)
	rules.SortForPlanning(orders)

	states := make([]*crewState, len(ds.Crews))
	for i, cs := range ds.Crews {
		busy := make([]rules.Interval, len(cs.Busy))
		copy(busy, cs.Busy)
		states[i] = &crewState{crew: cs.Crew, busy: busy, load: hoursBefore[cs.Crew.Name]}
	}

	res := MethodResult{
		Method:       key,
		Label:        label,
		HoursBefore:  hoursBefore,
		HoursAfter:   map[string]float64{},
		UnassignedOrderIDs: []int64{},
		Assignments:  []AssignmentSim{},
	}

	for _, st := range states {
		res.HoursAfter[st.crew.Name] = st.load
	}

	totalLoad := 0.0
	for _, h := range hoursBefore {
		totalLoad += h
	}

	for _, order := range orders {
		cands := make([]rules.Candidate, len(states))

		// Parallel candidate evaluation (bounded by the number of crews).
		done := make(chan int)
		for i, st := range states {
			go func(i int, st *crewState) {
				cands[i] = rules.EvaluateCrew(st.crew, order, st.busy, st.load, totalLoad, len(states), w, r, Day)
				done <- i
			}(i, st)
		}
		for range states {
			<-done
		}

		var chosen rules.Candidate
		var ok bool
		if key == "base" {
			chosen, ok = rules.SelectBase(cands)
		} else {
			chosen, ok = rules.SelectProposed(cands)
		}
		if !ok {
			res.Unassigned++
			res.UnassignedOrderIDs = append(res.UnassignedOrderIDs, order.ID)
			continue
		}

		// Safety check: a chosen candidate must never overlap an existing interval.
		iv := rules.Interval{Start: chosen.Start, End: chosen.End}
		for _, st := range states {
			if st.crew.ID != chosen.CrewID {
				continue
			}
			for _, busy := range st.busy {
				if overlaps(iv, busy) {
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

func hoursOf(ivs []rules.Interval) float64 {
	h := 0.0
	for _, iv := range ivs {
		h += iv.End.Sub(iv.Start).Hours()
	}
	return h
}

func overlaps(a, b rules.Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}
