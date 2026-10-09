// Package domain defines the entities and states of the telecommunications
// case study. Pure package: no database or HTTP dependencies.
package domain

import "time"

// Priority of a service request. Higher number = more urgent.
type Priority int

const (
	PriorityLow    Priority = 1
	PriorityMedium Priority = 2
	PriorityHigh   Priority = 3
)

type RequestStatus string

const (
	RequestReceived  RequestStatus = "received"
	RequestRemote    RequestStatus = "resolved_remote"
	RequestWithOrder RequestStatus = "with_order"
	RequestCancelled RequestStatus = "cancelled"
)

type Request struct {
	ID          int64     `json:"id"`
	Customer    string    `json:"customer"`
	ServiceType string    `json:"service_type"`
	Description string    `json:"description"`
	Priority    Priority  `json:"priority"`
	LocationX   float64   `json:"location_x"`
	LocationY   float64   `json:"location_y"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	DurationMin int       `json:"duration_min"`
	Status      RequestStatus `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type OrderStatus string

const (
	OrderPending    OrderStatus = "pending"
	OrderAssigned   OrderStatus = "assigned"
	OrderInProgress OrderStatus = "in_progress"
	OrderCompleted  OrderStatus = "completed"
	OrderIncident   OrderStatus = "incident"
	OrderCancelled  OrderStatus = "cancelled"
)

// WorkOrder is a service request that requires an on-site visit.
type WorkOrder struct {
	ID          int64     `json:"id"`
	RequestID   int64     `json:"request_id"`
	Requirements []string `json:"requirements"`
	DurationMin int       `json:"duration_min"`
	Status      OrderStatus `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	// Denormalized from the request to keep the prototype simple.
	Customer    string    `json:"customer"`
	Priority    Priority  `json:"priority"`
	LocationX   float64   `json:"location_x"`
	LocationY   float64   `json:"location_y"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	ServiceType string    `json:"service_type"`
}

// Crew is a field team.
type Crew struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Members  []string `json:"members"`
	Skills   []string `json:"skills"`
	Zone     string   `json:"zone"`
	BaseX    float64  `json:"base_x"`
	BaseY    float64  `json:"base_y"`
	// Daily availability in minutes from midnight (UTC wall clock).
	AvailableFromMin int  `json:"available_from_min"`
	AvailableToMin   int  `json:"available_to_min"`
	Active           bool `json:"active"`
}

type AssignmentStatus string

const (
	AssignmentConfirmed  AssignmentStatus = "confirmed"
	AssignmentInProgress AssignmentStatus = "in_progress"
	AssignmentCompleted  AssignmentStatus = "completed"
	AssignmentCancelled  AssignmentStatus = "cancelled"
	AssignmentReplaced   AssignmentStatus = "replaced"
)

// Blocking reports whether the assignment occupies the crew's schedule.
// Must match the predicate of the EXCLUDE constraint in the migration.
// Cancelled and replaced assignments free their slot; completed ones still
// represent real work already done on that schedule.
func (s AssignmentStatus) Blocking() bool {
	return s == AssignmentConfirmed || s == AssignmentInProgress || s == AssignmentCompleted
}

type Assignment struct {
	ID            int64            `json:"id"`
	OrderID       int64            `json:"order_id"`
	CrewID        int64            `json:"crew_id"`
	Start         time.Time        `json:"start"`
	End           time.Time        `json:"end"`
	Status        AssignmentStatus `json:"status"`
	Justification string           `json:"justification"`
	CreatedAt     time.Time        `json:"created_at"`
}

// OrderTransitions defines which status transitions a work order may take.
var OrderTransitions = map[OrderStatus][]OrderStatus{
	OrderPending:    {OrderAssigned, OrderCancelled},
	OrderAssigned:   {OrderInProgress, OrderPending, OrderCancelled},
	OrderInProgress: {OrderCompleted, OrderIncident},
	OrderCompleted:  {},
	OrderIncident:   {OrderPending, OrderCancelled},
	OrderCancelled:  {},
}

// ValidOrderTransition reports whether desde → hasta is an allowed transition.
func ValidOrderTransition(desde, hasta OrderStatus) bool {
	for _, e := range OrderTransitions[desde] {
		if e == hasta {
			return true
		}
	}
	return false
}
