CREATE TABLE processed_events (
    event_key TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE appointment_calendar_events (
    appointment_id BIGINT PRIMARY KEY,
    google_event_id TEXT,
    status TEXT NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    specialist_name TEXT NOT NULL,
    client_name TEXT NOT NULL,
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE calendar_sync_logs (
    id BIGSERIAL PRIMARY KEY,
    event_key TEXT NOT NULL,
    event_type TEXT NOT NULL,
    appointment_id BIGINT NOT NULL,
    status TEXT NOT NULL,
    google_event_id TEXT,
    error_text TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_calendar_sync_logs_event_key ON calendar_sync_logs (event_key);
CREATE INDEX idx_calendar_sync_logs_created_at ON calendar_sync_logs (created_at);
CREATE INDEX idx_appointment_calendar_events_status ON appointment_calendar_events (status);
