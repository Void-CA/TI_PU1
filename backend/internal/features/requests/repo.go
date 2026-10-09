// Package requests exposes service requests: listing, creation and the
// remote/on-site evaluation that opens a work order.
package requests

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/platform/db"
)

// RequestView joins a service request with its order (if any).
type RequestView struct {
	Request domain.Request    `json:"request"`
	Order   *domain.WorkOrder `json:"order,omitempty"`
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) ListRequests(ctx context.Context) ([]RequestView, error) {
	rows, err := r.pool.Query(ctx, `
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
func (r *Repo) CreateRequest(ctx context.Context, req domain.Request) (domain.Request, error) {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO service_requests (customer, service_type, description, priority,
		                              location_x, location_y, window_start, window_end,
		                              duration_min, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'received')
		RETURNING id, created_at`,
		req.Customer, req.ServiceType, req.Description, req.Priority,
		req.LocationX, req.LocationY, req.WindowStart, req.WindowEnd, req.DurationMin,
	).Scan(&req.ID, &req.CreatedAt)
	req.Status = domain.RequestReceived
	return req, err
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
func (r *Repo) EvaluateRequest(ctx context.Context, requestID int64, remote bool) (RequestView, error) {
	tx, err := r.pool.Begin(ctx)
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
		return RequestView{}, db.ErrNotFound
	}
	if err != nil {
		return RequestView{}, err
	}
	if rv.Request.Status != domain.RequestReceived {
		return RequestView{}, fmt.Errorf("%w: request already evaluated", db.ErrInvalidState)
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
