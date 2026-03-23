CREATE TABLE processed_events (
    event_key TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invoices (
    appointment_id BIGINT PRIMARY KEY,
    specialist_id BIGINT NOT NULL,
    specialist_name TEXT NOT NULL,
    client_id BIGINT NOT NULL,
    client_name TEXT NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    currency TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'paid', 'canceled', 'refunded', 'payment_failed', 'refund_failed')),
    provider_payment_id TEXT,
    provider_refund_id TEXT,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE billing_logs (
    id BIGSERIAL PRIMARY KEY,
    event_key TEXT NOT NULL,
    event_type TEXT NOT NULL,
    appointment_id BIGINT NOT NULL,
    operation TEXT NOT NULL,
    status TEXT NOT NULL,
    error_text TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_billing_logs_event_key ON billing_logs (event_key);
CREATE INDEX idx_billing_logs_created_at ON billing_logs (created_at);
CREATE INDEX idx_billing_logs_appointment_id ON billing_logs (appointment_id);
CREATE INDEX idx_invoices_status ON invoices (status);
