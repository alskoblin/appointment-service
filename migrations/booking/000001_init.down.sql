DROP INDEX IF EXISTS idx_outbox_pending;
DROP INDEX IF EXISTS idx_schedules_work_date;
DROP INDEX IF EXISTS idx_appointments_status;
DROP INDEX IF EXISTS idx_appointments_client_start_time;
DROP INDEX IF EXISTS idx_appointments_specialist_start_time;

DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS appointments;
DROP TYPE IF EXISTS appointment_status;
DROP TABLE IF EXISTS schedules;
DROP TABLE IF EXISTS clients;
DROP TABLE IF EXISTS specialists;
