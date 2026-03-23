DROP INDEX IF EXISTS idx_appointments_reminder_1h_pending;
DROP INDEX IF EXISTS idx_appointments_reminder_24h_pending;

ALTER TABLE appointments
DROP COLUMN IF EXISTS reminder_1h_sent_at,
DROP COLUMN IF EXISTS reminder_24h_sent_at;
