package reminder

import (
	"context"
	"log/slog"
	"time"

	"appointment-service/internal/booking/domain"
)

type Repository interface {
	EnqueueReminderEvents(ctx context.Context, eventType string, topic string, from time.Time, to time.Time, limit int) (int, error)
}

type Dispatcher struct {
	repo      Repository
	logger    *slog.Logger
	topic     string
	batchSize int
	interval  time.Duration
}

func NewDispatcher(repo Repository, logger *slog.Logger, topic string) *Dispatcher {
	return &Dispatcher{
		repo:      repo,
		logger:    logger,
		topic:     topic,
		batchSize: 100,
		interval:  30 * time.Second,
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("reminder dispatcher stopped")
			return
		case <-ticker.C:
			d.tick(ctx)
		}
	}
}

func (d *Dispatcher) tick(ctx context.Context) {
	now := time.Now().UTC()

	enqueued24, err := d.repo.EnqueueReminderEvents(
		ctx,
		domain.AppointmentEventReminder24,
		d.topic,
		now.Add(23*time.Hour),
		now.Add(24*time.Hour),
		d.batchSize,
	)
	if err != nil {
		d.logger.Error("enqueue 24h reminders failed", "error", err)
	} else if enqueued24 > 0 {
		d.logger.Info("reminder events enqueued", "kind", "24h", "count", enqueued24)
	}

	enqueued1, err := d.repo.EnqueueReminderEvents(
		ctx,
		domain.AppointmentEventReminder1,
		d.topic,
		now,
		now.Add(1*time.Hour),
		d.batchSize,
	)
	if err != nil {
		d.logger.Error("enqueue 1h reminders failed", "error", err)
	} else if enqueued1 > 0 {
		d.logger.Info("reminder events enqueued", "kind", "1h", "count", enqueued1)
	}
}
