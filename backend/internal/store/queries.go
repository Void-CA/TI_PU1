package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/rules"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrInvalidState     = errors.New("invalid state transition")
	ErrSlotTaken        = errors.New("time slot already occupied")
	ErrOrderNotPending  = errors.New("order is not pending")
	ErrNoJustification  = errors.New("justification required")
)

// CrewView is a crew plus its schedule-derived load for the operating day.
type CrewView struct {
	Crew       domain.Crew   `json:"crew"`
	LoadHours  float64       `json:"load_hours"`
	Busy       []rules.Interval `json:"busy"`
	// LastPoint is the crew's current starting point: its base if it has no
	// assignments that day, otherwise the location of its last order.
	LastX float64 `json:"last_x"`
	LastY float64 `json:"last_y"`
}

func (s *Store) ListCrews(ctx context.Context, day time.Time) ([]CrewView, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, name, members, skills, zone, base_x, base_y,
		       available_from_min, available_to_min, active
		FROM crews ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var crews []domain.Crew
	for rows.Next() {
		var c domain.Crew
		if err := rows.Scan(&c.ID, &c.Name, &c.Members, &c.Skills, &c.Zone,
			&c.BaseX, &c.BaseY, &c.AvailableFromMin, &c.AvailableToMin, &c.Active); err != nil {
			return nil, err
		}
		crews = append(crews, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	views := make([]CrewView, 0, len(crews))
	for _, c := range crews {
		v := CrewView{Crew: c, Busy: []rules.Interval{}, LastX: c.BaseX, LastY: c.BaseY}
		as, err := s.crewAssignments(ctx, c.ID, day)
		if err != nil {
			return nil, err
		}
		for _, a := range as {
			v.Busy = append(v.Busy, rules.Interval{Start: a.Start, End: a.End})
			v.LoadHours += a.End.Sub(a.Start).Hours()
		}
		// Start point = location of the chronologically last assignment's order.
		if len(as) > 0 {
			last := as[0]
			for _, a := range as[1:] {
				if a.End.After(last.End) {
					last = a
				}
			}
			var x, y float64
			if err := s.Pool.QueryRow(ctx, `
				SELECT sr.location_x, sr.location_y
				FROM work_orders wo JOIN service_requests sr ON sr.id = wo.request_id
				WHERE wo.id = $1`, last.OrderID).Scan(&x, &y); err == nil {
				v.LastX, v.LastY = x, y
			}
		}
		views = append(views, v)
	}
	return views, nil
}

// crewAssignments returns blocking assignments for a crew on the given day,
// ordered by start time.
func (s *Store) crewAssignments(ctx context.Context, crewID int64, day time.Time) ([]domain.Assignment, error) {
	return s.assignmentsFor(ctx, crewID, day, s.Pool)
}

func (s *Store) assignmentsFor(ctx context.Context, crewID int64, day time.Time, q querier) ([]domain.Assignment, error) {
	from := day
	to := day.Add(24 * time.Hour)
	rows, err := q.Query(ctx, `
		SELECT id, order_id, crew_id, start_at, end_at, status, justification, created_at
		FROM assignments
		WHERE crew_id = $1 AND status IN ('confirmed','in_progress','completed')
		  AND start_at >= $2 AND start_at < $3
		ORDER BY start_at`, crewID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Assignment
	for rows.Next() {
		var a domain.Assignment
		if err := rows.Scan(&a.ID, &a.OrderID, &a.CrewID, &a.Start, &a.End,
			&a.Status, &a.Justification, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// RequestView joins a service request with its order (if any).
type RequestView struct {
	Request domain.Request    `json:"request"`
	Order   *domain.WorkOrder `json:"order,omitempty"`
}

func (s *Store) ListRequests(ctx context.Context) ([]RequestView, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, r.customer, r.service_type, r.description, r.priority,
		       r.location_x, r.location_y, r.window_start, r.window_end,
		       r.duration_min, r.status, r.created_at,
		       wo.id, wo.requirements, wo.duration_min, wo.status, wo.created_at
		FROM service_requests r
		LEFT JOIN work_orders wo ON wo.request_id = r.id
		ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequestView
	for rows.Next() {
		var rv RequestView
		var orderID *int64
		var reqs []string
		var odur *int
		var ost *string
		var oc *time.Time
		if err := rows.Scan(&rv.Request.ID, &rv.Request.Customer, &rv.Request.ServiceType,
			&rv.Request.Description, &rv.Request.Priority, &rv.Request.LocationX,
			&rv.Request.LocationY, &rv.Request.WindowStart, &rv.Request.WindowEnd,
			&rv.Request.DurationMin, &rv.Request.Status, &rv.Request.CreatedAt,
			&orderID, &reqs, &odur, &ost, &oc); err != nil {
			return nil, err
		}
		if orderID != nil {
			rv.Order = &domain.WorkOrder{
				ID: *orderID, RequestID: rv.Request.ID, Requirements: reqs,
				DurationMin: *odur, Status: domain.OrderStatus(*ost), CreatedAt: *oc,
				Customer: rv.Request.Customer, Priority: rv.Request.Priority,
				LocationX: rv.Request.LocationX, LocationY: rv.Request.LocationY,
				WindowStart: rv.Request.WindowStart, WindowEnd: rv.Request.WindowEnd,
				ServiceType: rv.Request.ServiceType,
			}
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

// CreateRequest registers a new service request (status received).
func (s *Store) CreateRequest(ctx context.Context, r domain.Request) (domain.Request, error) {
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO service_requests (customer, service_type, description, priority,
		                              location_x, location_y, window_start, window_end,
		                              duration_min, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'received')
		RETURNING id, created_at`,
		r.Customer, r.ServiceType, r.Description, r.Priority,
		r.LocationX, r.LocationY, r.WindowStart, r.WindowEnd, r.DurationMin,
	).Scan(&r.ID, &r.CreatedAt)
	r.Status = domain.RequestReceived
	return r, err
}

// RequirementsFor maps a service type to the skills an order requires.
func RequirementsFor(serviceType string) []string {
	switch serviceType {
	case "fiber":
		return []string{"fiber"}
	case "router":
		return []string{"router"}
	case "splicing":
		return []string{"splicing"}
	case "cabling":
		return []string{"cabling"}
	case "coax":
		return []string{"coax"}
	case "fiber_splice":
		return []string{"fiber", "splicing"}
	default:
		// Unknown service types require a skill nobody may have:
		// realistic unassignable case for demos.
		return []string{serviceType}
	}
}

// EvaluateRequest decides remote vs on-site. Remote closes the request;
// on-site creates a work order (status pending).
func (s *Store) EvaluateRequest(ctx context.Context, requestID int64, remote bool) (RequestView, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return RequestView{}, err
	}
	defer tx.Rollback(ctx)

	var rv RequestView
	err = tx.QueryRow(ctx, `
		SELECT id, customer, service_type, description, priority,
		       location_x, location_y, window_start, window_end,
		       duration_min, status, created_at
		FROM service_requests WHERE id = $1 FOR UPDATE`, requestID,
	).Scan(&rv.Request.ID, &rv.Request.Customer, &rv.Request.ServiceType,
		&rv.Request.Description, &rv.Request.Priority, &rv.Request.LocationX,
		&rv.Request.LocationY, &rv.Request.WindowStart, &rv.Request.WindowEnd,
		&rv.Request.DurationMin, &rv.Request.Status, &rv.Request.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RequestView{}, ErrNotFound
	}
	if err != nil {
		return RequestView{}, err
	}
	if rv.Request.Status != domain.RequestReceived {
		return RequestView{}, fmt.Errorf("%w: request already evaluated", ErrInvalidState)
	}

	if remote {
		if _, err := tx.Exec(ctx,
			`UPDATE service_requests SET status = 'resolved_remote' WHERE id = $1`, requestID); err != nil {
			return RequestView{}, err
		}
		rv.Request.Status = domain.RequestRemote
		return rv, tx.Commit(ctx)
	}

	reqs := RequirementsFor(rv.Request.ServiceType)
	var orderID int64
	var orderCreated time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO work_orders (request_id, requirements, duration_min, status)
		VALUES ($1,$2,$3,'pending')
		RETURNING id, created_at`,
		requestID, reqs, rv.Request.DurationMin,
	).Scan(&orderID, &orderCreated)
	if err != nil {
		return RequestView{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE service_requests SET status = 'with_order' WHERE id = $1`, requestID); err != nil {
		return RequestView{}, err
	}
	rv.Request.Status = domain.RequestWithOrder
	rv.Order = &domain.WorkOrder{
		ID: orderID, RequestID: requestID, Requirements: reqs,
		DurationMin: rv.Request.DurationMin, Status: domain.OrderPending,
		CreatedAt: orderCreated, Customer: rv.Request.Customer,
		Priority: rv.Request.Priority, LocationX: rv.Request.LocationX,
		LocationY: rv.Request.LocationY, WindowStart: rv.Request.WindowStart,
		WindowEnd: rv.Request.WindowEnd, ServiceType: rv.Request.ServiceType,
	}
	return rv, tx.Commit(ctx)
}

// ListOrders returns work orders with their denormalized request data,
// optionally filtered by status.
func (s *Store) ListOrders(ctx context.Context, status *domain.OrderStatus) ([]domain.WorkOrder, error) {
	q := `
		SELECT wo.id, wo.request_id, wo.requirements, wo.duration_min, wo.status, wo.created_at,
		       sr.customer, sr.priority, sr.location_x, sr.location_y,
		       sr.window_start, sr.window_end, sr.service_type
		FROM work_orders wo
		JOIN service_requests sr ON sr.id = wo.request_id`
	args := []any{}
	if status != nil {
		q += ` WHERE wo.status = $1`
		args = append(args, string(*status))
	}
	q += ` ORDER BY wo.id`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOrders(rows)
}

func (s *Store) GetOrder(ctx context.Context, id int64) (domain.WorkOrder, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT wo.id, wo.request_id, wo.requirements, wo.duration_min, wo.status, wo.created_at,
		       sr.customer, sr.priority, sr.location_x, sr.location_y,
		       sr.window_start, sr.window_end, sr.service_type
		FROM work_orders wo
		JOIN service_requests sr ON sr.id = wo.request_id
		WHERE wo.id = $1`, id)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	defer rows.Close()
	orders, err := scanOrders(rows)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	if len(orders) == 0 {
		return domain.WorkOrder{}, ErrNotFound
	}
	return orders[0], nil
}

func scanOrders(rows pgx.Rows) ([]domain.WorkOrder, error) {
	var out []domain.WorkOrder
	for rows.Next() {
		var o domain.WorkOrder
		if err := rows.Scan(&o.ID, &o.RequestID, &o.Requirements, &o.DurationMin,
			&o.Status, &o.CreatedAt, &o.Customer, &o.Priority, &o.LocationX,
			&o.LocationY, &o.WindowStart, &o.WindowEnd, &o.ServiceType); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ScheduleView is one assignment joined with its order for the crew UI.
type ScheduleView struct {
	Assignment domain.Assignment  `json:"assignment"`
	Order      *domain.WorkOrder  `json:"order,omitempty"`
}

// Schedule returns the operating-day schedule of a crew.
func (s *Store) Schedule(ctx context.Context, crewID int64, day time.Time) ([]ScheduleView, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT a.id, a.order_id, a.crew_id, a.start_at, a.end_at, a.status, a.justification, a.created_at,
		       wo.id, wo.request_id, wo.requirements, wo.duration_min, wo.status, wo.created_at,
		       sr.customer, sr.priority, sr.location_x, sr.location_y,
		       sr.window_start, sr.window_end, sr.service_type
		FROM assignments a
		JOIN work_orders wo ON wo.id = a.order_id
		JOIN service_requests sr ON sr.id = wo.request_id
		WHERE a.crew_id = $1 AND a.status IN ('confirmed','in_progress','completed')
		  AND a.start_at >= $2 AND a.start_at < $3
		ORDER BY a.start_at`, crewID, day, day.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduleView
	for rows.Next() {
		var sv ScheduleView
		var o domain.WorkOrder
		if err := rows.Scan(&sv.Assignment.ID, &sv.Assignment.OrderID, &sv.Assignment.CrewID,
			&sv.Assignment.Start, &sv.Assignment.End, &sv.Assignment.Status,
			&sv.Assignment.Justification, &sv.Assignment.CreatedAt,
			&o.ID, &o.RequestID, &o.Requirements, &o.DurationMin, &o.Status, &o.CreatedAt,
			&o.Customer, &o.Priority, &o.LocationX, &o.LocationY,
			&o.WindowStart, &o.WindowEnd, &o.ServiceType); err != nil {
			return nil, err
		}
		sv.Order = &o
		out = append(out, sv)
	}
	return out, rows.Err()
}
