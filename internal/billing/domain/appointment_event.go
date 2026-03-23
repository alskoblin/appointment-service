package domain

import "time"

type AppointmentEventPayload struct {
	EventKey       string     `json:"event_key"`
	EventType      string     `json:"event_type"`
	AppointmentID  int64      `json:"appointment_id"`
	SpecialistID   int64      `json:"specialist_id"`
	SpecialistName string     `json:"specialist_name"`
	ClientID       int64      `json:"client_id"`
	ClientName     string     `json:"client_name"`
	TelegramChatID *int64     `json:"telegram_chat_id,omitempty"`
	StartTime      time.Time  `json:"start_time"`
	EndTime        time.Time  `json:"end_time"`
	OldStartTime   *time.Time `json:"old_start_time,omitempty"`
	OldEndTime     *time.Time `json:"old_end_time,omitempty"`
	Status         string     `json:"status"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

const (
	AppointmentEventCreated    = "appointment.created.v1"
	AppointmentEventCanceled   = "appointment.canceled.v1"
	AppointmentEventReschedule = "appointment.rescheduled.v1"
)
