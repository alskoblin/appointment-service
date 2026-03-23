package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"appointment-service/internal/calendar/domain"
	googlecalendar "appointment-service/internal/calendar/infrastructure/google"
)

type Processor struct {
	repo   Repository
	client CalendarClient
	logger *slog.Logger
}

func NewProcessor(repo Repository, client CalendarClient, logger *slog.Logger) *Processor {
	return &Processor{repo: repo, client: client, logger: logger}
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

	if !isCalendarRelevant(payload.EventType) {
		msg := "event type is not relevant for calendar sync"
		return p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusSkipped, nil, &msg)
	}

	if payload.AppointmentID <= 0 {
		msg := "appointment_id must be positive for calendar sync"
		return p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusSkipped, nil, &msg)
	}

	if p.client == nil || !p.client.Enabled() {
		msg := "google calendar client is disabled"
		return p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusSkipped, nil, &msg)
	}

	switch payload.EventType {
	case domain.AppointmentEventCreated, domain.AppointmentEventReschedule:
		return p.syncUpsert(ctx, payload)
	case domain.AppointmentEventCanceled:
		return p.syncDelete(ctx, payload)
	default:
		msg := "event type is not supported by calendar sync"
		return p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusSkipped, nil, &msg)
	}
}

func (p *Processor) syncUpsert(ctx context.Context, payload domain.AppointmentEventPayload) error {
	mapping, exists, err := p.repo.GetCalendarEventMapping(ctx, payload.AppointmentID)
	if err != nil {
		return err
	}

	if exists && strings.TrimSpace(mapping.GoogleEventID) != "" {
		err = p.client.UpdateEvent(ctx, mapping.GoogleEventID, payload)
		if err == nil {
			id := mapping.GoogleEventID
			return p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusSynced, &id, nil)
		}

		if err != googlecalendar.ErrNotFound {
			return p.markFailed(ctx, payload, err)
		}
	}

	googleEventID, err := p.client.CreateEvent(ctx, payload)
	if err != nil {
		return p.markFailed(ctx, payload, err)
	}

	return p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusSynced, &googleEventID, nil)
}

func (p *Processor) syncDelete(ctx context.Context, payload domain.AppointmentEventPayload) error {
	mapping, exists, err := p.repo.GetCalendarEventMapping(ctx, payload.AppointmentID)
	if err != nil {
		return err
	}

	if !exists || strings.TrimSpace(mapping.GoogleEventID) == "" {
		msg := "calendar event is not linked for this appointment"
		return p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusSkipped, nil, &msg)
	}

	err = p.client.DeleteEvent(ctx, mapping.GoogleEventID)
	if err != nil && err != googlecalendar.ErrNotFound {
		return p.markFailed(ctx, payload, err)
	}

	return p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusDeleted, nil, nil)
}

func (p *Processor) markFailed(ctx context.Context, payload domain.AppointmentEventPayload, err error) error {
	errText := err.Error()
	if markErr := p.repo.MarkProcessedAndLog(ctx, payload, domain.SyncStatusFailed, nil, &errText); markErr != nil {
		return markErr
	}
	p.logger.Error("calendar sync failed", "event_key", payload.EventKey, "event_type", payload.EventType, "error", err)
	return nil
}

func isCalendarRelevant(eventType string) bool {
	switch eventType {
	case domain.AppointmentEventCreated, domain.AppointmentEventReschedule, domain.AppointmentEventCanceled:
		return true
	default:
		return false
	}
}
