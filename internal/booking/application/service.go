package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"appointment-service/internal/apperr"
	"appointment-service/internal/booking/auth"
	"appointment-service/internal/booking/domain"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo       Repository
	logger     *slog.Logger
	kafkaTopic string
	tokens     *auth.Manager
}

type ScheduleView struct {
	Specialist   domain.Specialist
	Schedule     domain.Schedule
	Appointments []domain.Appointment
}

type FreeSlotsView struct {
	SpecialistID        int64
	Date                time.Time
	SlotDurationMinutes int
	FreeSlots           []domain.TimeSlot
}

type CreateAppointmentInput struct {
	SpecialistID int64
	ClientID     int64
	StartTime    time.Time
}

type CreateSpecialistInput struct {
	FullName            string
	Profession          string
	SlotDurationMinutes int
	Timezone            string
}

type CreateClientInput struct {
	FullName       string
	Phone          string
	TelegramChatID *int64
}

type RegisterInput struct {
	Email               string
	Password            string
	Role                string
	FullName            string
	Phone               string
	TelegramChatID      *int64
	Profession          string
	SlotDurationMinutes int
	Timezone            string
}

type LoginInput struct {
	Email    string
	Password string
}

type AuthResult struct {
	AccessToken string
	TokenType   string
	ExpiresAt   time.Time
	User        AuthUser
}

type AuthUser struct {
	ID           int64
	Email        string
	Role         domain.UserRole
	ClientID     *int64
	SpecialistID *int64
}

type SaveScheduleInput struct {
	SpecialistID     int64
	WorkDate         time.Time
	StartMinute      int
	EndMinute        int
	BreakStartMinute *int
	BreakEndMinute   *int
	IsDayOff         bool
}

func New(repo Repository, logger *slog.Logger, kafkaTopic string, tokens *auth.Manager) *Service {
	return &Service{repo: repo, logger: logger, kafkaTopic: kafkaTopic, tokens: tokens}
}

func (s *Service) ListSpecialists(ctx context.Context) ([]domain.Specialist, error) {
	return s.repo.ListSpecialists(ctx)
}

func (s *Service) CreateSpecialist(ctx context.Context, input CreateSpecialistInput) (domain.Specialist, error) {
	specialist, err := buildSpecialist(input.FullName, input.Profession, input.SlotDurationMinutes, input.Timezone)
	if err != nil {
		return domain.Specialist{}, err
	}
	return s.repo.CreateSpecialist(ctx, specialist)
}

