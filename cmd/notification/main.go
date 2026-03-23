package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"appointment-service/internal/config"
	"appointment-service/internal/notifier/bootstrap"
)

func main() {
	cfg, err := config.LoadNotification()
	if err != nil {
		slog.Error("failed to load notification config", "error", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	logger.Info("notification service starting")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := bootstrap.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to initialize notification service", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	if err := application.Run(ctx); err != nil {
		logger.Error("notification service stopped with error", "error", err)
		os.Exit(1)
	}

	logger.Info("notification service stopped gracefully")
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
