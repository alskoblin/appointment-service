package domain

import "time"

type Client struct {
	ID             int64     `json:"id"`
	FullName       string    `json:"full_name"`
	Phone          string    `json:"phone"`
	TelegramChatID *int64    `json:"telegram_chat_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