func (s *Service) CreateClient(ctx context.Context, input CreateClientInput) (domain.Client, error) {
	client, err := buildClient(input.FullName, input.Phone, input.TelegramChatID)
	if err != nil {
		return domain.Client{}, err
	}
	return s.repo.CreateClient(ctx, client)
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (AuthResult, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, err
	}
	password := strings.TrimSpace(input.Password)
	if len(password) < 8 {
		return AuthResult{}, apperr.Validation("password must be at least 8 characters")
	}
	role, err := domain.ParseUserRole(input.Role)
	if err != nil {
		return AuthResult{}, apperr.Validation(err.Error())
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, apperr.Internal("failed to hash password", err)
	}

	user := domain.User{
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
	}

	var created domain.User
	switch role {
	case domain.RoleAdmin:
		created, err = s.repo.CreateUser(ctx, user)
	case domain.RoleClient:
		client, buildErr := buildClient(input.FullName, input.Phone, input.TelegramChatID)
		if buildErr != nil {
			return AuthResult{}, buildErr
		}
		created, err = s.repo.CreateUserWithClient(ctx, user, client)
	case domain.RoleSpecialist:
		specialist, buildErr := buildSpecialist(input.FullName, input.Profession, input.SlotDurationMinutes, input.Timezone)
		if buildErr != nil {
			return AuthResult{}, buildErr
		}
		created, err = s.repo.CreateUserWithSpecialist(ctx, user, specialist)
	default:
		return AuthResult{}, apperr.Validation("unsupported role")
	}
	if err != nil {
		return AuthResult{}, err
	}
	return s.issueAuthResult(created)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (AuthResult, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, apperr.Unauthorized("invalid email or password")
	}
	password := strings.TrimSpace(input.Password)
	if password == "" {
		return AuthResult{}, apperr.Unauthorized("invalid email or password")
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if IsNotFound(err) {
			return AuthResult{}, apperr.Unauthorized("invalid email or password")
		}
		return AuthResult{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return AuthResult{}, apperr.Unauthorized("invalid email or password")
	}

	return s.issueAuthResult(user)
}

func (s *Service) CreateSchedule(ctx context.Context, input SaveScheduleInput) (domain.Schedule, error) {
	if err := validateScheduleInput(input); err != nil {
		return domain.Schedule{}, err
	}
	if _, err := s.repo.GetSpecialistByID(ctx, input.SpecialistID); err != nil {
		return domain.Schedule{}, err
	}

	return s.repo.CreateSchedule(ctx, domain.Schedule{
		SpecialistID:     input.SpecialistID,
		WorkDate:         normalizeDate(input.WorkDate),
		StartMinute:      input.StartMinute,
		EndMinute:        input.EndMinute,
		BreakStartMinute: input.BreakStartMinute,
		BreakEndMinute:   input.BreakEndMinute,
		IsDayOff:         input.IsDayOff,
	})
}

func (s *Service) UpsertSchedule(ctx context.Context, input SaveScheduleInput) (domain.Schedule, error) {
	if err := validateScheduleInput(input); err != nil {
		return domain.Schedule{}, err
	}
	if _, err := s.repo.GetSpecialistByID(ctx, input.SpecialistID); err != nil {
		return domain.Schedule{}, err
	}

	return s.repo.UpsertSchedule(ctx, domain.Schedule{
		SpecialistID:     input.SpecialistID,
		WorkDate:         normalizeDate(input.WorkDate),
		StartMinute:      input.StartMinute,
		EndMinute:        input.EndMinute,
		BreakStartMinute: input.BreakStartMinute,
		BreakEndMinute:   input.BreakEndMinute,
		IsDayOff:         input.IsDayOff,
	})
}

func (s *Service) GetSpecialistSchedule(ctx context.Context, specialistID int64, date time.Time) (ScheduleView, error) {
	specialist, err := s.repo.GetSpecialistByID(ctx, specialistID)
	if err != nil {
		return ScheduleView{}, err
	}

	loc := specialistLocation(specialist)
	workDate := normalizeDate(date)

	schedule, err := s.repo.GetScheduleByDate(ctx, specialistID, workDate)
	if err != nil {
		return ScheduleView{}, err
	}

	fromUTC, toUTC := dayRangeUTC(workDate, loc)
	appointments, err := s.repo.ListBookedAppointmentsByRange(ctx, specialistID, fromUTC, toUTC)
	if err != nil {
		return ScheduleView{}, err
	}

	return ScheduleView{Specialist: specialist, Schedule: schedule, Appointments: appointments}, nil
}

func (s *Service) GetFreeSlots(ctx context.Context, specialistID int64, date time.Time) (FreeSlotsView, error) {
	view, err := s.GetSpecialistSchedule(ctx, specialistID, date)
	if err != nil {
		return FreeSlotsView{}, err
	}

	if view.Schedule.IsDayOff {
		return FreeSlotsView{
			SpecialistID:        specialistID,
			Date:                view.Schedule.WorkDate,
			SlotDurationMinutes: view.Specialist.SlotDurationMinutes,
			FreeSlots:           []domain.TimeSlot{},
		}, nil
	}

	loc := specialistLocation(view.Specialist)
	workDate := normalizeDate(date)
	free := make([]domain.TimeSlot, 0)
	duration := time.Duration(view.Specialist.SlotDurationMinutes) * time.Minute

	for minute := view.Schedule.StartMinute; minute+view.Specialist.SlotDurationMinutes <= view.Schedule.EndMinute; minute += view.Specialist.SlotDurationMinutes {
		slotLocalStart := time.Date(workDate.Year(), workDate.Month(), workDate.Day(), 0, 0, 0, 0, loc).Add(time.Duration(minute) * time.Minute)
		slotLocalEnd := slotLocalStart.Add(duration)
		if inBreak(view.Schedule, minute, minute+view.Specialist.SlotDurationMinutes) {
			continue
		}

		slotUTCStart := slotLocalStart.UTC()
		slotUTCEnd := slotLocalEnd.UTC()
		busy := false
		for _, a := range view.Appointments {
			if intervalsOverlap(slotUTCStart, slotUTCEnd, a.StartTime, a.EndTime) {
				busy = true
				break
			}
		}
		if !busy {
			free = append(free, domain.TimeSlot{Start: slotUTCStart, End: slotUTCEnd})
		}
	}

	return FreeSlotsView{
		SpecialistID:        specialistID,
		Date:                view.Schedule.WorkDate,
		SlotDurationMinutes: view.Specialist.SlotDurationMinutes,
		FreeSlots:           free,
	}, nil
}

func (s *Service) GetAppointmentByID(ctx context.Context, appointmentID int64) (domain.Appointment, error) {
	if appointmentID <= 0 {
		return domain.Appointment{}, apperr.Validation("appointment id must be positive")
	}
	return s.repo.GetAppointmentByID(ctx, appointmentID)
}

func (s *Service) CreateAppointment(ctx context.Context, input CreateAppointmentInput) (domain.Appointment, error) {
	if input.SpecialistID <= 0 || input.ClientID <= 0 {
		return domain.Appointment{}, apperr.Validation("specialist_id and client_id must be positive")
	}
	if input.StartTime.IsZero() {
		return domain.Appointment{}, apperr.Validation("start_time is required")
	}
	if input.StartTime.Before(time.Now().Add(-1 * time.Minute)) {
		return domain.Appointment{}, apperr.Validation("cannot create appointment in the past")
	}

	specialist, err := s.repo.GetSpecialistByID(ctx, input.SpecialistID)
	if err != nil {
		return domain.Appointment{}, err
	}
	client, err := s.repo.GetClientByID(ctx, input.ClientID)
	if err != nil {
		return domain.Appointment{}, err
	}

	endTime, _, err := s.validateSlot(ctx, specialist, input.StartTime)
	if err != nil {
		return domain.Appointment{}, err
	}

	eventKey, err := generateEventKey()
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to generate event key", err)
	}

	eventPayload, err := json.Marshal(domain.AppointmentEventPayload{
		EventKey:       eventKey,
		EventType:      domain.AppointmentEventCreated,
		AppointmentID:  0,
		SpecialistID:   specialist.ID,
		SpecialistName: specialist.FullName,
		ClientID:       client.ID,
		ClientName:     client.FullName,
		TelegramChatID: client.TelegramChatID,
		StartTime:      input.StartTime.UTC(),
		EndTime:        endTime.UTC(),
		Status:         string(domain.AppointmentStatusBooked),
		OccurredAt:     time.Now().UTC(),
	})
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to marshal event", err)
	}

	created, err := s.repo.CreateAppointment(ctx, domain.Appointment{
		SpecialistID: input.SpecialistID,
		ClientID:     input.ClientID,
		StartTime:    input.StartTime.UTC(),
		EndTime:      endTime.UTC(),
		Status:       domain.AppointmentStatusBooked,
	}, OutboxEvent{EventKey: eventKey, Topic: s.kafkaTopic, EventType: domain.AppointmentEventCreated, Payload: eventPayload})
	if err != nil {
		return domain.Appointment{}, err
	}

	return created, nil
}

