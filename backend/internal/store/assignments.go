package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"pu1/backend/internal/domain"
	"pu1/backend/internal/rules"
)

// isExclusionViolation reports whether err is PostgreSQL's exclusion_violation
// (code 23P01), raised by the no_overlap_blocking_assignments constraint.
func isExclusionViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23P01"
}

// Candidates evaluates every crew for one order against the current DB state.
// Crews are evaluated in parallel; the result slice is ordered by crew id.
func (s *Store) Candidates(ctx context.Context, orderID int64, day time.Time, w rules.Weights, r rules.Refs) ([]rules.Candidate, error) {
	order, err := s.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	views, err := s.ListCrews(ctx, day)
	if err != nil {
		return nil, err
	}

	totalLoad := 0.0
	for _, v := range views {
		totalLoad += v.LoadHours
	}

	cands := make([]rules.Candidate, len(views))
	done := make(chan int, len(views))
	for i, v := range views {
		go func(i int, v CrewView) {
			crew := v.Crew
			// Distance from the crew's actual start point (base or last order).
			crew.BaseX, crew.BaseY = v.LastX, v.LastY
			cands[i] = rules.EvaluateCrew(crew, order, v.Busy, v.LoadHours, totalLoad, len(views), w, r, day)
			done <- i
		}(i, v)
	}
	for range views {
		<-done
	}
	return cands, nil
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

// Confirm atomically verifies current availability and reserves the slot.
// Returns ErrSlotTaken when another operation occupied the interval first
// (mapped from the DB exclusion constraint or from the in-transaction check).
func (s *Store) Confirm(ctx context.Context, in ConfirmInput, day time.Time, w rules.Weights, r rules.Refs) (domain.Assignment, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Assignment{}, err
	}
	defer tx.Rollback(ctx)

	// Serialize confirmations per crew so the availability re-check below is
	// race-free even before the constraint fires.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, in.CrewID); err != nil {
		return domain.Assignment{}, err
	}

	order, err := orderForUpdate(ctx, tx, in.OrderID)
	if err != nil {
		return domain.Assignment{}, err
	}
	if order.Status != domain.OrderPending {
		return domain.Assignment{}, ErrOrderNotPending
	}

	start, end := in.Start, in.End
	if start.IsZero() || end.IsZero() {
		slot, ok, err := earliestSlotTx(ctx, tx, in.CrewID, order, day)
		if err != nil {
			return domain.Assignment{}, err
		}
		if !ok {
			return domain.Assignment{}, ErrSlotTaken
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
			return domain.Assignment{}, ErrSlotTaken
		}
		return domain.Assignment{}, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE work_orders SET status = 'assigned' WHERE id = $1`, in.OrderID); err != nil {
		return domain.Assignment{}, err
	}
	return a, tx.Commit(ctx)
}

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
		return domain.WorkOrder{}, ErrNotFound
	}
	return orders[0], nil
}

func earliestSlotTx(ctx context.Context, tx pgx.Tx, crewID int64, order domain.WorkOrder, day time.Time) (rules.Interval, bool, error) {
	var crew domain.Crew
	var active bool
	err := tx.QueryRow(ctx, `
		SELECT id, name, members, skills, zone, base_x, base_y,
		       available_from_min, available_to_min, active
		FROM crews WHERE id = $1`, crewID,
	).Scan(&crew.ID, &crew.Name, &crew.Members, &crew.Skills, &crew.Zone,
		&crew.BaseX, &crew.BaseY, &crew.AvailableFromMin, &crew.AvailableToMin, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return rules.Interval{}, false, ErrNotFound
	}
	if err != nil {
		return rules.Interval{}, false, err
	}
	crew.Active = active

	busy, err := s_assignmentsForTx(ctx, tx, crewID, day)
	if err != nil {
		return rules.Interval{}, false, err
	}
	load := 0.0
	for _, iv := range busy {
		load += iv.End.Sub(iv.Start).Hours()
	}
	// Evaluate only for the slot: cost is irrelevant here.
	cand := rules.EvaluateCrew(crew, order, busy, load, load, 1, rules.DefaultWeights(), rules.DefaultRefs(), day)
	if !cand.Feasible {
		return rules.Interval{}, false, nil
	}
	return rules.Interval{Start: cand.Start, End: cand.End}, true, nil
}

func s_assignmentsForTx(ctx context.Context, tx pgx.Tx, crewID int64, day time.Time) ([]rules.Interval, error) {
	rows, err := tx.Query(ctx, `
		SELECT start_at, end_at FROM assignments
		WHERE crew_id = $1 AND status IN ('confirmed','in_progress','completed')
		  AND start_at >= $2 AND start_at < $3
		ORDER BY start_at`, crewID, day, day.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rules.Interval
	for rows.Next() {
		var iv rules.Interval
		if err := rows.Scan(&iv.Start, &iv.End); err != nil {
			return nil, err
		}
		out = append(out, iv)
	}
	return out, rows.Err()
}

// Reassign replaces an existing blocking assignment with a new one on another
// crew, all inside a single transaction: if the new reservation cannot be
// created, the previous assignment is preserved (rollback).
func (s *Store) Reassign(ctx context.Context, assignmentID, newCrewID int64, justification string, day time.Time) (domain.Assignment, error) {
	if justification == "" {
		return domain.Assignment{}, ErrNoJustification
	}
	tx, err := s.Pool.Begin(ctx)
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
		return domain.Assignment{}, ErrNotFound
	}
	if err != nil {
		return domain.Assignment{}, err
	}
	if !old.Status.Blocking() {
		return domain.Assignment{}, fmt.Errorf("%w: assignment is %s", ErrInvalidState, old.Status)
	}
	if old.CrewID == newCrewID {
		return domain.Assignment{}, fmt.Errorf("%w: same crew", ErrInvalidState)
	}

	// Serialize the new crew's confirmations too.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, newCrewID); err != nil {
		return domain.Assignment{}, err
	}

	order, err := orderForUpdate(ctx, tx, old.OrderID)
	if err != nil {
		return domain.Assignment{}, err
	}

	// Compute the new slot on the target crew from current availability.
	var crew domain.Crew
	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT id, name, members, skills, zone, base_x, base_y,
		       available_from_min, available_to_min, active
		FROM crews WHERE id = $1`, newCrewID,
	).Scan(&crew.ID, &crew.Name, &crew.Members, &crew.Skills, &crew.Zone,
		&crew.BaseX, &crew.BaseY, &crew.AvailableFromMin, &crew.AvailableToMin, &active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Assignment{}, ErrNotFound
		}
		return domain.Assignment{}, err
	}
	crew.Active = active
	busy, err := s_assignmentsForTx(ctx, tx, newCrewID, day)
	if err != nil {
		return domain.Assignment{}, err
	}
	load := 0.0
	for _, iv := range busy {
		load += iv.End.Sub(iv.Start).Hours()
	}
	cand := rules.EvaluateCrew(crew, order, busy, load, load, 1, rules.DefaultWeights(), rules.DefaultRefs(), day)
	if !cand.Feasible {
		return domain.Assignment{}, fmt.Errorf("%w: no free slot on crew %d", ErrSlotTaken, newCrewID)
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
			return domain.Assignment{}, ErrSlotTaken
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
func (s *Store) CancelAssignment(ctx context.Context, assignmentID int64, justification string) error {
	if justification == "" {
		return ErrNoJustification
	}
	tx, err := s.Pool.Begin(ctx)
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
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !status.Blocking() {
		return fmt.Errorf("%w: assignment is %s", ErrInvalidState, status)
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

// UpdateOrderStatus moves a work order through its lifecycle. Transitions are
// validated against the domain rules; crew-originated transitions also require
// an active assignment for that crew (soft role check, not real security).
func (s *Store) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus domain.OrderStatus, crewID *int64) (domain.WorkOrder, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	defer tx.Rollback(ctx)

	order, err := orderForUpdate(ctx, tx, orderID)
	if err != nil {
		return domain.WorkOrder{}, err
	}
	if !domain.ValidOrderTransition(order.Status, newStatus) {
		return domain.WorkOrder{}, fmt.Errorf("%w: %s -> %s", ErrInvalidState, order.Status, newStatus)
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
			return domain.WorkOrder{}, fmt.Errorf("%w: crew has no active assignment for this order", ErrInvalidState)
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

// PlanResult is the per-order outcome of a planning run.
type PlanResult struct {
	OrderID  int64           `json:"order_id"`
	CrewID   int64           `json:"crew_id,omitempty"`
	Crew     string          `json:"crew,omitempty"`
	Start    *time.Time      `json:"start,omitempty"`
	End      *time.Time      `json:"end,omitempty"`
	Cost     float64         `json:"cost,omitempty"`
	Assigned bool            `json:"assigned"`
	Reason   string          `json:"reason,omitempty"`
	// Retries counts confirmations rejected by a concurrent change (recalculated).
	Retries int `json:"retries,omitempty"`
}

// PlanDay runs the hybrid initial planning: pending orders are processed in a
// deterministic order (priority desc, window asc, id asc); candidate crews are
// evaluated in parallel per order; the chosen reservation is confirmed
// atomically. If the reservation is rejected because the schedule changed, the
// best remaining candidate is recalculated (bounded retries).
func (s *Store) PlanDay(ctx context.Context, day time.Time, w rules.Weights, r rules.Refs) ([]PlanResult, error) {
	status := domain.OrderPending
	orders, err := s.ListOrders(ctx, &status)
	if err != nil {
		return nil, err
	}
	rules.SortForPlanning(orders)

	var results []PlanResult
	for _, order := range orders {
		res := PlanResult{OrderID: order.ID}
		for attempt := 0; attempt < 3; attempt++ {
			cands, err := s.Candidates(ctx, order.ID, day, w, r)
			if err != nil {
				return nil, err
			}
			chosen, ok := rules.SelectProposed(cands)
			if !ok {
				res.Reason = "no feasible crew"
				break
			}
			_, err = s.Confirm(ctx, ConfirmInput{
				OrderID:       order.ID,
				CrewID:        chosen.CrewID,
				Justification: fmt.Sprintf("batch planning (attempt %d)", attempt+1),
				Start:         chosen.Start,
				End:           chosen.End,
			}, day, w, r)
			if errors.Is(err, ErrSlotTaken) || errors.Is(err, ErrOrderNotPending) {
				res.Retries++
				continue // state changed: recalculate candidates
			}
			if err != nil {
				return nil, err
			}
			res.Assigned = true
			res.CrewID = chosen.CrewID
			res.Crew = chosen.Name
			start, end := chosen.Start, chosen.End
			res.Start, res.End = &start, &end
			res.Cost = chosen.Cost
			res.Reason = ""
			break
		}
		if !res.Assigned && res.Reason == "" {
			res.Reason = "confirmation rejected after retries"
		}
		results = append(results, res)
	}
	return results, nil
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

// DemoConcurrency creates two fresh pending orders, then fires simultaneous
// confirmations for the same crew and time slot from multiple goroutines.
// Exactly one confirmation may succeed; the rest must be rejected.
func (s *Store) DemoConcurrency(ctx context.Context, crewID int64, day time.Time, attempts int) (DemoResult, error) {
	if attempts < 2 {
		attempts = 2
	}
	if attempts > 8 {
		attempts = 8
	}

	var crewActive bool
	if err := s.Pool.QueryRow(ctx,
		`SELECT active FROM crews WHERE id = $1`, crewID).Scan(&crewActive); err != nil {
		return DemoResult{}, ErrNotFound
	}
	if !crewActive {
		return DemoResult{}, fmt.Errorf("%w: crew %d is inactive", ErrInvalidState, crewID)
	}

	// Clean up any previous demo run so the test is repeatable.
	if _, err := s.Pool.Exec(ctx, `
		DELETE FROM assignments WHERE justification = 'concurrency demo'`); err != nil {
		return DemoResult{}, err
	}
	if _, err := s.Pool.Exec(ctx, `
		DELETE FROM work_orders wo
		USING service_requests sr
		WHERE wo.request_id = sr.id AND sr.customer = 'DEMO concurrencia'`); err != nil {
		return DemoResult{}, err
	}
	if _, err := s.Pool.Exec(ctx, `
		DELETE FROM service_requests WHERE customer = 'DEMO concurrencia'`); err != nil {
		return DemoResult{}, err
	}

	// Create the demo orders (customer marked as demo for traceability).
	orderIDs := make([]int64, attempts)
	for i := range orderIDs {
		var reqID int64
		windowStart := day.Add(16 * time.Hour) // 16:00-17:00, unlikely to collide
		windowEnd := day.Add(17 * time.Hour)
		err := s.Pool.QueryRow(ctx, `
			INSERT INTO service_requests (customer, service_type, description, priority,
			                              location_x, location_y, window_start, window_end,
			                              duration_min, status)
			VALUES ('DEMO concurrencia', 'router', 'Prueba de asignación simultánea', 3,
			        4, 4, $1, $2, 45, 'with_order')
			RETURNING id`, windowStart, windowEnd).Scan(&reqID)
		if err != nil {
			return DemoResult{}, err
		}
		if err := s.Pool.QueryRow(ctx, `
			INSERT INTO work_orders (request_id, requirements, duration_min, status)
			VALUES ($1, '{router}', 45, 'pending')
			RETURNING id`, reqID).Scan(&orderIDs[i]); err != nil {
			return DemoResult{}, err
		}
	}

	// All goroutines target the same slot on the same crew.
	start := day.Add(16 * time.Hour)
	end := day.Add(16 * time.Hour + 45*time.Minute)

	type outcome struct {
		err error
	}
	outcomes := make(chan outcome, attempts)
	for i := 0; i < attempts; i++ {
		go func(orderID int64) {
			_, err := s.Confirm(ctx, ConfirmInput{
				OrderID:       orderID,
				CrewID:        crewID,
				Justification: "concurrency demo",
				Start:         start,
				End:           end,
			}, day, rules.DefaultWeights(), rules.DefaultRefs())
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
