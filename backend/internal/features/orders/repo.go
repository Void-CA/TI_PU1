// Package orders exposes work orders: listing, lookup and lifecycle
// transitions. The transition rules themselves live in domain (entity
// invariants); this package owns the use case and its SQL.
package orders

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/platform/db"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// ListOrders returns work orders with denormalized request data, optionally
// filtered by status.
func (r *Repo) ListOrders(ctx context.Context, status *domain.OrderStatus) ([]domain.WorkOrder, error) {
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
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOrders(rows)
}

// GetOrder returns one work order (joined with its request) or ErrNotFound.
func (r *Repo) GetOrder(ctx context.Context, id int64) (domain.WorkOrder, error) {
	rows, err := r.pool.Query(ctx, `
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
		return domain.WorkOrder{}, db.ErrNotFound
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

// orderForUpdate locks the work order row inside a transaction.
func orderForUpdate(ctx context.Context, tx pgx.Tx, orderID int64) (domain.WorkOrder, error) {
	rows, err := tx.Query(ctx, `
		SELECT wo.id, wo.request_id, wo.requirements, wo.duration_min, wo.status, wo.created_at,
		       sr.customer, sr.priority, sr.location_x, sr.location_y,
		       sr.window_start, sr.window_end, sr.service_type
		FROM work_orders wo
		JOIN service_requests sr ON sr.id = wo.request_id
		WHERE wo.id = $1
		FOR UPDATE OF wo`, orderID)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	defer rows.Close()
	orders, err := scanOrders(rows)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	if len(orders) == 0 {
		return domain.WorkOrder{}, db.ErrNotFound
	}
	return orders[0], nil
}

// UpdateStatus moves a work order through its lifecycle. Transitions are
// validated against the domain rules; crew-originated transitions also require
// an active assignment for that crew (soft role check, not real security).
// crewID == nil means the transition comes from the admin.
func (r *Repo) UpdateStatus(ctx context.Context, orderID int64, newStatus domain.OrderStatus, crewID *int64) (domain.WorkOrder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	defer tx.Rollback(ctx)

	order, err := orderForUpdate(ctx, tx, orderID)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	if !domain.ValidOrderTransition(order.Status, newStatus) {
		return domain.WorkOrder{}, fmt.Errorf("%w: %s -> %s", db.ErrInvalidState, order.Status, newStatus)
	}

	// When a crew drives the transition, it must hold the active assignment.
	if crewID != nil {
		var n int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM assignments
			WHERE order_id = $1 AND crew_id = $2 AND status IN ('confirmed','in_progress')`,
			orderID, *crewID).Scan(&n); err != nil {
			return domain.WorkOrder{}, err
		}
		if n == 0 {
			return domain.WorkOrder{}, fmt.Errorf("%w: crew has no active assignment for this order", db.ErrInvalidState)
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE work_orders SET status = $2 WHERE id = $1`, orderID, string(newStatus)); err != nil {
		return domain.WorkOrder{}, err
	}

	// Mirror the order status onto its active assignment.
	switch newStatus {
	case domain.OrderInProgress:
		if _, err := tx.Exec(ctx, `
			UPDATE assignments SET status = 'in_progress'
			WHERE order_id = $1 AND status = 'confirmed'`, orderID); err != nil {
			return domain.WorkOrder{}, err
		}
	case domain.OrderCompleted:
		if _, err := tx.Exec(ctx, `
			UPDATE assignments SET status = 'completed'
			WHERE order_id = $1 AND status IN ('confirmed','in_progress')`, orderID); err != nil {
			return domain.WorkOrder{}, err
		}
	case domain.OrderPending:
		// Returning to pending frees the schedule.
		if _, err := tx.Exec(ctx, `
			UPDATE assignments SET status = 'replaced',
			       justification = 'order returned to pending'
			WHERE order_id = $1 AND status IN ('confirmed','in_progress')`, orderID); err != nil {
			return domain.WorkOrder{}, err
		}
	}

	order.Status = newStatus
	return order, tx.Commit(ctx)
}
