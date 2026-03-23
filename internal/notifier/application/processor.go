package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"appointment-service/internal/notifier/domain"
)

type Processor struct {
	repo     Repository
	notifier Notifier
	logger   *slog.Logger
}

func NewProcessor(repo Repository, notifier Notifier, logger *slog.Logger) *Processor {
	return &Processor{repo: repo, notifier: notifier, logger: logger}
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

	if payload.TelegramChatID == nil {
		msg := "telegram_chat_id is missing"
		return p.repo.MarkProcessedAndLog(ctx, payload, domain.NotificationStatusSkipped, &msg)
	}

	if p.notifier == nil || !p.notifier.Enabled() {
		msg := "telegram client is disabled"
		return p.repo.MarkProcessedAndLog(ctx, payload, domain.NotificationStatusSkipped, &msg)
	}

	messageText := renderMessage(payload)
	notifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := p.notifier.SendMessage(notifyCtx, *payload.TelegramChatID, messageText); err != nil {
		errText := err.Error()
		if markErr := p.repo.MarkProcessedAndLog(ctx, payload, domain.NotificationStatusFailed, &errText); markErr != nil {
			return markErr
		}
		p.logger.Error("telegram send failed", "event_key", payload.EventKey, "event_type", payload.EventType, "error", err)
		return nil
	}

	return p.repo.MarkProcessedAndLog(ctx, payload, domain.NotificationStatusSent, nil)
}

func renderMessage(payload domain.AppointmentEventPayload) string {
	start := payload.StartTime.Format(time.RFC3339)

	switch payload.EventType {
	case domain.AppointmentEventCreated:
		return fmt.Sprintf("Appointment confirmed\nSpecialist: %s\nStart: %s", payload.SpecialistName, start)
	case domain.AppointmentEventCanceled:
		return fmt.Sprintf("Appointment canceled\nSpecialist: %s\nStart: %s", payload.SpecialistName, start)
	case domain.AppointmentEventReschedule:
		if payload.OldStartTime == nil {
			return fmt.Sprintf("Appointment rescheduled\nSpecialist: %s\nNew start: %s", payload.SpecialistName, start)
		}
		return fmt.Sprintf("Appointment rescheduled\nSpecialist: %s\nOld start: %s\nNew start: %s", payload.SpecialistName, payload.OldStartTime.Format(time.RFC3339), start)
	case domain.AppointmentEventReminder24:
		return fmt.Sprintf("Reminder: your appointment is in 24 hours\nSpecialist: %s\nStart: %s", payload.SpecialistName, start)
	case domain.AppointmentEventReminder1:
		return fmt.Sprintf("Reminder: your appointment is in 1 hour\nSpecialist: %s\nStart: %s", payload.SpecialistName, start)
	default:
		return fmt.Sprintf("Appointment updated\nSpecialist: %s\nStart: %s", payload.SpecialistName, start)
	}
}
