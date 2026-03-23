package application

import (
	"context"

	"appointment-service/internal/billing/domain"
)

type Repository interface {
	IsProcessed(ctx context.Context, eventKey string) (bool, error)
	UpsertFromCreatedAndMarkProcessed(ctx context.Context, payload domain.AppointmentEventPayload, amountCents int64, currency string) (domain.Invoice, error)
	UpsertFromRescheduledAndMarkProcessed(ctx context.Context, payload domain.AppointmentEventPayload) (domain.Invoice, error)
	MarkCanceledAndProcessed(ctx context.Context, payload domain.AppointmentEventPayload) (domain.Invoice, error)
	MarkRefundedAndProcessed(ctx context.Context, payload domain.AppointmentEventPayload, refundID string) (domain.Invoice, error)
	MarkRefundFailedAndProcessed(ctx context.Context, payload domain.AppointmentEventPayload, errorText string) error
	MarkProcessedOnly(ctx context.Context, payload domain.AppointmentEventPayload, status domain.LogStatus, errorText *string) error
	GetInvoiceByAppointmentID(ctx context.Context, appointmentID int64) (domain.Invoice, error)
	MarkInvoicePaid(ctx context.Context, appointmentID int64, paymentID string) (domain.Invoice, error)
	MarkInvoicePaymentFailed(ctx context.Context, appointmentID int64, errorText string) error
}

type PaymentGateway interface {
	Enabled() bool
	Charge(ctx context.Context, req ChargeRequest) (string, error)
	Refund(ctx context.Context, req RefundRequest) (string, error)
}

type ChargeRequest struct {
	IdempotencyKey string
	AmountCents    int64
	Currency       string
	Description    string
}

type RefundRequest struct {
	IdempotencyKey string
	PaymentID      string
	AmountCents    int64
	Currency       string
	Description    string
}
