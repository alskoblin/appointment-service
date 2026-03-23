package application

import (
	"context"
	"fmt"

	"appointment-service/internal/apperr"
	"appointment-service/internal/billing/domain"
)

type Service struct {
	repo     Repository
	payments PaymentGateway
}

func NewService(repo Repository, payments PaymentGateway) *Service {
	return &Service{repo: repo, payments: payments}
}

func (s *Service) GetInvoice(ctx context.Context, appointmentID int64) (domain.Invoice, error) {
	if appointmentID <= 0 {
		return domain.Invoice{}, apperr.Validation("appointment_id must be positive")
	}
	return s.repo.GetInvoiceByAppointmentID(ctx, appointmentID)
}

func (s *Service) PayInvoice(ctx context.Context, appointmentID int64) (domain.Invoice, error) {
	if appointmentID <= 0 {
		return domain.Invoice{}, apperr.Validation("appointment_id must be positive")
	}

	invoice, err := s.repo.GetInvoiceByAppointmentID(ctx, appointmentID)
	if err != nil {
		return domain.Invoice{}, err
	}

	switch invoice.Status {
	case domain.InvoiceStatusPaid:
		return invoice, nil
	case domain.InvoiceStatusRefunded:
		return domain.Invoice{}, apperr.Conflict("invoice already refunded")
	case domain.InvoiceStatusCanceled:
		return domain.Invoice{}, apperr.Conflict("cannot pay canceled invoice")
	}

	if !s.payments.Enabled() {
		return domain.Invoice{}, apperr.Conflict("payment gateway is disabled")
	}

	paymentID, err := s.payments.Charge(ctx, ChargeRequest{
		IdempotencyKey: fmt.Sprintf("invoice.pay.%d", appointmentID),
		AmountCents:    invoice.AmountCents,
		Currency:       invoice.Currency,
		Description:    fmt.Sprintf("Payment for appointment %d", appointmentID),
	})
	if err != nil {
		_ = s.repo.MarkInvoicePaymentFailed(ctx, appointmentID, err.Error())
		return domain.Invoice{}, apperr.Conflict("payment failed")
	}

	paid, err := s.repo.MarkInvoicePaid(ctx, appointmentID, paymentID)
	if err != nil {
		return domain.Invoice{}, err
	}
	return paid, nil
}
