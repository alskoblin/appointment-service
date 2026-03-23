package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"appointment-service/internal/apperr"
	"appointment-service/internal/booking/domain"
)

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

func generateEventKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
