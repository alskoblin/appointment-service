package postgres

import (
	"context"
	"fmt"

	"appointment-service/internal/notifier/domain"

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

func (r *Repository) MarkProcessedAndLog(ctx context.Context, payload domain.AppointmentEventPayload, status domain.NotificationStatus, errorText *string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	const insertProcessed = `
		INSERT INTO processed_events (event_key, event_type)
		VALUES ($1, $2)
		ON CONFLICT (event_key) DO NOTHING`
	if _, err := tx.Exec(ctx, insertProcessed, payload.EventKey, payload.EventType); err != nil {
		return fmt.Errorf("insert processed event: %w", err)
	}

	const insertLog = `
		INSERT INTO notification_logs (event_key, event_type, appointment_id, client_id, telegram_chat_id, status, error_text, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $6 = 'sent' THEN now() ELSE NULL END)`
	if _, err := tx.Exec(ctx, insertLog, payload.EventKey, payload.EventType, payload.AppointmentID, payload.ClientID, payload.TelegramChatID, status, errorText); err != nil {
		return fmt.Errorf("insert notification log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
