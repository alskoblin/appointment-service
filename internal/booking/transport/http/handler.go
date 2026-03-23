package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"appointment-service/internal/apperr"
	"appointment-service/internal/booking/application"
	"appointment-service/internal/booking/domain"
	"appointment-service/internal/util"
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
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("GET /specialists", h.listSpecialists)
	mux.HandleFunc("POST /specialists", h.createSpecialist)
	mux.HandleFunc("POST /clients", h.createClient)
	mux.HandleFunc("POST /specialists/{id}/schedule", h.createSchedule)
	mux.HandleFunc("PUT /specialists/{id}/schedule", h.upsertSchedule)
	mux.HandleFunc("GET /specialists/{id}/schedule", h.getSchedule)
	mux.HandleFunc("GET /specialists/{id}/slots", h.getFreeSlots)
	mux.HandleFunc("POST /appointments", h.createAppointment)
	mux.HandleFunc("PATCH /appointments/{id}/reschedule", h.rescheduleAppointment)
	mux.HandleFunc("PATCH /appointments/{id}/cancel", h.cancelAppointment)
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	result, err := h.svc.Register(r.Context(), application.RegisterInput{
		Email:               req.Email,
		Password:            req.Password,
		Role:                req.Role,
		FullName:            req.FullName,
		Phone:               req.Phone,
		TelegramChatID:      req.TelegramChatID,
		Profession:          req.Profession,
		SlotDurationMinutes: req.SlotDurationMinutes,
		Timezone:            req.Timezone,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toAuthResponse(result))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	result, err := h.svc.Login(r.Context(), application.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toAuthResponse(result))
}

func (h *Handler) listSpecialists(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAnyRole(w, r, domain.RoleAdmin, domain.RoleClient, domain.RoleSpecialist); !ok {
		return
	}

	items, err := h.svc.ListSpecialists(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}

	out := make([]specialistDTO, 0, len(items))
	for _, s := range items {
		out = append(out, specialistDTO{ID: s.ID, FullName: s.FullName, Profession: s.Profession, SlotDurationMinutes: s.SlotDurationMinutes, Timezone: s.Timezone})
	}

	writeJSON(w, http.StatusOK, map[string]any{"specialists": out})
}

func (h *Handler) createSpecialist(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAnyRole(w, r, domain.RoleAdmin); !ok {
		return
	}

	var req createSpecialistRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	created, err := h.svc.CreateSpecialist(r.Context(), application.CreateSpecialistInput{
		FullName:            req.FullName,
		Profession:          req.Profession,
		SlotDurationMinutes: req.SlotDurationMinutes,
		Timezone:            req.Timezone,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"specialist": specialistDTO{
			ID:                  created.ID,
			FullName:            created.FullName,
			Profession:          created.Profession,
			SlotDurationMinutes: created.SlotDurationMinutes,
			Timezone:            created.Timezone,
		},
	})
}

func (h *Handler) createClient(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAnyRole(w, r, domain.RoleAdmin); !ok {
		return
	}

	var req createClientRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	created, err := h.svc.CreateClient(r.Context(), application.CreateClientInput{
		FullName:       req.FullName,
		Phone:          req.Phone,
		TelegramChatID: req.TelegramChatID,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"client": toClientDTO(created)})
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	specialistID, err := pathInt64(r, "id")
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}
	if _, ok := h.requireSpecialistOrAdmin(w, r, specialistID); !ok {
		return
	}

	input, err := parseSaveScheduleInput(specialistID, r)
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	saved, err := h.svc.CreateSchedule(r.Context(), input)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"schedule": toScheduleDTO(saved)})
}

func (h *Handler) upsertSchedule(w http.ResponseWriter, r *http.Request) {
	specialistID, err := pathInt64(r, "id")
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}
	if _, ok := h.requireSpecialistOrAdmin(w, r, specialistID); !ok {
		return
	}

	input, err := parseSaveScheduleInput(specialistID, r)
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	saved, err := h.svc.UpsertSchedule(r.Context(), input)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedule": toScheduleDTO(saved)})
}