func (s *Service) CancelAppointment(ctx context.Context, appointmentID int64, reason string) (domain.Appointment, error) {
	if appointmentID <= 0 {
		return domain.Appointment{}, apperr.Validation("appointment id must be positive")
	}

	current, err := s.repo.GetAppointmentByID(ctx, appointmentID)
	if err != nil {
		return domain.Appointment{}, err
	}
	specialist, err := s.repo.GetSpecialistByID(ctx, current.SpecialistID)
	if err != nil {
		return domain.Appointment{}, err
	}
	client, err := s.repo.GetClientByID(ctx, current.ClientID)
	if err != nil {
		return domain.Appointment{}, err
	}

	eventKey, err := generateEventKey()
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to generate event key", err)
	}
	eventPayload, err := json.Marshal(domain.AppointmentEventPayload{
		EventKey:       eventKey,
		EventType:      domain.AppointmentEventCanceled,
		AppointmentID:  appointmentID,
		SpecialistID:   specialist.ID,
		SpecialistName: specialist.FullName,
		ClientID:       client.ID,
		ClientName:     client.FullName,
		TelegramChatID: client.TelegramChatID,
		StartTime:      current.StartTime,
		EndTime:        current.EndTime,
		Status:         string(domain.AppointmentStatusCanceled),
		OccurredAt:     time.Now().UTC(),
	})
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to marshal event", err)
	}

	canceled, err := s.repo.CancelAppointment(ctx, appointmentID, reason, OutboxEvent{EventKey: eventKey, Topic: s.kafkaTopic, EventType: domain.AppointmentEventCanceled, Payload: eventPayload})
	if err != nil {
		return domain.Appointment{}, err
	}
	return canceled, nil
}

