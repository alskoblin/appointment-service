package postgres

import (
	"context"
	"fmt"

	"appointment-service/internal/calendar/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) IsProcessed(ctx context.Context, eventKey string) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM processed_events WHERE event_key = $1)`
	var exists bool
	if err := r.db.QueryRow(ctx, q, eventKey).Scan(&exists); err != nil {
		return false, fmt.Errorf("check processed event: %w", err)
	}
	return exists, nil
}

func (r *Repository) GetCalendarEventMapping(ctx context.Context, appointmentID int64) (domain.CalendarEventMapping, bool, error) {
	const q = `
		SELECT appointment_id, google_event_id, status
		FROM appointment_calendar_events
		WHERE appointment_id = $1`

	var row domain.CalendarEventMapping
	err := r.db.QueryRow(ctx, q, appointmentID).Scan(&row.AppointmentID, &row.GoogleEventID, &row.Status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.CalendarEventMapping{}, false, nil
		}
		return domain.CalendarEventMapping{}, false, fmt.Errorf("get calendar event mapping: %w", err)
	}
	return row, true, nil
}

func (r *Repository) MarkProcessedAndLog(
	ctx context.Context,
	payload domain.AppointmentEventPayload,
	status domain.SyncStatus,
	googleEventID *string,
	errorText *string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	const insertProcessed = `
		INSERT INTO processed_events (event_key, event_type)
		VALUES ($1, $2)
		ON CONFLICT (event_key) DO NOTHING`
	tag, err := tx.Exec(ctx, insertProcessed, payload.EventKey, payload.EventType)
	if err != nil {
		return fmt.Errorf("insert processed event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	const insertLog = `
		INSERT INTO calendar_sync_logs (event_key, event_type, appointment_id, status, google_event_id, error_text)
		VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := tx.Exec(ctx, insertLog, payload.EventKey, payload.EventType, payload.AppointmentID, status, googleEventID, errorText); err != nil {
		return fmt.Errorf("insert calendar sync log: %w", err)
	}

	switch status {
	case domain.SyncStatusSynced:
		if googleEventID == nil || *googleEventID == "" {
			return fmt.Errorf("google_event_id is required for synced status")
		}
		const upsertSynced = `
			INSERT INTO appointment_calendar_events (
				appointment_id,
				google_event_id,
				status,
				start_time,
				end_time,
				specialist_name,
				client_name,
				last_synced_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now())
			ON CONFLICT (appointment_id) DO UPDATE
			SET google_event_id = EXCLUDED.google_event_id,
			    status = EXCLUDED.status,
			    start_time = EXCLUDED.start_time,
			    end_time = EXCLUDED.end_time,
			    specialist_name = EXCLUDED.specialist_name,
			    client_name = EXCLUDED.client_name,
			    last_synced_at = now(),
			    updated_at = now()`
		if _, err := tx.Exec(
			ctx,
			upsertSynced,
			payload.AppointmentID,
			*googleEventID,
			status,
			payload.StartTime,
			payload.EndTime,
			payload.SpecialistName,
			payload.ClientName,
		); err != nil {
			return fmt.Errorf("upsert appointment_calendar_events (synced): %w", err)
		}
	case domain.SyncStatusDeleted:
		const upsertDeleted = `
			INSERT INTO appointment_calendar_events (
				appointment_id,
				google_event_id,
				status,
				start_time,
				end_time,
				specialist_name,
				client_name,
				last_synced_at
			)
			VALUES ($1, NULL, $2, $3, $4, $5, $6, now())
			ON CONFLICT (appointment_id) DO UPDATE
			SET google_event_id = NULL,
			    status = EXCLUDED.status,
			    start_time = EXCLUDED.start_time,
			    end_time = EXCLUDED.end_time,
			    specialist_name = EXCLUDED.specialist_name,
			    client_name = EXCLUDED.client_name,
			    last_synced_at = now(),
			    updated_at = now()`
		if _, err := tx.Exec(
			ctx,
			upsertDeleted,
			payload.AppointmentID,
			status,
			payload.StartTime,
			payload.EndTime,
			payload.SpecialistName,
			payload.ClientName,
		); err != nil {
			return fmt.Errorf("upsert appointment_calendar_events (deleted): %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
