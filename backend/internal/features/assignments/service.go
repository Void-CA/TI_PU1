// Package assignments owns the transactional core: candidate evaluation,
// atomic confirmation, reassignment, cancellation and the concurrency demo.
//
// The database is the authority on confirmed assignments: the EXCLUDE
// constraint in the schema is the hard guarantee against double-booking,
// and confirmation always re-verifies availability inside a transaction.
package assignments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/engine"
	"pu1/backend/internal/features/crews"
	"pu1/backend/internal/platform/db"
)

// Service coordinates the assignment use cases. It talks to PostgreSQL
// directly (no HTTP, no handlers of other features).
type Service struct {
	pool    *pgxpool.Pool
	crews   *crews.Repo
	day     time.Time
	weights engine.Weights
	refs    engine.Refs
}

func NewService(pool *pgxpool.Pool, day time.Time, w engine.Weights, r engine.Refs) *Service {
	return &Service{pool: pool, crews: crews.NewRepo(pool), day: day, weights: w, refs: r}
}

// ConfirmInput is the data required to confirm an assignment.
type ConfirmInput struct {
	OrderID       int64
	CrewID        int64
	Justification string
	// Optional explicit slot; when zero, the earliest free slot is computed.
	Start time.Time
	End   time.Time
}

// Candidates evaluates every crew for one order against the current DB state.
// Crews are evaluated in parallel; the result slice is ordered by crew id.
func (s *Service) Candidates(ctx context.Context, orderID int64) ([]engine.Candidate, error) {
	order, err := s.order(ctx, s.pool, orderID, false)
	if err != nil {
		return nil, err
	}
	views, err := s.crews.ListCrews(ctx, s.day)
	if err != nil {
		return nil, err
	}

	totalLoad := 0.0
	for _, v := range views {
		totalLoad += v.LoadHours
	}

	cands := make([]engine.Candidate, len(views))
	done := make(chan int, len(views))
	for i, v := range views {
		go func(i int, v crews.CrewView) {
			crew := v.Crew
			// Distance from the crew's actual start point (base or last order).
			crew.BaseX, crew.BaseY = v.LastX, v.LastY
			cands[i] = engine.EvaluateCrew(crew, order, v.Busy, v.LoadHours, totalLoad, len(views), s.weights, s.refs, s.day)
			done <- i
		}(i, v)
	}
	for range views {
		<-done
	}
	return cands, nil
}