func (s *Service) RescheduleAppointment(ctx context.Context, appointmentID int64, newStartTime time.Time) (domain.Appointment, error) {
	if appointmentID <= 0 {
		return domain.Appointment{}, apperr.Validation("appointment id must be positive")
	}
	if newStartTime.IsZero() {
		return domain.Appointment{}, apperr.Validation("new_start_time is required")
	}

	current, err := s.repo.GetAppointmentByID(ctx, appointmentID)
	if err != nil {
		return domain.Appointment{}, err
	}
	if current.Status != domain.AppointmentStatusBooked {
		return domain.Appointment{}, apperr.Conflict("only active appointment can be rescheduled")
	}
	if current.StartTime.Equal(newStartTime.UTC()) {
		return domain.Appointment{}, apperr.Validation("new_start_time must be different from current start_time")
	}

	specialist, err := s.repo.GetSpecialistByID(ctx, current.SpecialistID)
	if err != nil {
		return domain.Appointment{}, err
	}
	client, err := s.repo.GetClientByID(ctx, current.ClientID)
	if err != nil {
		return domain.Appointment{}, err
	}

	newEndTime, _, err := s.validateSlot(ctx, specialist, newStartTime)
	if err != nil {
		return domain.Appointment{}, err
	}

	eventKey, err := generateEventKey()
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to generate event key", err)
	}
	oldStart := current.StartTime
	oldEnd := current.EndTime
	eventPayload, err := json.Marshal(domain.AppointmentEventPayload{
		EventKey:       eventKey,
		EventType:      domain.AppointmentEventReschedule,
		AppointmentID:  appointmentID,
		SpecialistID:   specialist.ID,
		SpecialistName: specialist.FullName,
		ClientID:       client.ID,
		ClientName:     client.FullName,
		TelegramChatID: client.TelegramChatID,
		StartTime:      newStartTime.UTC(),
		EndTime:        newEndTime.UTC(),
		OldStartTime:   &oldStart,
		OldEndTime:     &oldEnd,
		Status:         string(domain.AppointmentStatusBooked),
		OccurredAt:     time.Now().UTC(),
	})
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to marshal event", err)
	}

	rescheduled, err := s.repo.RescheduleAppointment(ctx, appointmentID, newStartTime.UTC(), newEndTime.UTC(), OutboxEvent{EventKey: eventKey, Topic: s.kafkaTopic, EventType: domain.AppointmentEventReschedule, Payload: eventPayload})
	if err != nil {
		return domain.Appointment{}, err
	}
	return rescheduled, nil
}

func (s *Service) validateSlot(ctx context.Context, specialist domain.Specialist, start time.Time) (time.Time, domain.Schedule, error) {
	loc := specialistLocation(specialist)
	localStart := start.In(loc)
	if localStart.Second() != 0 || localStart.Nanosecond() != 0 {
		return time.Time{}, domain.Schedule{}, apperr.Validation("start_time must be rounded to a minute")
	}

	workDate := localDateInLocation(start, loc)
	schedule, err := s.repo.GetScheduleByDate(ctx, specialist.ID, workDate)
	if err != nil {
		return time.Time{}, domain.Schedule{}, err
	}
	if schedule.IsDayOff {
		return time.Time{}, domain.Schedule{}, apperr.Conflict("specialist is not working on this date")
	}

	duration := time.Duration(specialist.SlotDurationMinutes) * time.Minute
	localEnd := localStart.Add(duration)
	startMinute := localStart.Hour()*60 + localStart.Minute()
	endMinute := localEnd.Hour()*60 + localEnd.Minute()

	if startMinute < schedule.StartMinute || endMinute > schedule.EndMinute {
		return time.Time{}, domain.Schedule{}, apperr.Validation("selected time is outside specialist working hours")
	}
	if (startMinute-schedule.StartMinute)%specialist.SlotDurationMinutes != 0 {
		return time.Time{}, domain.Schedule{}, apperr.Validation("selected time does not match slot boundaries")
	}
	if inBreak(schedule, startMinute, endMinute) {
		return time.Time{}, domain.Schedule{}, apperr.Validation("selected time is inside specialist break")
	}
	return localEnd, schedule, nil
}

