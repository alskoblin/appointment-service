package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"appointment-service/internal/booking/application"
	bookingauth "appointment-service/internal/booking/auth"
	"appointment-service/internal/booking/infrastructure/kafka"
	"appointment-service/internal/booking/infrastructure/outbox"
	bookingpg "appointment-service/internal/booking/infrastructure/postgres"
	"appointment-service/internal/booking/infrastructure/reminder"
	bookinghttp "appointment-service/internal/booking/transport/http"
	"appointment-service/internal/config"
	"appointment-service/internal/middleware"

	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	cfg      config.BookingConfig
	logger   *slog.Logger
	db       *pgxpool.Pool
	http     *http.Server
	producer *kafka.Producer
	relay    *outbox.Relay
	reminder *reminder.Dispatcher
}

func New(ctx context.Context, cfg config.BookingConfig, logger *slog.Logger) (*App, error) {
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

	repo := bookingpg.New(db)
	tokenManager := bookingauth.NewManager(cfg.JWTSecret, time.Duration(cfg.JWTTTLMinutes)*time.Minute)
	appSvc := application.New(repo, logger, cfg.KafkaTopic, tokenManager)
	h := bookinghttp.New(appSvc, logger)

	producer := kafka.NewProducer(cfg.KafkaBrokers)
	relay := outbox.NewRelay(repo, producer, logger)
	reminderDispatcher := reminder.NewDispatcher(repo, logger, cfg.KafkaTopic)

	mux := http.NewServeMux()
	h.Register(mux)

	handlerWithMW := middleware.Chain(
		mux,
		func(next http.Handler) http.Handler { return middleware.Recover(logger, next) },
		func(next http.Handler) http.Handler { return middleware.Logging(logger, next) },
		middleware.RequestID,
		bookinghttp.AuthMiddleware(tokenManager),
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
		producer: producer,
		relay:    relay,
		reminder: reminderDispatcher,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go a.relay.Run(ctx)
	go a.reminder.Run(ctx)

	go func() {
		a.logger.Info("booking http server starting", "addr", a.http.Addr)
		err := a.http.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server failed: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown failed: %w", err)
	}
	a.logger.Info("booking http server stopped")
	return nil
}

func (a *App) Close() {
	if a.producer != nil {
		if err := a.producer.Close(); err != nil {
			a.logger.Warn("failed to close kafka producer", "error", err)
		}
	}
	if a.db != nil {
		a.db.Close()
		a.logger.Info("database pool closed")
	}
}
