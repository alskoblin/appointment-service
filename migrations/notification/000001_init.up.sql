CREATE TABLE processed_events (
    event_key TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notification_logs (
    id BIGSERIAL PRIMARY KEY,
    event_key TEXT NOT NULL,
    event_type TEXT NOT NULL,
    appointment_id BIGINT NOT NULL,
    client_id BIGINT NOT NULL,
    telegram_chat_id BIGINT,
    status TEXT NOT NULL,
    error_text TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notification_logs_event_key ON notification_logs (event_key);
CREATE INDEX idx_notification_logs_created_at ON notification_logs (created_at);
