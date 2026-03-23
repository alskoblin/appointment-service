package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"appointment-service/internal/calendar/bootstrap"
	"appointment-service/internal/config"
)

func main() {
	cfg, err := config.LoadCalendarSync()
	if err != nil {
		slog.Error("failed to load calendar-sync config", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	logger.Info("calendar-sync service starting")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := bootstrap.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to initialize calendar-sync service", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	if err := application.Run(ctx); err != nil {
		logger.Error("calendar-sync service stopped with error", "error", err)
		os.Exit(1)
	}

	logger.Info("calendar-sync service stopped gracefully")
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