func validateScheduleInput(input SaveScheduleInput) error {
	if input.SpecialistID <= 0 {
		return apperr.Validation("specialist_id must be positive")
	}
	if input.WorkDate.IsZero() {
		return apperr.Validation("date is required")
	}
	if input.StartMinute < 0 || input.StartMinute >= 1440 {
		return apperr.Validation("start must be in HH:MM range 00:00-23:59")
	}
	if input.EndMinute <= 0 || input.EndMinute > 1440 {
		return apperr.Validation("end must be in HH:MM range 00:01-24:00")
	}
	if input.StartMinute >= input.EndMinute {
		return apperr.Validation("start must be earlier than end")
	}

	if (input.BreakStartMinute == nil) != (input.BreakEndMinute == nil) {
		return apperr.Validation("break_start and break_end must be provided together")
	}
	if input.BreakStartMinute != nil && input.BreakEndMinute != nil {
		breakStart := *input.BreakStartMinute
		breakEnd := *input.BreakEndMinute
		if breakStart < input.StartMinute || breakEnd > input.EndMinute {
			return apperr.Validation("break must be inside working hours")
		}
		if breakStart >= breakEnd {
			return apperr.Validation("break_start must be earlier than break_end")
		}
	}
	return nil
}

func specialistLocation(specialist domain.Specialist) *time.Location {
	loc, err := time.LoadLocation(specialist.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func normalizeDate(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
}

func localDateInLocation(ts time.Time, loc *time.Location) time.Time {
	inLoc := ts.In(loc)
	return time.Date(inLoc.Year(), inLoc.Month(), inLoc.Day(), 0, 0, 0, 0, time.UTC)
}

func dayRangeUTC(workDate time.Time, loc *time.Location) (time.Time, time.Time) {
	localDayStart := time.Date(workDate.Year(), workDate.Month(), workDate.Day(), 0, 0, 0, 0, loc)
	localDayEnd := localDayStart.Add(24 * time.Hour)
	return localDayStart.UTC(), localDayEnd.UTC()
}

func intervalsOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

func inBreak(schedule domain.Schedule, startMinute int, endMinute int) bool {
	if schedule.BreakStartMinute == nil || schedule.BreakEndMinute == nil {
		return false
	}
	return startMinute < *schedule.BreakEndMinute && *schedule.BreakStartMinute < endMinute
}

func IsNotFound(err error) bool {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Code == apperr.CodeNotFound
	}
	return false
}

func generateEventKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (s *Service) issueAuthResult(user domain.User) (AuthResult, error) {
	if s.tokens == nil {
		return AuthResult{}, apperr.Internal("token manager is not configured", errors.New("nil token manager"))
	}

	token, expiresAt, err := s.tokens.Generate(user)
	if err != nil {
		return AuthResult{}, apperr.Internal("failed to issue auth token", err)
	}

	return AuthResult{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt.UTC(),
		User: AuthUser{
			ID:           user.ID,
			Email:        user.Email,
			Role:         user.Role,
			ClientID:     user.ClientID,
			SpecialistID: user.SpecialistID,
		},
	}, nil
}

func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if email == "" {
		return "", apperr.Validation("email is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", apperr.Validation("email must be valid")
	}
	return email, nil
}

func buildClient(fullName string, phone string, telegramChatID *int64) (domain.Client, error) {
	name := strings.TrimSpace(fullName)
	phoneValue := strings.TrimSpace(phone)
	if name == "" {
		return domain.Client{}, apperr.Validation("full_name is required")
	}
	if phoneValue == "" {
		return domain.Client{}, apperr.Validation("phone is required")
	}
	if telegramChatID != nil && *telegramChatID <= 0 {
		return domain.Client{}, apperr.Validation("telegram_chat_id must be positive when provided")
	}

	return domain.Client{
		FullName:       name,
		Phone:          phoneValue,
		TelegramChatID: telegramChatID,
	}, nil
}

func buildSpecialist(fullName string, profession string, slotDurationMinutes int, timezone string) (domain.Specialist, error) {
	name := strings.TrimSpace(fullName)
	professionValue := strings.TrimSpace(profession)
	if name == "" {
		return domain.Specialist{}, apperr.Validation("full_name is required")
	}
	if professionValue == "" {
		return domain.Specialist{}, apperr.Validation("profession is required")
	}
	if slotDurationMinutes <= 0 || slotDurationMinutes > 240 {
		return domain.Specialist{}, apperr.Validation("slot_duration_minutes must be between 1 and 240")
	}

	timezoneValue := strings.TrimSpace(timezone)
	if timezoneValue == "" {
		timezoneValue = "UTC"
	}
	if _, err := time.LoadLocation(timezoneValue); err != nil {
		return domain.Specialist{}, apperr.Validation("timezone must be a valid IANA timezone (for example, Europe/Moscow)")
	}

	return domain.Specialist{
		FullName:            name,
		Profession:          professionValue,
		SlotDurationMinutes: slotDurationMinutes,
		Timezone:            timezoneValue,
	}, nil
}
