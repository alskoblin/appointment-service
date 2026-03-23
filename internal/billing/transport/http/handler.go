package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"appointment-service/internal/apperr"
	"appointment-service/internal/billing/application"
	"appointment-service/internal/billing/domain"
)

type Handler struct {
	svc    *application.Service
	logger *slog.Logger
}

func New(svc *application.Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /billing/invoices/{appointment_id}", h.getInvoice)
	mux.HandleFunc("POST /billing/invoices/{appointment_id}/pay", h.payInvoice)
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) getInvoice(w http.ResponseWriter, r *http.Request) {
	appointmentID, err := pathInt64(r, "appointment_id")
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	invoice, err := h.svc.GetInvoice(r.Context(), appointmentID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"invoice": toInvoiceDTO(invoice)})
}

func (h *Handler) payInvoice(w http.ResponseWriter, r *http.Request) {
	appointmentID, err := pathInt64(r, "appointment_id")
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	invoice, err := h.svc.PayInvoice(r.Context(), appointmentID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"invoice": toInvoiceDTO(invoice)})
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		status := httpStatusByCode(appErr.Code)
		message := appErr.Message
		if appErr.Code == apperr.CodeInternal {
			message = "internal server error"
			h.logger.Error("internal error", "error", err)
		}
		writeJSON(w, status, errorResponse{Error: apiError{Code: string(appErr.Code), Message: message}})
		return
	}

	h.logger.Error("untyped error", "error", err)
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: apiError{Code: string(apperr.CodeInternal), Message: "internal server error"}})
}

func httpStatusByCode(code apperr.Code) int {
	switch code {
	case apperr.CodeValidation:
		return http.StatusBadRequest
	case apperr.CodeNotFound:
		return http.StatusNotFound
	case apperr.CodeConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func pathInt64(r *http.Request, field string) (int64, error) {
	value := r.PathValue(field)
	if value == "" {
		return 0, fmt.Errorf("path parameter %s is required", field)
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("path parameter %s must be positive int64", field)
	}
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func toInvoiceDTO(invoice domain.Invoice) invoiceDTO {
	return invoiceDTO{
		AppointmentID:     invoice.AppointmentID,
		SpecialistID:      invoice.SpecialistID,
		SpecialistName:    invoice.SpecialistName,
		ClientID:          invoice.ClientID,
		ClientName:        invoice.ClientName,
		StartTime:         invoice.StartTime.Format(time.RFC3339),
		EndTime:           invoice.EndTime.Format(time.RFC3339),
		AmountCents:       invoice.AmountCents,
		Currency:          invoice.Currency,
		Status:            string(invoice.Status),
		ProviderPaymentID: invoice.ProviderPaymentID,
		ProviderRefundID:  invoice.ProviderRefundID,
		LastError:         invoice.LastError,
		CreatedAt:         invoice.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         invoice.UpdatedAt.Format(time.RFC3339),
	}
}

type invoiceDTO struct {
	AppointmentID     int64   `json:"appointment_id"`
	SpecialistID      int64   `json:"specialist_id"`
	SpecialistName    string  `json:"specialist_name"`
	ClientID          int64   `json:"client_id"`
	ClientName        string  `json:"client_name"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	AmountCents       int64   `json:"amount_cents"`
	Currency          string  `json:"currency"`
	Status            string  `json:"status"`
	ProviderPaymentID *string `json:"provider_payment_id,omitempty"`
	ProviderRefundID  *string `json:"provider_refund_id,omitempty"`
	LastError         *string `json:"last_error,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error apiError `json:"error"`
}
