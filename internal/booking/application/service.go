package application

import (
	"log/slog"

	"appointment-service/internal/booking/auth"
)

type Service struct {
	repo       Repository
	logger     *slog.Logger
	kafkaTopic string
	tokens     *auth.Manager
}

func New(repo Repository, logger *slog.Logger, kafkaTopic string, tokens *auth.Manager) *Service {
	return &Service{repo: repo, logger: logger, kafkaTopic: kafkaTopic, tokens: tokens}
}
