package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"appointment-service/internal/apperr"
	"appointment-service/internal/billing/domain"
)

type Processor struct {
	repo         Repository
	payments     PaymentGateway
	logger       *slog.Logger
	defaultCents int64
	currency     string
}

func NewProcessor(
	repo Repository,
	payments PaymentGateway,
	logger *slog.Logger,
	defaultCents int64,
	currency string,
) *Processor {
	return &Processor{
		repo:         repo,
		payments:     payments,
		logger:       logger,
		defaultCents: defaultCents,
		currency:     currency,
	}
}

func (p *Processor) Process(ctx context.Context, raw []byte) error {
	var payload domain.AppointmentEventPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("unmarshal appointment event: %w", err)
	}
	if payload.EventKey == "" {
		return fmt.Errorf("event_key is empty")
	}

	processed, err := p.repo.IsProcessed(ctx, payload.EventKey)
	if err != nil {
		return err
	}
	if processed {
		return nil
	}

	switch payload.EventType {
	case domain.AppointmentEventCreated:
		_, err = p.repo.UpsertFromCreatedAndMarkProcessed(ctx, payload, p.defaultCents, p.currency)
		return err
	case domain.AppointmentEventReschedule:
		_, err = p.repo.UpsertFromRescheduledAndMarkProcessed(ctx, payload)
		return err
	case domain.AppointmentEventCanceled:
		return p.handleCancel(ctx, payload)
	default:
		msg := "event type is not relevant for billing"
		return p.repo.MarkProcessedOnly(ctx, payload, domain.LogStatusSkipped, &msg)
	}
}

func (p *Processor) handleCancel(ctx context.Context, payload domain.AppointmentEventPayload) error {
	invoice, err := p.repo.GetInvoiceByAppointmentID(ctx, payload.AppointmentID)
	if err != nil {
		if isNotFound(err) {
			msg := "invoice not found for appointment cancel event"
			return p.repo.MarkProcessedOnly(ctx, payload, domain.LogStatusSkipped, &msg)
		}
		return err
	}

	if invoice.Status != domain.InvoiceStatusPaid {
		_, err = p.repo.MarkCanceledAndProcessed(ctx, payload)
		return err
	}

	if !p.payments.Enabled() {
		msg := "payment gateway is disabled for refund"
		return p.repo.MarkRefundFailedAndProcessed(ctx, payload, msg)
	}
	if invoice.ProviderPaymentID == nil || *invoice.ProviderPaymentID == "" {
		msg := "missing provider_payment_id for refund"
		return p.repo.MarkRefundFailedAndProcessed(ctx, payload, msg)
	}

	refundID, refundErr := p.payments.Refund(ctx, RefundRequest{
		IdempotencyKey: payload.EventKey,
		PaymentID:      *invoice.ProviderPaymentID,
		AmountCents:    invoice.AmountCents,
		Currency:       invoice.Currency,
		Description:    fmt.Sprintf("Refund for appointment %d", payload.AppointmentID),
	})
	if refundErr != nil {
		errText := refundErr.Error()
		if markErr := p.repo.MarkRefundFailedAndProcessed(ctx, payload, errText); markErr != nil {
			return markErr
		}
		p.logger.Error("refund failed", "appointment_id", payload.AppointmentID, "error", refundErr)
		return nil
	}

	_, err = p.repo.MarkRefundedAndProcessed(ctx, payload, refundID)
	return err
}

func isNotFound(err error) bool {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Code == apperr.CodeNotFound
	}
	return false
}
