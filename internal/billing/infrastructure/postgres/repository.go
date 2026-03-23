package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"appointment-service/internal/apperr"
	"appointment-service/internal/billing/domain"

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

func (r *Repository) UpsertFromCreatedAndMarkProcessed(
	ctx context.Context,
	payload domain.AppointmentEventPayload,
	amountCents int64,
	currency string,
) (domain.Invoice, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Invoice{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	inserted, err := insertProcessed(ctx, tx, payload.EventKey, payload.EventType)
	if err != nil {
		return domain.Invoice{}, err
	}
	if !inserted {
		invoice, getErr := getInvoice(ctx, tx, payload.AppointmentID)
		if getErr != nil {
			if isNoRows(getErr) {
				return domain.Invoice{}, nil
			}
			return domain.Invoice{}, getErr
		}
		return invoice, nil
	}

	const q = `
		INSERT INTO invoices (
			appointment_id,
			specialist_id,
			specialist_name,
			client_id,
			client_name,
			start_time,
			end_time,
			amount_cents,
			currency,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
		ON CONFLICT (appointment_id) DO UPDATE
		SET specialist_id = EXCLUDED.specialist_id,
		    specialist_name = EXCLUDED.specialist_name,
		    client_id = EXCLUDED.client_id,
		    client_name = EXCLUDED.client_name,
		    start_time = EXCLUDED.start_time,
		    end_time = EXCLUDED.end_time,
		    amount_cents = CASE
		        WHEN invoices.status IN ('pending', 'payment_failed', 'canceled') THEN EXCLUDED.amount_cents
		        ELSE invoices.amount_cents
		    END,
		    currency = CASE
		        WHEN invoices.status IN ('pending', 'payment_failed', 'canceled') THEN EXCLUDED.currency
		        ELSE invoices.currency
		    END,
		    status = CASE
		        WHEN invoices.status IN ('pending', 'payment_failed', 'canceled') THEN 'pending'
		        ELSE invoices.status
		    END,
		    updated_at = now()
		RETURNING appointment_id, specialist_id, specialist_name, client_id, client_name, start_time, end_time, amount_cents, currency, status, provider_payment_id, provider_refund_id, last_error, created_at, updated_at`

	invoice, err := scanInvoice(tx.QueryRow(
		ctx,
		q,
		payload.AppointmentID,
		payload.SpecialistID,
		payload.SpecialistName,
		payload.ClientID,
		payload.ClientName,
		payload.StartTime,
		payload.EndTime,
		amountCents,
		strings.ToUpper(strings.TrimSpace(currency)),
	))
	if err != nil {
		return domain.Invoice{}, fmt.Errorf("upsert invoice on created: %w", err)
	}

	if err := insertLog(ctx, tx, payload, "invoice_upsert_created", domain.LogStatusSynced, nil); err != nil {
		return domain.Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Invoice{}, fmt.Errorf("commit tx: %w", err)
	}
	return invoice, nil
}

func (r *Repository) UpsertFromRescheduledAndMarkProcessed(ctx context.Context, payload domain.AppointmentEventPayload) (domain.Invoice, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Invoice{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	inserted, err := insertProcessed(ctx, tx, payload.EventKey, payload.EventType)
	if err != nil {
		return domain.Invoice{}, err
	}
	if !inserted {
		invoice, getErr := getInvoice(ctx, tx, payload.AppointmentID)
		if getErr != nil {
			if isNoRows(getErr) {
				return domain.Invoice{}, nil
			}
			return domain.Invoice{}, getErr
		}
		return invoice, nil
	}

	const q = `
		UPDATE invoices
		SET specialist_id = $2,
		    specialist_name = $3,
		    client_id = $4,
		    client_name = $5,
		    start_time = $6,
		    end_time = $7,
		    updated_at = now()
		WHERE appointment_id = $1
		RETURNING appointment_id, specialist_id, specialist_name, client_id, client_name, start_time, end_time, amount_cents, currency, status, provider_payment_id, provider_refund_id, last_error, created_at, updated_at`

	invoice, err := scanInvoice(tx.QueryRow(
		ctx,
		q,
		payload.AppointmentID,
		payload.SpecialistID,
		payload.SpecialistName,
		payload.ClientID,
		payload.ClientName,
		payload.StartTime,
		payload.EndTime,
	))
	if err != nil {
		if isNoRows(err) {
			msg := "invoice not found on reschedule event"
			if logErr := insertLog(ctx, tx, payload, "invoice_upsert_reschedule", domain.LogStatusSkipped, &msg); logErr != nil {
				return domain.Invoice{}, logErr
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return domain.Invoice{}, fmt.Errorf("commit tx: %w", commitErr)
			}
			return domain.Invoice{}, nil
		}
		return domain.Invoice{}, fmt.Errorf("update invoice on reschedule: %w", err)
	}

	if err := insertLog(ctx, tx, payload, "invoice_upsert_reschedule", domain.LogStatusSynced, nil); err != nil {
		return domain.Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Invoice{}, fmt.Errorf("commit tx: %w", err)
	}
	return invoice, nil
}

func (r *Repository) MarkCanceledAndProcessed(ctx context.Context, payload domain.AppointmentEventPayload) (domain.Invoice, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Invoice{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	inserted, err := insertProcessed(ctx, tx, payload.EventKey, payload.EventType)
	if err != nil {
		return domain.Invoice{}, err
	}
	if !inserted {
		invoice, getErr := getInvoice(ctx, tx, payload.AppointmentID)
		if getErr != nil {
			if isNoRows(getErr) {
				return domain.Invoice{}, nil
			}
			return domain.Invoice{}, getErr
		}
		return invoice, nil
	}

	const q = `
		UPDATE invoices
		SET status = CASE
		        WHEN status = 'paid' THEN status
		        WHEN status = 'refunded' THEN status
		        ELSE 'canceled'
		    END,
		    specialist_name = $2,
		    client_name = $3,
		    start_time = $4,
		    end_time = $5,
		    updated_at = now()
		WHERE appointment_id = $1
		RETURNING appointment_id, specialist_id, specialist_name, client_id, client_name, start_time, end_time, amount_cents, currency, status, provider_payment_id, provider_refund_id, last_error, created_at, updated_at`

	invoice, err := scanInvoice(tx.QueryRow(
		ctx,
		q,
		payload.AppointmentID,
		payload.SpecialistName,
		payload.ClientName,
		payload.StartTime,
		payload.EndTime,
	))
	if err != nil {
		if isNoRows(err) {
			msg := "invoice not found on cancel event"
			if logErr := insertLog(ctx, tx, payload, "invoice_cancel", domain.LogStatusSkipped, &msg); logErr != nil {
				return domain.Invoice{}, logErr
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return domain.Invoice{}, fmt.Errorf("commit tx: %w", commitErr)
			}
			return domain.Invoice{}, nil
		}
		return domain.Invoice{}, fmt.Errorf("update invoice on cancel: %w", err)
	}

	if err := insertLog(ctx, tx, payload, "invoice_cancel", domain.LogStatusSynced, nil); err != nil {
		return domain.Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Invoice{}, fmt.Errorf("commit tx: %w", err)
	}
	return invoice, nil
}

func (r *Repository) MarkRefundedAndProcessed(ctx context.Context, payload domain.AppointmentEventPayload, refundID string) (domain.Invoice, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Invoice{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	inserted, err := insertProcessed(ctx, tx, payload.EventKey, payload.EventType)
	if err != nil {
		return domain.Invoice{}, err
	}
	if !inserted {
		invoice, getErr := getInvoice(ctx, tx, payload.AppointmentID)
		if getErr != nil {
			if isNoRows(getErr) {
				return domain.Invoice{}, nil
			}
			return domain.Invoice{}, getErr
		}
		return invoice, nil
	}

	const q = `
		UPDATE invoices
		SET status = 'refunded',
		    provider_refund_id = $2,
		    last_error = NULL,
		    specialist_name = $3,
		    client_name = $4,
		    start_time = $5,
		    end_time = $6,
		    updated_at = now()
		WHERE appointment_id = $1
		RETURNING appointment_id, specialist_id, specialist_name, client_id, client_name, start_time, end_time, amount_cents, currency, status, provider_payment_id, provider_refund_id, last_error, created_at, updated_at`

	invoice, err := scanInvoice(tx.QueryRow(
		ctx,
		q,
		payload.AppointmentID,
		refundID,
		payload.SpecialistName,
		payload.ClientName,
		payload.StartTime,
		payload.EndTime,
	))
	if err != nil {
		if isNoRows(err) {
			msg := "invoice not found on refund"
			if logErr := insertLog(ctx, tx, payload, "invoice_refund", domain.LogStatusSkipped, &msg); logErr != nil {
				return domain.Invoice{}, logErr
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return domain.Invoice{}, fmt.Errorf("commit tx: %w", commitErr)
			}
			return domain.Invoice{}, nil
		}
		return domain.Invoice{}, fmt.Errorf("update invoice on refund: %w", err)
	}

	if err := insertLog(ctx, tx, payload, "invoice_refund", domain.LogStatusSynced, nil); err != nil {
		return domain.Invoice{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Invoice{}, fmt.Errorf("commit tx: %w", err)
	}
	return invoice, nil
}

func (r *Repository) MarkRefundFailedAndProcessed(ctx context.Context, payload domain.AppointmentEventPayload, errorText string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	inserted, err := insertProcessed(ctx, tx, payload.EventKey, payload.EventType)
	if err != nil {
		return err
	}
	if !inserted {
		return nil
	}

	const q = `
		UPDATE invoices
		SET status = 'refund_failed',
		    last_error = LEFT($2, 1000),
		    specialist_name = $3,
		    client_name = $4,
		    start_time = $5,
		    end_time = $6,
		    updated_at = now()
		WHERE appointment_id = $1`
	_, err = tx.Exec(
		ctx,
		q,
		payload.AppointmentID,
		errorText,
		payload.SpecialistName,
		payload.ClientName,
		payload.StartTime,
		payload.EndTime,
	)
	if err != nil {
		return fmt.Errorf("update invoice on refund failed: %w", err)
	}

	errText := errorText
	if err := insertLog(ctx, tx, payload, "invoice_refund", domain.LogStatusFailed, &errText); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *Repository) MarkProcessedOnly(ctx context.Context, payload domain.AppointmentEventPayload, status domain.LogStatus, errorText *string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	inserted, err := insertProcessed(ctx, tx, payload.EventKey, payload.EventType)
	if err != nil {
		return err
	}
	if !inserted {
		return nil
	}

	if err := insertLog(ctx, tx, payload, "event_"+payload.EventType, status, errorText); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *Repository) GetInvoiceByAppointmentID(ctx context.Context, appointmentID int64) (domain.Invoice, error) {
	invoice, err := scanInvoice(r.db.QueryRow(
		ctx,
		`SELECT appointment_id, specialist_id, specialist_name, client_id, client_name, start_time, end_time, amount_cents, currency, status, provider_payment_id, provider_refund_id, last_error, created_at, updated_at
		 FROM invoices WHERE appointment_id = $1`,
		appointmentID,
	))
	if err != nil {
		if isNoRows(err) {
			return domain.Invoice{}, apperr.NotFound("invoice not found")
		}
		return domain.Invoice{}, apperr.Internal("failed to get invoice", err)
	}
	return invoice, nil
}

func (r *Repository) MarkInvoicePaid(ctx context.Context, appointmentID int64, paymentID string) (domain.Invoice, error) {
	const q = `
		UPDATE invoices
		SET status = 'paid',
		    provider_payment_id = $2,
		    last_error = NULL,
		    updated_at = now()
		WHERE appointment_id = $1
		  AND status IN ('pending', 'payment_failed')
		RETURNING appointment_id, specialist_id, specialist_name, client_id, client_name, start_time, end_time, amount_cents, currency, status, provider_payment_id, provider_refund_id, last_error, created_at, updated_at`

	invoice, err := scanInvoice(r.db.QueryRow(ctx, q, appointmentID, paymentID))
	if err == nil {
		return invoice, nil
	}
	if !isNoRows(err) {
		return domain.Invoice{}, apperr.Internal("failed to mark invoice paid", err)
	}

	current, getErr := r.GetInvoiceByAppointmentID(ctx, appointmentID)
	if getErr != nil {
		return domain.Invoice{}, getErr
	}
	switch current.Status {
	case domain.InvoiceStatusPaid:
		return current, nil
	case domain.InvoiceStatusCanceled:
		return domain.Invoice{}, apperr.Conflict("cannot pay canceled invoice")
	case domain.InvoiceStatusRefunded:
		return domain.Invoice{}, apperr.Conflict("cannot pay refunded invoice")
	default:
		return domain.Invoice{}, apperr.Conflict("invoice cannot be paid in current status")
	}
}

func (r *Repository) MarkInvoicePaymentFailed(ctx context.Context, appointmentID int64, errorText string) error {
	const q = `
		UPDATE invoices
		SET status = 'payment_failed',
		    last_error = LEFT($2, 1000),
		    updated_at = now()
		WHERE appointment_id = $1
		  AND status IN ('pending', 'payment_failed')`
	_, err := r.db.Exec(ctx, q, appointmentID, errorText)
	if err != nil {
		return apperr.Internal("failed to mark payment as failed", err)
	}
	return nil
}

func insertProcessed(ctx context.Context, tx pgx.Tx, eventKey string, eventType string) (bool, error) {
	const q = `
		INSERT INTO processed_events (event_key, event_type)
		VALUES ($1, $2)
		ON CONFLICT (event_key) DO NOTHING`
	tag, err := tx.Exec(ctx, q, eventKey, eventType)
	if err != nil {
		return false, fmt.Errorf("insert processed event: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func insertLog(
	ctx context.Context,
	tx pgx.Tx,
	payload domain.AppointmentEventPayload,
	operation string,
	status domain.LogStatus,
	errorText *string,
) error {
	const q = `
		INSERT INTO billing_logs (event_key, event_type, appointment_id, operation, status, error_text)
		VALUES ($1, $2, $3, $4, $5, LEFT($6, 1000))`

	var errText any
	if errorText != nil {
		errText = *errorText
	}
	if _, err := tx.Exec(ctx, q, payload.EventKey, payload.EventType, payload.AppointmentID, operation, status, errText); err != nil {
		return fmt.Errorf("insert billing log: %w", err)
	}
	return nil
}

func getInvoice(ctx context.Context, tx pgx.Tx, appointmentID int64) (domain.Invoice, error) {
	return scanInvoice(tx.QueryRow(
		ctx,
		`SELECT appointment_id, specialist_id, specialist_name, client_id, client_name, start_time, end_time, amount_cents, currency, status, provider_payment_id, provider_refund_id, last_error, created_at, updated_at
		 FROM invoices WHERE appointment_id = $1`,
		appointmentID,
	))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanInvoice(s scanner) (domain.Invoice, error) {
	var invoice domain.Invoice
	err := s.Scan(
		&invoice.AppointmentID,
		&invoice.SpecialistID,
		&invoice.SpecialistName,
		&invoice.ClientID,
		&invoice.ClientName,
		&invoice.StartTime,
		&invoice.EndTime,
		&invoice.AmountCents,
		&invoice.Currency,
		&invoice.Status,
		&invoice.ProviderPaymentID,
		&invoice.ProviderRefundID,
		&invoice.LastError,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
	)
	return invoice, err
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
