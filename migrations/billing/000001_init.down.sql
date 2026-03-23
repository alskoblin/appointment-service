DROP INDEX IF EXISTS idx_invoices_status;
DROP INDEX IF EXISTS idx_billing_logs_appointment_id;
DROP INDEX IF EXISTS idx_billing_logs_created_at;
DROP INDEX IF EXISTS idx_billing_logs_event_key;

DROP TABLE IF EXISTS billing_logs;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS processed_events;