// Confirm atomically verifies current availability and reserves the slot.
// Returns db.ErrSlotTaken when another operation occupied the interval first
// (mapped from the DB exclusion constraint or from the in-transaction check).
func (s *Service) Confirm(ctx context.Context, in ConfirmInput) (domain.Assignment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Assignment{}, err
	}
	defer tx.Rollback(ctx)

	// Serialize confirmations per crew so the availability re-check below is
	// race-free even before the constraint fires.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, in.CrewID); err != nil {
		return domain.Assignment{}, err
	}

	order, err := s.order(ctx, tx, in.OrderID, true)
	if err != nil {
		return domain.Assignment{}, err
	}
	if order.Status != domain.OrderPending {
		return domain.Assignment{}, db.ErrOrderNotPending
	}

	start, end := in.Start, in.End
	if start.IsZero() || end.IsZero() {
		slot, ok, err := s.earliestSlot(ctx, tx, in.CrewID, order)
		if err != nil {
			return domain.Assignment{}, err
		}
		if !ok {
			return domain.Assignment{}, db.ErrSlotTaken
		}
		start, end = slot.Start, slot.End
	}

	var a domain.Assignment
	err = tx.QueryRow(ctx, `
		INSERT INTO assignments (order_id, crew_id, start_at, end_at, status, justification)
		VALUES ($1,$2,$3,$4,'confirmed',$5)
		RETURNING id, order_id, crew_id, start_at, end_at, status, justification, created_at`,
		in.OrderID, in.CrewID, start, end, in.Justification,
	).Scan(&a.ID, &a.OrderID, &a.CrewID, &a.Start, &a.End, &a.Status, &a.Justification, &a.CreatedAt)
	if err != nil {
		if isExclusionViolation(err) {
			return domain.Assignment{}, db.ErrSlotTaken
		}
		return domain.Assignment{}, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE work_orders SET status = 'assigned' WHERE id = $1`, in.OrderID); err != nil {
		return domain.Assignment{}, err
	}
	return a, tx.Commit(ctx)
}

// Reassign replaces an existing blocking assignment with a new one on another
// crew, all inside a single transaction: if the new reservation cannot be
// created, the previous assignment is preserved (rollback).
func (s *Service) Reassign(ctx context.Context, assignmentID, newCrewID int64, justification string) (domain.Assignment, error) {
	if justification == "" {
		return domain.Assignment{}, db.ErrNoJustification
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Assignment{}, err
	}
	defer tx.Rollback(ctx)

	// Lock the current assignment: it must still be blocking.
	var old domain.Assignment
	err = tx.QueryRow(ctx, `
		SELECT id, order_id, crew_id, start_at, end_at, status, justification, created_at
		FROM assignments WHERE id = $1 FOR UPDATE`, assignmentID,
	).Scan(&old.ID, &old.OrderID, &old.CrewID, &old.Start, &old.End,
		&old.Status, &old.Justification, &old.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Assignment{}, db.ErrNotFound
	}
	if err != nil {
		return domain.Assignment{}, err
	}
	if !old.Status.Blocking() {
		return domain.Assignment{}, fmt.Errorf("%w: assignment is %s", db.ErrInvalidState, old.Status)
	}
	if old.CrewID == newCrewID {
		return domain.Assignment{}, fmt.Errorf("%w: same crew", db.ErrInvalidState)
	}

	// Serialize the new crew's confirmations too.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, newCrewID); err != nil {
		return domain.Assignment{}, err
	}

	order, err := s.order(ctx, tx, old.OrderID, true)
	if err != nil {
		return domain.Assignment{}, err
	}

	crew, err := s.crewRow(ctx, tx, newCrewID)
	if err != nil {
		return domain.Assignment{}, err
	}
	busy, err := s.busyIntervals(ctx, tx, newCrewID)
	if err != nil {
		return domain.Assignment{}, err
	}
	load := 0.0
	for _, iv := range busy {
		load += iv.End.Sub(iv.Start).Hours()
	}
	cand := engine.EvaluateCrew(crew, order, busy, load, load, 1, s.weights, s.refs, s.day)
	if !cand.Feasible {
		return domain.Assignment{}, fmt.Errorf("%w: no free slot on crew %d", db.ErrSlotTaken, newCrewID)
	}

	// Insert the replacement first, then free the old one.
	var na domain.Assignment
	err = tx.QueryRow(ctx, `
		INSERT INTO assignments (order_id, crew_id, start_at, end_at, status, justification)
		VALUES ($1,$2,$3,$4,'confirmed',$5)
		RETURNING id, order_id, crew_id, start_at, end_at, status, justification, created_at`,
		old.OrderID, newCrewID, cand.Start, cand.End, justification,
	).Scan(&na.ID, &na.OrderID, &na.CrewID, &na.Start, &na.End, &na.Status, &na.Justification, &na.CreatedAt)
	if err != nil {
		if isExclusionViolation(err) {
			return domain.Assignment{}, db.ErrSlotTaken
		}
		return domain.Assignment{}, err
	}

	// The old assignment frees its slot and is marked as replaced.
	if _, err := tx.Exec(ctx, `
		UPDATE assignments SET status = 'replaced', justification = $2 WHERE id = $1`,
		old.ID, fmt.Sprintf("Replaced by assignment %d: %s", na.ID, justification)); err != nil {
		return domain.Assignment{}, err
	}

	return na, tx.Commit(ctx)
}

// CancelAssignment frees the slot and returns the order to pending.
func (s *Service) Cancel(ctx context.Context, assignmentID int64, justification string) error {
	if justification == "" {
		return db.ErrNoJustification
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var orderID int64
	var status domain.AssignmentStatus
	err = tx.QueryRow(ctx, `
		SELECT order_id, status FROM assignments WHERE id = $1 FOR UPDATE`, assignmentID,
	).Scan(&orderID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !status.Blocking() {
		return fmt.Errorf("%w: assignment is %s", db.ErrInvalidState, status)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE assignments SET status = 'cancelled', justification = $2 WHERE id = $1`,
		assignmentID, justification); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE work_orders SET status = 'pending' WHERE id = $1`, orderID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- transactional helpers ---

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Service) order(ctx context.Context, q querier, orderID int64, forUpdate bool) (domain.WorkOrder, error) {
	sqlText := `
		SELECT wo.id, wo.request_id, wo.requirements, wo.duration_min, wo.status, wo.created_at,
		       sr.customer, sr.priority, sr.location_x, sr.location_y,
		       sr.window_start, sr.window_end, sr.service_type
		FROM work_orders wo
		JOIN service_requests sr ON sr.id = wo.request_id
		WHERE wo.id = $1`
	if forUpdate {
		sqlText += ` FOR UPDATE OF wo`
	}
	rows, err := q.Query(ctx, sqlText, orderID)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.WorkOrder{}, err
		}
		return domain.WorkOrder{}, db.ErrNotFound
	}
	var o domain.WorkOrder
	if err := rows.Scan(&o.ID, &o.RequestID, &o.Requirements, &o.DurationMin,
		&o.Status, &o.CreatedAt, &o.Customer, &o.Priority, &o.LocationX,
		&o.LocationY, &o.WindowStart, &o.WindowEnd, &o.ServiceType); err != nil {
		return domain.WorkOrder{}, err
	}
	return o, rows.Err()
}

func (s *Service) crewRow(ctx context.Context, q querier, crewID int64) (domain.Crew, error) {
	var c domain.Crew
	err := q.QueryRow(ctx, `
		SELECT id, name, members, skills, zone, base_x, base_y,
		       available_from_min, available_to_min, active
		FROM crews WHERE id = $1`, crewID,
	).Scan(&c.ID, &c.Name, &c.Members, &c.Skills, &c.Zone,
		&c.BaseX, &c.BaseY, &c.AvailableFromMin, &c.AvailableToMin, &c.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Crew{}, db.ErrNotFound
	}
	return c, err
}

func (s *Service) busyIntervals(ctx context.Context, q querier, crewID int64) ([]domain.Interval, error) {
	rows, err := q.Query(ctx, `
		SELECT start_at, end_at FROM assignments
		WHERE crew_id = $1 AND status IN ('confirmed','in_progress','completed')
		  AND start_at >= $2 AND start_at < $3
		ORDER BY start_at`, crewID, s.day, s.day.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Interval
	for rows.Next() {
		var iv domain.Interval
		if err := rows.Scan(&iv.Start, &iv.End); err != nil {
			return nil, err
		}
		out = append(out, iv)
	}
	return out, rows.Err()
}

// earliestSlot computes the earliest free slot for the order on the crew,
// from the state visible inside the transaction.
func (s *Service) earliestSlot(ctx context.Context, tx pgx.Tx, crewID int64, order domain.WorkOrder) (domain.Interval, bool, error) {
	crew, err := s.crewRow(ctx, tx, crewID)
	if err != nil {
		return domain.Interval{}, false, err
	}
	busy, err := s.busyIntervals(ctx, tx, crewID)
	if err != nil {
		return domain.Interval{}, false, err
	}
	load := 0.0
	for _, iv := range busy {
		load += iv.End.Sub(iv.Start).Hours()
	}
	// Evaluate only for the slot: cost is irrelevant here.
	cand := engine.EvaluateCrew(crew, order, busy, load, load, 1, s.weights, s.refs, s.day)
	if !cand.Feasible {
		return domain.Interval{}, false, nil
	}
	return domain.Interval{Start: cand.Start, End: cand.End}, true, nil
}

// isExclusionViolation reports whether err is PostgreSQL's exclusion_violation
// (code 23P01), raised by the no_overlap_blocking_assignments constraint.
func isExclusionViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23P01"
}

// DemoResult summarizes the simultaneous-confirmation test.
type DemoResult struct {
	CrewID      int64    `json:"crew_id"`
	WindowStart string   `json:"window_start"`
	WindowEnd   string   `json:"window_end"`
	Attempts    int      `json:"attempts"`
	Succeeded   int      `json:"succeeded"`
	Rejected    int      `json:"rejected"`
	OrderIDs    []int64  `json:"order_ids"`
	Errors      []string `json:"errors"`
}

// DemoConcurrency creates fresh pending orders, then fires simultaneous
// confirmations for the same crew and time slot from multiple goroutines.
// Exactly one confirmation may succeed; the rest must be rejected.
func (s *Service) DemoConcurrency(ctx context.Context, crewID int64, attempts int) (DemoResult, error) {
	if attempts < 2 {
		attempts = 2
	}
	if attempts > 8 {
		attempts = 8
	}

	crew, err := s.crewRow(ctx, s.pool, crewID)
	if err != nil {
		return DemoResult{}, err
	}
	if !crew.Active {
		return DemoResult{}, fmt.Errorf("%w: crew %d is inactive", db.ErrInvalidState, crewID)
	}

	// Clean up any previous demo run so the test is repeatable.
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM assignments WHERE justification = 'concurrency demo'`); err != nil {
		return DemoResult{}, err
	}
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM work_orders wo
		USING service_requests sr
		WHERE wo.request_id = sr.id AND sr.customer = 'DEMO concurrencia'`); err != nil {
		return DemoResult{}, err
	}
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM service_requests WHERE customer = 'DEMO concurrencia'`); err != nil {
		return DemoResult{}, err
	}

	// Create the demo orders (customer marked as demo for traceability).
	orderIDs := make([]int64, attempts)
	for i := range orderIDs {
		var reqID int64
		windowStart := s.day.Add(16 * time.Hour) // 16:00-17:00, unlikely to collide
		windowEnd := s.day.Add(17 * time.Hour)
		err := s.pool.QueryRow(ctx, `
			INSERT INTO service_requests (customer, service_type, description, priority,
			                              location_x, location_y, window_start, window_end,
			                              duration_min, status)
			VALUES ('DEMO concurrencia', 'router', 'Prueba de asignación simultánea', 3,
			        4, 4, $1, $2, 45, 'with_order')
			RETURNING id`, windowStart, windowEnd).Scan(&reqID)
		if err != nil {
			return DemoResult{}, err
		}
		if err := s.pool.QueryRow(ctx, `
			INSERT INTO work_orders (request_id, requirements, duration_min, status)
			VALUES ($1, '{router}', 45, 'pending')
			RETURNING id`, reqID).Scan(&orderIDs[i]); err != nil {
			return DemoResult{}, err
		}
	}

	// All goroutines target the same slot on the same crew.
	start := s.day.Add(16 * time.Hour)
	end := s.day.Add(16*time.Hour + 45*time.Minute)

	type outcome struct{ err error }
	outcomes := make(chan outcome, attempts)
	for i := 0; i < attempts; i++ {
		go func(orderID int64) {
			_, err := s.Confirm(ctx, ConfirmInput{
				OrderID:       orderID,
				CrewID:        crewID,
				Justification: "concurrency demo",
				Start:         start,
				End:           end,
			})
			outcomes <- outcome{err}
		}(orderIDs[i])
	}

	res := DemoResult{
		CrewID:      crewID,
		WindowStart: start.Format("15:04"),
		WindowEnd:   end.Format("15:04"),
		Attempts:    attempts,
		OrderIDs:    orderIDs,
		Errors:      []string{},
	}
	for i := 0; i < attempts; i++ {
		o := <-outcomes
		if o.err == nil {
			res.Succeeded++
		} else {
			res.Rejected++
			res.Errors = append(res.Errors, o.err.Error())
		}
	}
	return res, nil
}
