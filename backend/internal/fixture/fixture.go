// Package fixture provides the fixed synthetic dataset of the case study.
// It builds plain domain entities and initial schedule intervals only: no
// engine, no database, no HTTP. Consumed by the evaluation feature and by
// the database seed.
package fixture

import (
	"time"

	"pu1/backend/internal/domain"
)

// Day is the single operating day of the prototype (fixed for reproducibility).
var Day = time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)

func at(h, m int) time.Time {
	return time.Date(2026, 10, 9, h, m, 0, 0, time.UTC)
}

// CrewSeed is a crew plus its initial busy intervals for the operating day.
type CrewSeed struct {
	Crew domain.Crew
	Busy []domain.Interval
}

// Dataset is the fixed set of crews, orders and initial schedule.
type Dataset struct {
	Crews  []CrewSeed
	Orders []domain.WorkOrder
}

// Build returns the deterministic synthetic dataset.
func Build() Dataset {
	crews := []CrewSeed{
		{Crew: domain.Crew{
			ID: 1, Name: "Alpha", Members: []string{"Ana", "Luis"},
			Skills: []string{"fiber", "router"}, Zone: "Norte",
			BaseX: 2, BaseY: 2, AvailableFromMin: 480, AvailableToMin: 1020, Active: true,
		}, Busy: []domain.Interval{{Start: at(9, 0), End: at(11, 0)}}},
		{Crew: domain.Crew{
			ID: 2, Name: "Beta", Members: []string{"Carla", "Diego"},
			Skills: []string{"fiber", "splicing"}, Zone: "Oriente",
			BaseX: 8, BaseY: 2, AvailableFromMin: 480, AvailableToMin: 1020, Active: true,
		}, Busy: []domain.Interval{{Start: at(10, 0), End: at(12, 0)}}},
		{Crew: domain.Crew{
			ID: 3, Name: "Gamma", Members: []string{"Elena", "Fernando"},
			Skills: []string{"router", "cabling"}, Zone: "Sur",
			BaseX: 2, BaseY: 8, AvailableFromMin: 480, AvailableToMin: 1020, Active: true,
		}, Busy: nil},
		{Crew: domain.Crew{
			ID: 4, Name: "Delta", Members: []string{"Gabriela", "Hugo"},
			Skills: []string{"fiber", "router", "splicing"}, Zone: "Occidente",
			BaseX: 9, BaseY: 9, AvailableFromMin: 540, AvailableToMin: 960, Active: true,
		}, Busy: []domain.Interval{{Start: at(13, 0), End: at(15, 0)}}},
		{Crew: domain.Crew{
			ID: 5, Name: "Epsilon", Members: []string{"Iván"},
			Skills: []string{"coax"}, Zone: "Centro",
			BaseX: 5, BaseY: 5, AvailableFromMin: 480, AvailableToMin: 1020, Active: true,
		}, Busy: []domain.Interval{{Start: at(8, 0), End: at(12, 0)}}},
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