func (h *Handler) getSchedule(w http.ResponseWriter, r *http.Request) {
	specialistID, err := pathInt64(r, "id")
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}
	if _, ok := h.requireSpecialistOrAdmin(w, r, specialistID); !ok {
		return
	}
	date, err := parseDateQuery(r)
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	view, err := h.svc.GetSpecialistSchedule(r.Context(), specialistID, date)
	if err != nil {
		h.writeError(w, err)
		return
	}

	appointments := make([]appointmentDTO, 0, len(view.Appointments))
	for _, a := range view.Appointments {
		appointments = append(appointments, toAppointmentDTO(a))
	}

	resp := scheduleResponse{
		SpecialistID: view.Specialist.ID,
		Date:         view.Schedule.WorkDate.Format(util.DateLayout),
		IsDayOff:     view.Schedule.IsDayOff,
		WorkingHours: workingHoursDTO{
			Start:      util.MinuteToHHMM(view.Schedule.StartMinute),
			End:        util.MinuteToHHMM(view.Schedule.EndMinute),
			BreakStart: minuteToOptionalHHMM(view.Schedule.BreakStartMinute),
			BreakEnd:   minuteToOptionalHHMM(view.Schedule.BreakEndMinute),
		},
		Appointments: appointments,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getFreeSlots(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAnyRole(w, r, domain.RoleAdmin, domain.RoleClient, domain.RoleSpecialist); !ok {
		return
	}

	specialistID, err := pathInt64(r, "id")
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}
	date, err := parseDateQuery(r)
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	view, err := h.svc.GetFreeSlots(r.Context(), specialistID, date)
	if err != nil {
		h.writeError(w, err)
		return
	}

	slots := make([]timeSlotDTO, 0, len(view.FreeSlots))
	for _, slot := range view.FreeSlots {
		slots = append(slots, timeSlotDTO{StartTime: slot.Start.Format(time.RFC3339), EndTime: slot.End.Format(time.RFC3339)})
	}

	writeJSON(w, http.StatusOK, freeSlotsResponse{
		SpecialistID:        view.SpecialistID,
		Date:                view.Date.Format(util.DateLayout),
		SlotDurationMinutes: view.SlotDurationMinutes,
		FreeSlots:           slots,
	})
}

func (h *Handler) createAppointment(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.requireAnyRole(w, r, domain.RoleAdmin, domain.RoleClient)
	if !ok {
		return
	}

	var req createAppointmentRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	start, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		h.writeError(w, apperr.Validation("start_time must be in RFC3339 format"))
		return
	}
	if identity.Role == domain.RoleClient {
		if identity.ClientID == nil || *identity.ClientID != req.ClientID {
			h.writeError(w, apperr.Forbidden("client can create appointments only for own account"))
			return
		}
	}

	appt, err := h.svc.CreateAppointment(r.Context(), application.CreateAppointmentInput{SpecialistID: req.SpecialistID, ClientID: req.ClientID, StartTime: start})
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"appointment": toAppointmentDTO(appt)})
}

func (h *Handler) rescheduleAppointment(w http.ResponseWriter, r *http.Request) {
	appointmentID, err := pathInt64(r, "id")
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	identity, ok := h.requireAnyRole(w, r, domain.RoleAdmin, domain.RoleClient, domain.RoleSpecialist)
	if !ok {
		return
	}
	if allowed := h.authorizeAppointmentAccess(r.Context(), identity, appointmentID, w); !allowed {
		return
	}

	var req rescheduleAppointmentRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	newStart, err := time.Parse(time.RFC3339, req.NewStartTime)
	if err != nil {
		h.writeError(w, apperr.Validation("new_start_time must be in RFC3339 format"))
		return
	}

	appt, err := h.svc.RescheduleAppointment(r.Context(), appointmentID, newStart)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"appointment": toAppointmentDTO(appt)})
}

func (h *Handler) cancelAppointment(w http.ResponseWriter, r *http.Request) {
	appointmentID, err := pathInt64(r, "id")
	if err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	identity, ok := h.requireAnyRole(w, r, domain.RoleAdmin, domain.RoleClient, domain.RoleSpecialist)
	if !ok {
		return
	}
	if allowed := h.authorizeAppointmentAccess(r.Context(), identity, appointmentID, w); !allowed {
		return
	}

	var req cancelAppointmentRequest
	if err := decodeJSON(r, &req); err != nil {
		h.writeError(w, apperr.Validation(err.Error()))
		return
	}

	appt, err := h.svc.CancelAppointment(r.Context(), appointmentID, req.Reason)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"appointment": toAppointmentDTO(appt)})
}

func (h *Handler) requireAnyRole(w http.ResponseWriter, r *http.Request, roles ...domain.UserRole) (domain.Identity, bool) {
	identity, ok := identityFromContext(r.Context())
	if !ok {
		h.writeError(w, apperr.Unauthorized("authentication required"))
		return domain.Identity{}, false
	}

	for _, role := range roles {
		if identity.Role == role {
			return identity, true
		}
	}

	h.writeError(w, apperr.Forbidden("insufficient role for this operation"))
	return domain.Identity{}, false
}

