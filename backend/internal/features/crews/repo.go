// Package crews exposes crew data: the list with schedule-derived load, and
// the per-crew operating-day schedule.
package crews

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/domain"
)

// CrewView is a crew plus its schedule-derived load for the operating day.
type CrewView struct {
	Crew      domain.Crew       `json:"crew"`
	LoadHours float64           `json:"load_hours"`
	Busy      []domain.Interval `json:"busy"`
	// LastX/LastY is the crew's current starting point: its base if it has no
	// assignments that day, otherwise the location of its last order.
	LastX float64 `json:"last_x"`
	LastY float64 `json:"last_y"`
}

// ScheduleView is one assignment joined with its order for the crew UI.
type ScheduleView struct {
	Assignment domain.Assignment `json:"assignment"`
	Order      *domain.WorkOrder `json:"order,omitempty"`
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// ListCrews returns every crew with its busy intervals, load and start point.
func (r *Repo) ListCrews(ctx context.Context, day time.Time) ([]CrewView, error) {
	rows, err := r.pool.Query(ctx, `
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
		v := CrewView{Crew: c, Busy: []domain.Interval{}, LastX: c.BaseX, LastY: c.BaseY}
		as, err := r.crewAssignments(ctx, c.ID, day)
		if err != nil {
			return nil, err
		}
		for _, a := range as {
			v.Busy = append(v.Busy, domain.Interval{Start: a.Start, End: a.End})
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
			if err := r.pool.QueryRow(ctx, `
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
func (r *Repo) crewAssignments(ctx context.Context, crewID int64, day time.Time) ([]domain.Assignment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, order_id, crew_id, start_at, end_at, status, justification, created_at
		FROM assignments
		WHERE crew_id = $1 AND status IN ('confirmed','in_progress','completed')
		  AND start_at >= $2 AND start_at < $3
		ORDER BY start_at`, crewID, day, day.Add(24*time.Hour))
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

// Schedule returns the operating-day schedule of a crew, joined with orders.
func (r *Repo) Schedule(ctx context.Context, crewID int64, day time.Time) ([]ScheduleView, error) {
	rows, err := r.pool.Query(ctx, `
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
