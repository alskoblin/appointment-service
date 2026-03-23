package application

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"appointment-service/internal/apperr"
	"appointment-service/internal/booking/domain"

	"golang.org/x/crypto/bcrypt"
)

func (s *Service) Register(ctx context.Context, input RegisterInput) (AuthResult, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, err
	}
	password := strings.TrimSpace(input.Password)
	if len(password) < 8 {
		return AuthResult{}, apperr.Validation("password must be at least 8 characters")
	}
	role, err := domain.ParseUserRole(input.Role)
	if err != nil {
		return AuthResult{}, apperr.Validation(err.Error())
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, apperr.Internal("failed to hash password", err)
	}

	user := domain.User{
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
	}

	var created domain.User
	switch role {
	case domain.RoleAdmin:
		created, err = s.repo.CreateUser(ctx, user)
	case domain.RoleClient:
		client, buildErr := buildClient(input.FullName, input.Phone, input.TelegramChatID)
		if buildErr != nil {
			return AuthResult{}, buildErr
		}
		created, err = s.repo.CreateUserWithClient(ctx, user, client)
	case domain.RoleSpecialist:
		specialist, buildErr := buildSpecialist(input.FullName, input.Profession, input.SlotDurationMinutes, input.Timezone)
		if buildErr != nil {
			return AuthResult{}, buildErr
		}
		created, err = s.repo.CreateUserWithSpecialist(ctx, user, specialist)
	default:
		return AuthResult{}, apperr.Validation("unsupported role")
	}
	if err != nil {
		return AuthResult{}, err
	}
	return s.issueAuthResult(created)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (AuthResult, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, apperr.Unauthorized("invalid email or password")
	}
	password := strings.TrimSpace(input.Password)
	if password == "" {
		return AuthResult{}, apperr.Unauthorized("invalid email or password")
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if IsNotFound(err) {
			return AuthResult{}, apperr.Unauthorized("invalid email or password")
		}
		return AuthResult{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return AuthResult{}, apperr.Unauthorized("invalid email or password")
	}

	return s.issueAuthResult(user)
}

func (s *Service) issueAuthResult(user domain.User) (AuthResult, error) {
	if s.tokens == nil {
		return AuthResult{}, apperr.Internal("token manager is not configured", errors.New("nil token manager"))
	}

	token, expiresAt, err := s.tokens.Generate(user)
	if err != nil {
		return AuthResult{}, apperr.Internal("failed to issue auth token", err)
	}

	return AuthResult{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt.UTC(),
		User: AuthUser{
			ID:           user.ID,
			Email:        user.Email,
			Role:         user.Role,
			ClientID:     user.ClientID,
			SpecialistID: user.SpecialistID,
		},
	}, nil
}

func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if email == "" {
		return "", apperr.Validation("email is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", apperr.Validation("email must be valid")
	}
	return email, nil
}