func (h *Handler) requireSpecialistOrAdmin(w http.ResponseWriter, r *http.Request, specialistID int64) (domain.Identity, bool) {
	identity, ok := h.requireAnyRole(w, r, domain.RoleAdmin, domain.RoleSpecialist)
	if !ok {
		return domain.Identity{}, false
	}
	if identity.Role == domain.RoleAdmin {
		return identity, true
	}
	if identity.SpecialistID == nil || *identity.SpecialistID != specialistID {
		h.writeError(w, apperr.Forbidden("specialist can manage only own schedule"))
		return domain.Identity{}, false
	}
	return identity, true
}

func (h *Handler) authorizeAppointmentAccess(ctx context.Context, identity domain.Identity, appointmentID int64, w http.ResponseWriter) bool {
	if identity.Role == domain.RoleAdmin {
		return true
	}

	appointment, err := h.svc.GetAppointmentByID(ctx, appointmentID)
	if err != nil {
		h.writeError(w, err)
		return false
	}

	switch identity.Role {
	case domain.RoleClient:
		if identity.ClientID == nil || *identity.ClientID != appointment.ClientID {
			h.writeError(w, apperr.Forbidden("client can manage only own appointments"))
			return false
		}
	case domain.RoleSpecialist:
		if identity.SpecialistID == nil || *identity.SpecialistID != appointment.SpecialistID {
			h.writeError(w, apperr.Forbidden("specialist can manage only own appointments"))
			return false
		}
	default:
		h.writeError(w, apperr.Forbidden("insufficient role for this operation"))
		return false
	}
	return true
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
	case apperr.CodeUnauthorized:
		return http.StatusUnauthorized
	case apperr.CodeForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func parseDateQuery(r *http.Request) (time.Time, error) {
	value := r.URL.Query().Get("date")
	if value == "" {
		return time.Time{}, fmt.Errorf("query parameter date is required")
	}
	return util.ParseDate(value)
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

func parseSaveScheduleInput(specialistID int64, r *http.Request) (application.SaveScheduleInput, error) {
	var req saveScheduleRequest
	if err := decodeJSON(r, &req); err != nil {
		return application.SaveScheduleInput{}, err
	}

	workDate, err := util.ParseDate(req.Date)
	if err != nil {
		return application.SaveScheduleInput{}, err
	}
	startMinute, err := util.ParseHHMM(req.Start)
	if err != nil {
		return application.SaveScheduleInput{}, fmt.Errorf("start: %w", err)
	}
	endMinute, err := util.ParseHHMM(req.End)
	if err != nil {
		return application.SaveScheduleInput{}, fmt.Errorf("end: %w", err)
	}

	if (req.BreakStart == nil) != (req.BreakEnd == nil) {
		return application.SaveScheduleInput{}, fmt.Errorf("break_start and break_end must be provided together")
	}

	breakStart, err := parseOptionalHHMM(req.BreakStart, "break_start")
	if err != nil {
		return application.SaveScheduleInput{}, err
	}
	breakEnd, err := parseOptionalHHMM(req.BreakEnd, "break_end")
	if err != nil {
		return application.SaveScheduleInput{}, err
	}

	return application.SaveScheduleInput{
		SpecialistID:     specialistID,
		WorkDate:         workDate,
		StartMinute:      startMinute,
		EndMinute:        endMinute,
		BreakStartMinute: breakStart,
		BreakEndMinute:   breakEnd,
		IsDayOff:         req.IsDayOff,
	}, nil
}

func parseOptionalHHMM(value *string, field string) (*int, error) {
	if value == nil {
		return nil, nil
	}
	minute, err := util.ParseHHMM(*value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	return &minute, nil
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid json body: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("json body must contain a single object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func minuteToOptionalHHMM(value *int) *string {
	if value == nil {
		return nil
	}
	formatted := util.MinuteToHHMM(*value)
	return &formatted
}

func toAppointmentDTO(a domain.Appointment) appointmentDTO {
	return appointmentDTO{
		ID:           a.ID,
		SpecialistID: a.SpecialistID,
		ClientID:     a.ClientID,
		StartTime:    a.StartTime.Format(time.RFC3339),
		EndTime:      a.EndTime.Format(time.RFC3339),
		Status:       string(a.Status),
		CancelReason: a.CancelReason,
		CanceledAt:   optionalRFC3339(a.CanceledAt),
		CreatedAt:    a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    a.UpdatedAt.Format(time.RFC3339),
	}
}

func optionalRFC3339(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.Format(time.RFC3339)
	return &v
}

func toClientDTO(c domain.Client) clientDTO {
	return clientDTO{
		ID:             c.ID,
		FullName:       c.FullName,
		Phone:          c.Phone,
		TelegramChatID: c.TelegramChatID,
		CreatedAt:      c.CreatedAt.Format(time.RFC3339),
	}
}

func toScheduleDTO(s domain.Schedule) scheduleDTO {
	return scheduleDTO{
		SpecialistID: s.SpecialistID,
		Date:         s.WorkDate.Format(util.DateLayout),
		IsDayOff:     s.IsDayOff,
		WorkingHours: workingHoursDTO{
			Start:      util.MinuteToHHMM(s.StartMinute),
			End:        util.MinuteToHHMM(s.EndMinute),
			BreakStart: minuteToOptionalHHMM(s.BreakStartMinute),
			BreakEnd:   minuteToOptionalHHMM(s.BreakEndMinute),
		},
	}
}

func toAuthResponse(result application.AuthResult) authResponse {
	return authResponse{
		AccessToken: result.AccessToken,
		TokenType:   result.TokenType,
		ExpiresAt:   result.ExpiresAt.Format(time.RFC3339),
		User: authUserDTO{
			ID:           result.User.ID,
			Email:        result.User.Email,
			Role:         string(result.User.Role),
			ClientID:     result.User.ClientID,
			SpecialistID: result.User.SpecialistID,
		},
	}
}

type specialistDTO struct {
	ID                  int64  `json:"id"`
	FullName            string `json:"full_name"`
	Profession          string `json:"profession"`
	SlotDurationMinutes int    `json:"slot_duration_minutes"`
	Timezone            string `json:"timezone"`
}

type clientDTO struct {
	ID             int64  `json:"id"`
	FullName       string `json:"full_name"`
	Phone          string `json:"phone"`
	TelegramChatID *int64 `json:"telegram_chat_id,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type appointmentDTO struct {
	ID           int64   `json:"id"`
	SpecialistID int64   `json:"specialist_id"`
	ClientID     int64   `json:"client_id"`
	StartTime    string  `json:"start_time"`
	EndTime      string  `json:"end_time"`
	Status       string  `json:"status"`
	CancelReason *string `json:"cancel_reason,omitempty"`
	CanceledAt   *string `json:"canceled_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type workingHoursDTO struct {
	Start      string  `json:"start"`
	End        string  `json:"end"`
	BreakStart *string `json:"break_start,omitempty"`
	BreakEnd   *string `json:"break_end,omitempty"`
}

type scheduleResponse struct {
	SpecialistID int64            `json:"specialist_id"`
	Date         string           `json:"date"`
	IsDayOff     bool             `json:"is_day_off"`
	WorkingHours workingHoursDTO  `json:"working_hours"`
	Appointments []appointmentDTO `json:"appointments"`
}

type timeSlotDTO struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type freeSlotsResponse struct {
	SpecialistID        int64         `json:"specialist_id"`
	Date                string        `json:"date"`
	SlotDurationMinutes int           `json:"slot_duration_minutes"`
	FreeSlots           []timeSlotDTO `json:"free_slots"`
}

type scheduleDTO struct {
	SpecialistID int64           `json:"specialist_id"`
	Date         string          `json:"date"`
	IsDayOff     bool            `json:"is_day_off"`
	WorkingHours workingHoursDTO `json:"working_hours"`
}

type createSpecialistRequest struct {
	FullName            string `json:"full_name"`
	Profession          string `json:"profession"`
	SlotDurationMinutes int    `json:"slot_duration_minutes"`
	Timezone            string `json:"timezone"`
}

type registerRequest struct {
	Email               string `json:"email"`
	Password            string `json:"password"`
	Role                string `json:"role"`
	FullName            string `json:"full_name"`
	Phone               string `json:"phone"`
	TelegramChatID      *int64 `json:"telegram_chat_id,omitempty"`
	Profession          string `json:"profession"`
	SlotDurationMinutes int    `json:"slot_duration_minutes"`
	Timezone            string `json:"timezone"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type saveScheduleRequest struct {
	Date       string  `json:"date"`
	Start      string  `json:"start"`
	End        string  `json:"end"`
	BreakStart *string `json:"break_start,omitempty"`
	BreakEnd   *string `json:"break_end,omitempty"`
	IsDayOff   bool    `json:"is_day_off"`
}

type createClientRequest struct {
	FullName       string `json:"full_name"`
	Phone          string `json:"phone"`
	TelegramChatID *int64 `json:"telegram_chat_id,omitempty"`
}

type createAppointmentRequest struct {
	SpecialistID int64  `json:"specialist_id"`
	ClientID     int64  `json:"client_id"`
	StartTime    string `json:"start_time"`
}

type rescheduleAppointmentRequest struct {
	NewStartTime string `json:"new_start_time"`
}

type cancelAppointmentRequest struct {
	Reason string `json:"reason"`
}

type authUserDTO struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	ClientID     *int64 `json:"client_id,omitempty"`
	SpecialistID *int64 `json:"specialist_id,omitempty"`
}

type authResponse struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	ExpiresAt   string      `json:"expires_at"`
	User        authUserDTO `json:"user"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error apiError `json:"error"`
}
