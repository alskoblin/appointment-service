package application

import (
	"context"

	"appointment-service/internal/calendar/domain"
)

type Repository interface {
	IsProcessed(ctx context.Context, eventKey string) (bool, error)
	GetCalendarEventMapping(ctx context.Context, appointmentID int64) (domain.CalendarEventMapping, bool, error)
	MarkProcessedAndLog(ctx context.Context, payload domain.AppointmentEventPayload, status domain.SyncStatus, googleEventID *string, errorText *string) error
}

type CalendarClient interface {
	Enabled() bool
	CreateEvent(ctx context.Context, payload domain.AppointmentEventPayload) (string, error)
	UpdateEvent(ctx context.Context, googleEventID string, payload domain.AppointmentEventPayload) error
	DeleteEvent(ctx context.Context, googleEventID string) error
}
