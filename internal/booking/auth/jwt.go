package auth

import (
	"errors"
	"fmt"
	"time"

	"appointment-service/internal/booking/domain"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

type Manager struct {
	secret []byte
	ttl    time.Duration
	nowFn  func() time.Time
}

type Claims struct {
	UserID       int64           `json:"uid"`
	Email        string          `json:"email"`
	Role         domain.UserRole `json:"role"`
	ClientID     *int64          `json:"client_id,omitempty"`
	SpecialistID *int64          `json:"specialist_id,omitempty"`
	jwt.RegisteredClaims
}

func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{
		secret: []byte(secret),
		ttl:    ttl,
		nowFn:  time.Now,
	}
}

func (m *Manager) Generate(user domain.User) (string, time.Time, error) {
	now := m.nowFn().UTC()
	expiresAt := now.Add(m.ttl)

	claims := Claims{
		UserID:       user.ID,
		Email:        user.Email,
		Role:         user.Role,
		ClientID:     user.ClientID,
		SpecialistID: user.SpecialistID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", user.ID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign jwt: %w", err)
	}
	return signed, expiresAt, nil
}

func (m *Manager) Parse(tokenString string) (domain.Identity, error) {
	var claims Claims
	token, err := jwt.ParseWithClaims(
		tokenString,
		&claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidToken
			}
			return m.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return domain.Identity{}, ErrInvalidToken
	}
	if !token.Valid {
		return domain.Identity{}, ErrInvalidToken
	}

	role, err := domain.ParseUserRole(string(claims.Role))
	if err != nil {
		return domain.Identity{}, ErrInvalidToken
	}
	if claims.UserID <= 0 || claims.Email == "" {
		return domain.Identity{}, ErrInvalidToken
	}

	return domain.Identity{
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         role,
		ClientID:     claims.ClientID,
		SpecialistID: claims.SpecialistID,
	}, nil
}
