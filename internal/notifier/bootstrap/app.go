package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"appointment-service/internal/config"
	"appointment-service/internal/notifier/application"
	notifpg "appointment-service/internal/notifier/infrastructure/postgres"
	notiftelegram "appointment-service/internal/notifier/infrastructure/telegram"
	notifierkafka "appointment-service/internal/notifier/transport/kafka"

	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	cfg      config.NotificationConfig
	logger   *slog.Logger
	db       *pgxpool.Pool
	consumer *notifierkafka.Consumer
}

func New(ctx context.Context, cfg config.NotificationConfig, logger *slog.Logger) (*App, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL())
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 1

	db, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create db pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = db.Ping(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	repo := notifpg.New(db)
	notifier := notiftelegram.New(cfg.TelegramBotToken, cfg.TelegramAPIBaseURL)
	processor := application.NewProcessor(repo, notifier, logger)
	consumer := notifierkafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroupID, processor, logger)

	return &App{cfg: cfg, logger: logger, db: db, consumer: consumer}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.logger.Info("notification consumer started", "topic", a.cfg.KafkaTopic, "group", a.cfg.KafkaGroupID)
	if err := a.consumer.Run(ctx); err != nil {
		return err
	}
	a.logger.Info("notification consumer stopped")
	return nil
}

func (a *App) Close() {
	if a.consumer != nil {
		if err := a.consumer.Close(); err != nil {
			a.logger.Warn("failed to close kafka reader", "error", err)
		}
	}
	if a.db != nil {
		a.db.Close()
		a.logger.Info("notification db pool closed")
	}
}
