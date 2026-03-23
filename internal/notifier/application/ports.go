package application

import (
	"context"

	"appointment-service/internal/notifier/domain"
)

type Repository interface {
	IsProcessed(ctx context.Context, eventKey string) (bool, error)
	MarkProcessedAndLog(ctx context.Context, payload domain.AppointmentEventPayload, status domain.NotificationStatus, errorText *string) error
}

type Notifier interface {
	Enabled() bool
	SendMessage(ctx context.Context, chatID int64, text string) error
}
