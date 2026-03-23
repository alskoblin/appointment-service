package domain

import "time"

type AppointmentStatus string

const (
	AppointmentStatusBooked   AppointmentStatus = "booked"
	AppointmentStatusCanceled AppointmentStatus = "canceled"
)

type Appointment struct {
	ID           int64             `json:"id"`
	SpecialistID int64             `json:"specialist_id"`
	ClientID     int64             `json:"client_id"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	Status       AppointmentStatus `json:"status"`
	CancelReason *string           `json:"cancel_reason,omitempty"`
	CanceledAt   *time.Time        `json:"canceled_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}
