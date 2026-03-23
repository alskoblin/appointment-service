CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE specialists (
    id BIGSERIAL PRIMARY KEY,
    full_name TEXT NOT NULL,
    profession TEXT NOT NULL,
    slot_duration_minutes INTEGER NOT NULL CHECK (slot_duration_minutes > 0 AND slot_duration_minutes <= 240),
    timezone TEXT NOT NULL DEFAULT 'UTC',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE clients (
    id BIGSERIAL PRIMARY KEY,
    full_name TEXT NOT NULL,
    phone TEXT NOT NULL,
    telegram_chat_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE schedules (
    id BIGSERIAL PRIMARY KEY,
    specialist_id BIGINT NOT NULL REFERENCES specialists(id) ON DELETE CASCADE,
    work_date DATE NOT NULL,
    start_minute INTEGER NOT NULL CHECK (start_minute >= 0 AND start_minute < 1440),
    end_minute INTEGER NOT NULL CHECK (end_minute > 0 AND end_minute <= 1440),
    break_start_minute INTEGER,
    break_end_minute INTEGER,
    is_day_off BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT schedules_working_interval_chk CHECK (start_minute < end_minute),
    CONSTRAINT schedules_break_pair_chk CHECK (
        (break_start_minute IS NULL AND break_end_minute IS NULL)
        OR
        (
            break_start_minute IS NOT NULL
            AND break_end_minute IS NOT NULL
            AND break_start_minute < break_end_minute
            AND break_start_minute >= start_minute
            AND break_end_minute <= end_minute
        )
    ),
    CONSTRAINT schedules_unique_by_day UNIQUE (specialist_id, work_date)
);

CREATE TYPE appointment_status AS ENUM ('booked', 'canceled');

CREATE TABLE appointments (
    id BIGSERIAL PRIMARY KEY,
    specialist_id BIGINT NOT NULL REFERENCES specialists(id) ON DELETE RESTRICT,
    client_id BIGINT NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    status appointment_status NOT NULL DEFAULT 'booked',
    cancel_reason TEXT,
    canceled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT appointments_time_interval_chk CHECK (start_time < end_time)
);

ALTER TABLE appointments
ADD CONSTRAINT appointments_no_overlap_per_specialist
EXCLUDE USING gist (
    specialist_id WITH =,
    tstzrange(start_time, end_time, '[)') WITH &&
)
WHERE (status = 'booked');

CREATE TABLE outbox_events (
    id BIGSERIAL PRIMARY KEY,
    event_key TEXT NOT NULL UNIQUE,
    topic TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    published_at TIMESTAMPTZ,
    locked_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_appointments_specialist_start_time ON appointments (specialist_id, start_time);
CREATE INDEX idx_appointments_client_start_time ON appointments (client_id, start_time);
CREATE INDEX idx_appointments_status ON appointments (status);
CREATE INDEX idx_schedules_work_date ON schedules (work_date);
CREATE INDEX idx_outbox_pending ON outbox_events (id) WHERE published_at IS NULL;
