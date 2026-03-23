package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"appointment-service/internal/billing/application"
	billpayment "appointment-service/internal/billing/infrastructure/payment"
	billpg "appointment-service/internal/billing/infrastructure/postgres"
	billhttp "appointment-service/internal/billing/transport/http"
	billkafka "appointment-service/internal/billing/transport/kafka"
	"appointment-service/internal/config"
	"appointment-service/internal/middleware"

	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	cfg      config.BillingConfig
	logger   *slog.Logger
	db       *pgxpool.Pool
	http     *http.Server
	consumer *billkafka.Consumer
}

func New(ctx context.Context, cfg config.BillingConfig, logger *slog.Logger) (*App, error) {
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

	repo := billpg.New(db)
	payments := billpayment.NewMockGateway(cfg.PaymentsEnabled)
	processor := application.NewProcessor(repo, payments, logger, cfg.DefaultAmountCents, cfg.Currency)
	consumer := billkafka.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroupID, processor, logger)

	svc := application.NewService(repo, payments)
	h := billhttp.New(svc, logger)

	mux := http.NewServeMux()
	h.Register(mux)

	handlerWithMW := middleware.Chain(
		mux,
		func(next http.Handler) http.Handler { return middleware.Recover(logger, next) },
		func(next http.Handler) http.Handler { return middleware.Logging(logger, next) },
		middleware.RequestID,
	)

	httpServer := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           handlerWithMW,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &App{
		cfg:      cfg,
		logger:   logger,
		db:       db,
		http:     httpServer,
		consumer: consumer,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 2)

	go func() {
		a.logger.Info("billing consumer started", "topic", a.cfg.KafkaTopic, "group", a.cfg.KafkaGroupID)
		if err := a.consumer.Run(ctx); err != nil {
			errCh <- fmt.Errorf("billing consumer failed: %w", err)
		}
		a.logger.Info("billing consumer stopped")
	}()

	go func() {
		a.logger.Info("billing http server starting", "addr", a.http.Addr)
		err := a.http.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("billing http server failed: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown failed: %w", err)
	}
	a.logger.Info("billing http server stopped")
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
		a.logger.Info("billing db pool closed")
	}
}
