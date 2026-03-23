package outbox

import (
	"context"
	"log/slog"
	"time"

	"appointment-service/internal/booking/application"
)

type Producer interface {
	Publish(ctx context.Context, topic string, key string, value []byte) error
}

type Relay struct {
	repo     application.OutboxRepository
	producer Producer
	logger   *slog.Logger
	batch    int
	pause    time.Duration
}

func NewRelay(repo application.OutboxRepository, producer Producer, logger *slog.Logger) *Relay {
	return &Relay{repo: repo, producer: producer, logger: logger, batch: 50, pause: 500 * time.Millisecond}
}

func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.pause)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("outbox relay stopped")
			return
		case <-ticker.C:
			r.processBatch(ctx)
		}
	}
}

func (r *Relay) processBatch(ctx context.Context) {
	events, err := r.repo.ClaimOutboxEvents(ctx, r.batch)
	if err != nil {
		r.logger.Error("outbox claim failed", "error", err)
		return
	}
	if len(events) == 0 {
		return
	}

	for _, event := range events {
		if err := r.producer.Publish(ctx, event.Topic, event.EventKey, event.Payload); err != nil {
			releaseErr := r.repo.ReleaseOutboxEvent(ctx, event.ID, err.Error())
			if releaseErr != nil {
				r.logger.Error("failed to release outbox event after kafka error", "event_id", event.ID, "error", releaseErr)
			}
			r.logger.Error("kafka publish failed", "event_id", event.ID, "event_type", event.EventType, "error", err)
			continue
		}

		if err := r.repo.MarkOutboxEventPublished(ctx, event.ID); err != nil {
			r.logger.Error("failed to mark outbox event as published", "event_id", event.ID, "error", err)
			continue
		}
	}
}
