ALTER TABLE appointments
ADD COLUMN reminder_24h_sent_at TIMESTAMPTZ,
ADD COLUMN reminder_1h_sent_at TIMESTAMPTZ;

CREATE INDEX idx_appointments_reminder_24h_pending
ON appointments (start_time)
WHERE status = 'booked' AND reminder_24h_sent_at IS NULL;

CREATE INDEX idx_appointments_reminder_1h_pending
ON appointments (start_time)
WHERE status = 'booked' AND reminder_1h_sent_at IS NULL;
