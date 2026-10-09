// Package store implements persistence with pgx/PostgreSQL.
//
// The database is the authority on confirmed assignments: the EXCLUDE
// constraint in the schema is the hard guarantee against double-booking,
// and confirmation always re-verifies availability inside a transaction.
package store

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pu1/backend/internal/sim"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// OperatingDay is the single workday used by the prototype (fixed for
// reproducibility of the demo; documented in the README).
var OperatingDay = time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)

type Store struct {
	Pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() {
	s.Pool.Close()
}

// Migrate applies the embedded SQL migrations in order, once each.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		if err := s.Pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// SeedIfEmpty populates crews, requests, orders and the initial schedule from
// the fixed synthetic dataset (the same one used by the evaluation).
func (s *Store) SeedIfEmpty(ctx context.Context) error {
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM crews`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	ds := sim.Build()
	day := OperatingDay

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Crews.
	for _, cs := range ds.Crews {
		c := cs.Crew
		if _, err := tx.Exec(ctx, `
			INSERT INTO crews (id, name, members, skills, zone, base_x, base_y,
			                   available_from_min, available_to_min, active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			c.ID, c.Name, c.Members, c.Skills, c.Zone, c.BaseX, c.BaseY,
			c.AvailableFromMin, c.AvailableToMin, c.Active,
		); err != nil {
			return fmt.Errorf("seed crew %s: %w", c.Name, err)
		}
	}

	// Requests + orders.
	for _, o := range ds.Orders {
		if _, err := tx.Exec(ctx, `
			INSERT INTO service_requests (id, customer, service_type, description, priority,
			                              location_x, location_y, window_start, window_end,
			                              duration_min, status)
			VALUES ($1,$2,$3,'',$4,$5,$6,$7,$8,$9,'with_order')`,
			o.RequestID, o.Customer, o.ServiceType,
			o.Priority, o.LocationX, o.LocationY, o.WindowStart, o.WindowEnd, o.DurationMin,
		); err != nil {
			return fmt.Errorf("seed request %d: %w", o.RequestID, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO work_orders (id, request_id, requirements, duration_min, status)
			VALUES ($1,$2,$3,$4,'pending')`,
			o.ID, o.RequestID, o.Requirements, o.DurationMin,
		); err != nil {
			return fmt.Errorf("seed order %d: %w", o.ID, err)
		}
	}

	// Dataset inserts used explicit ids 1..12; realign sequences BEFORE inserting
	// maintenance rows so their auto-ids do not collide.
	if err := seedSetvals(ctx, tx); err != nil {
		return err
	}

	// Initial schedule: one maintenance request/order per crew with busy intervals.
	for _, cs := range ds.Crews {
		if len(cs.Busy) == 0 {
			continue
		}
		var reqID, orderID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO service_requests (customer, service_type, description, priority,
			                              location_x, location_y, window_start, window_end,
			                              duration_min, status)
			VALUES ($1,'maintenance','Ventana de mantenimiento inicial',2,$2,$3,$4,$5,60,'with_order')
			RETURNING id`,
			"Mantenimiento "+cs.Crew.Name, cs.Crew.BaseX, cs.Crew.BaseY, day, day,
		).Scan(&reqID); err != nil {
			return fmt.Errorf("seed maintenance request: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO work_orders (request_id, requirements, duration_min, status)
			VALUES ($1,'{}',60,'assigned')
			RETURNING id`, reqID).Scan(&orderID); err != nil {
			return fmt.Errorf("seed maintenance order: %w", err)
		}
		for _, iv := range cs.Busy {
			start := atMinute(day, iv.Start)
			end := atMinute(day, iv.End)
			if _, err := tx.Exec(ctx, `
				INSERT INTO assignments (order_id, crew_id, start_at, end_at, status, justification)
				VALUES ($1,$2,$3,$4,'confirmed','initial schedule (seed)')`,
				orderID, cs.Crew.ID, start, end,
			); err != nil {
				return fmt.Errorf("seed initial assignment crew %s: %w", cs.Crew.Name, err)
			}
		}
	}

	return tx.Commit(ctx)
}

func seedSetvals(ctx context.Context, tx pgx.Tx) error {
	pairs := [][2]string{
		{"crews", "id"},
		{"service_requests", "id"},
		{"work_orders", "id"},
		{"assignments", "id"},
	}
	for _, p := range pairs {
		q := fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence('%s', '%s'), GREATEST(COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, 1), false)`,
			p[0], p[1], p[1], p[0])
		if _, err := tx.Exec(ctx, q); err != nil {
			return fmt.Errorf("setval %s: %w", p[0], err)
		}
	}
	return nil
}

// atMinute returns the day at the same hour/minute as t (UTC wall clock).
func atMinute(day, t time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
}
