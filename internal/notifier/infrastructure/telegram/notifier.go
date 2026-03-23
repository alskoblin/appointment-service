package telegram

import (
	"context"

	basetg "appointment-service/internal/telegram"
)

type Notifier struct {
	client *basetg.Client
}

func New(token string, baseURL string) *Notifier {
	return &Notifier{client: basetg.NewClient(token, baseURL)}
}

func (n *Notifier) Enabled() bool {
	if n == nil || n.client == nil {
		return false
	}
	return n.client.Enabled()
}

func (n *Notifier) SendMessage(ctx context.Context, chatID int64, text string) error {
	return n.client.SendMessage(ctx, chatID, text)
}
