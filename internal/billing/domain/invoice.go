package domain

import "time"

type InvoiceStatus string

const (
	InvoiceStatusPending       InvoiceStatus = "pending"
	InvoiceStatusPaid          InvoiceStatus = "paid"
	InvoiceStatusCanceled      InvoiceStatus = "canceled"
	InvoiceStatusRefunded      InvoiceStatus = "refunded"
	InvoiceStatusPaymentFailed InvoiceStatus = "payment_failed"
	InvoiceStatusRefundFailed  InvoiceStatus = "refund_failed"
)

type Invoice struct {
	AppointmentID     int64         `json:"appointment_id"`
	SpecialistID      int64         `json:"specialist_id"`
	SpecialistName    string        `json:"specialist_name"`
	ClientID          int64         `json:"client_id"`
	ClientName        string        `json:"client_name"`
	StartTime         time.Time     `json:"start_time"`
	EndTime           time.Time     `json:"end_time"`
	AmountCents       int64         `json:"amount_cents"`
	Currency          string        `json:"currency"`
	Status            InvoiceStatus `json:"status"`
	ProviderPaymentID *string       `json:"provider_payment_id,omitempty"`
	ProviderRefundID  *string       `json:"provider_refund_id,omitempty"`
	LastError         *string       `json:"last_error,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

type LogStatus string

const (
	LogStatusSynced  LogStatus = "synced"
	LogStatusSkipped LogStatus = "skipped"
	LogStatusFailed  LogStatus = "failed"
)
