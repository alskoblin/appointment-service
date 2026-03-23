package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"appointment-service/internal/billing/application"
)

type MockGateway struct {
	enabled bool
	mu      sync.Mutex
	charges map[string]string
	refunds map[string]string
}

func NewMockGateway(enabled bool) *MockGateway {
	return &MockGateway{
		enabled: enabled,
		charges: make(map[string]string),
		refunds: make(map[string]string),
	}
}

func (g *MockGateway) Enabled() bool {
	if g == nil {
		return false
	}
	return g.enabled
}

func (g *MockGateway) Charge(_ context.Context, req application.ChargeRequest) (string, error) {
	if !g.Enabled() {
		return "", fmt.Errorf("mock payment gateway is disabled")
	}
	if req.AmountCents <= 0 {
		return "", fmt.Errorf("amount_cents must be positive")
	}
	if strings.TrimSpace(req.Currency) == "" {
		return "", fmt.Errorf("currency is required")
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return "", fmt.Errorf("idempotency_key is required")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if existing := g.charges[req.IdempotencyKey]; existing != "" {
		return existing, nil
	}
	paymentID, err := randomID("pay")
	if err != nil {
		return "", err
	}
	g.charges[req.IdempotencyKey] = paymentID
	return paymentID, nil
}

func (g *MockGateway) Refund(_ context.Context, req application.RefundRequest) (string, error) {
	if !g.Enabled() {
		return "", fmt.Errorf("mock payment gateway is disabled")
	}
	if req.AmountCents <= 0 {
		return "", fmt.Errorf("amount_cents must be positive")
	}
	if strings.TrimSpace(req.Currency) == "" {
		return "", fmt.Errorf("currency is required")
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return "", fmt.Errorf("idempotency_key is required")
	}
	if strings.TrimSpace(req.PaymentID) == "" {
		return "", fmt.Errorf("payment_id is required")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if existing := g.refunds[req.IdempotencyKey]; existing != "" {
		return existing, nil
	}
	refundID, err := randomID("refund")
	if err != nil {
		return "", err
	}
	g.refunds[req.IdempotencyKey] = refundID
	return refundID, nil
}

func randomID(prefix string) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(buf), nil
}
