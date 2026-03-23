package application

import (
	"context"
	"strings"
	"time"

	"appointment-service/internal/apperr"
	"appointment-service/internal/booking/domain"
)

func (s *Service) ListSpecialists(ctx context.Context) ([]domain.Specialist, error) {
	return s.repo.ListSpecialists(ctx)
}

func (s *Service) CreateSpecialist(ctx context.Context, input CreateSpecialistInput) (domain.Specialist, error) {
	specialist, err := buildSpecialist(input.FullName, input.Profession, input.SlotDurationMinutes, input.Timezone)
	if err != nil {
		return domain.Specialist{}, err
	}
	return s.repo.CreateSpecialist(ctx, specialist)
}

func (s *Service) CreateClient(ctx context.Context, input CreateClientInput) (domain.Client, error) {
	client, err := buildClient(input.FullName, input.Phone, input.TelegramChatID)
	if err != nil {
		return domain.Client{}, err
	}
	return s.repo.CreateClient(ctx, client)
}

func buildClient(fullName string, phone string, telegramChatID *int64) (domain.Client, error) {
	name := strings.TrimSpace(fullName)
	phoneValue := strings.TrimSpace(phone)
	if name == "" {
		return domain.Client{}, apperr.Validation("full_name is required")
	}
	if phoneValue == "" {
		return domain.Client{}, apperr.Validation("phone is required")
	}
	if telegramChatID != nil && *telegramChatID <= 0 {
		return domain.Client{}, apperr.Validation("telegram_chat_id must be positive when provided")
	}

	return domain.Client{
		FullName:       name,
		Phone:          phoneValue,
		TelegramChatID: telegramChatID,
	}, nil
}

func buildSpecialist(fullName string, profession string, slotDurationMinutes int, timezone string) (domain.Specialist, error) {
	name := strings.TrimSpace(fullName)
	professionValue := strings.TrimSpace(profession)
	if name == "" {
		return domain.Specialist{}, apperr.Validation("full_name is required")
	}
	if professionValue == "" {
		return domain.Specialist{}, apperr.Validation("profession is required")
	}
	if slotDurationMinutes <= 0 || slotDurationMinutes > 240 {
		return domain.Specialist{}, apperr.Validation("slot_duration_minutes must be between 1 and 240")
	}

	timezoneValue := strings.TrimSpace(timezone)
	if timezoneValue == "" {
		timezoneValue = "UTC"
	}
	if _, err := time.LoadLocation(timezoneValue); err != nil {
		return domain.Specialist{}, apperr.Validation("timezone must be a valid IANA timezone (for example, Europe/Moscow)")
	}

	return domain.Specialist{
		FullName:            name,
		Profession:          professionValue,
		SlotDurationMinutes: slotDurationMinutes,
		Timezone:            timezoneValue,
	}, nil
}
