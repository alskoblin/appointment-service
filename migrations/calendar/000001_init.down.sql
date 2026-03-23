DROP INDEX IF EXISTS idx_appointment_calendar_events_status;
DROP INDEX IF EXISTS idx_calendar_sync_logs_created_at;
DROP INDEX IF EXISTS idx_calendar_sync_logs_event_key;

DROP TABLE IF EXISTS calendar_sync_logs;
DROP TABLE IF EXISTS appointment_calendar_events;
DROP TABLE IF EXISTS processed_events;
