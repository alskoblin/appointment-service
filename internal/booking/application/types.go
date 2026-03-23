package application

import (
	"time"

	"appointment-service/internal/booking/domain"
)

type ScheduleView struct {
	Specialist   domain.Specialist
	Schedule     domain.Schedule
	Appointments []domain.Appointment
}

type FreeSlotsView struct {
	SpecialistID        int64
	Date                time.Time
	SlotDurationMinutes int
	FreeSlots           []domain.TimeSlot
}

type CreateAppointmentInput struct {
	SpecialistID int64
	ClientID     int64
	StartTime    time.Time
}

type CreateSpecialistInput struct {
	FullName            string
	Profession          string
	SlotDurationMinutes int
	Timezone            string
}

type CreateClientInput struct {
	FullName       string
	Phone          string
	TelegramChatID *int64
}

type RegisterInput struct {
	Email               string
	Password            string
	Role                string
	FullName            string
	Phone               string
	TelegramChatID      *int64
	Profession          string
	SlotDurationMinutes int
	Timezone            string
}

type LoginInput struct {
	Email    string
	Password string
}

type AuthResult struct {
	AccessToken string
	TokenType   string
	ExpiresAt   time.Time
	User        AuthUser
}

type AuthUser struct {
	ID           int64
	Email        string
	Role         domain.UserRole
	ClientID     *int64
	SpecialistID *int64
}

type SaveScheduleInput struct {
	SpecialistID     int64
	WorkDate         time.Time
	StartMinute      int
	EndMinute        int
	BreakStartMinute *int
	BreakEndMinute   *int
	IsDayOff         bool
}
