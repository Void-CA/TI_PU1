-- 0001_init.sql — core schema for the field-service assignment prototype.

-- btree_gist enables the GiST exclusion constraint on (crew_id, time range).
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE crews (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL,
    members           TEXT[] NOT NULL DEFAULT '{}',
    skills            TEXT[] NOT NULL DEFAULT '{}',
    zone              TEXT NOT NULL DEFAULT '',
    base_x            DOUBLE PRECISION NOT NULL DEFAULT 0,
    base_y            DOUBLE PRECISION NOT NULL DEFAULT 0,
    available_from_min INT NOT NULL DEFAULT 480, -- 08:00
    available_to_min   INT NOT NULL DEFAULT 1020, -- 17:00
    active            BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE service_requests (
    id            BIGSERIAL PRIMARY KEY,
    customer      TEXT NOT NULL,
    service_type  TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    priority      INT NOT NULL DEFAULT 2 CHECK (priority BETWEEN 1 AND 3),
    location_x    DOUBLE PRECISION NOT NULL,
    location_y    DOUBLE PRECISION NOT NULL,
    window_start  TIMESTAMPTZ NOT NULL,
    window_end    TIMESTAMPTZ NOT NULL,
    duration_min  INT NOT NULL CHECK (duration_min > 0),
    status        TEXT NOT NULL DEFAULT 'received'
                  CHECK (status IN ('received','resolved_remote','with_order','cancelled')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE work_orders (
    id           BIGSERIAL PRIMARY KEY,
    request_id   BIGINT NOT NULL REFERENCES service_requests(id),
    requirements TEXT[] NOT NULL DEFAULT '{}',
    duration_min INT NOT NULL CHECK (duration_min > 0),
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','assigned','in_progress','completed','incident','cancelled')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE assignments (
    id             BIGSERIAL PRIMARY KEY,
    order_id       BIGINT NOT NULL REFERENCES work_orders(id),
    crew_id        BIGINT NOT NULL REFERENCES crews(id),
    start_at       TIMESTAMPTZ NOT NULL,
    end_at         TIMESTAMPTZ NOT NULL,
    status         TEXT NOT NULL DEFAULT 'confirmed'
                   CHECK (status IN ('confirmed','in_progress','completed','cancelled','replaced')),
    justification  TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_at > start_at),
    -- Atomic guard against double-booking: two blocking assignments for the same
    -- crew can never overlap. Cancelled/replaced assignments free their slot.
    CONSTRAINT no_overlap_blocking_assignments EXCLUDE USING gist (
        crew_id WITH =,
        tstzrange(start_at, end_at) WITH &&
    ) WHERE (status IN ('confirmed','in_progress','completed'))
);

CREATE INDEX idx_work_orders_status ON work_orders(status);
CREATE INDEX idx_assignments_crew ON assignments(crew_id, start_at);
CREATE INDEX idx_assignments_order ON assignments(order_id);
