package application

import (
	"context"
	"encoding/json"
	"time"

	"appointment-service/internal/booking/domain"
)

type OutboxEvent struct {
	ID         int64           `json:"id"`
	EventKey   string          `json:"event_key"`
	Topic      string          `json:"topic"`
	EventType  string          `json:"event_type"`
	Payload    json.RawMessage `json:"payload"`
	RetryCount int             `json:"retry_count"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Repository interface {
	ListSpecialists(ctx context.Context) ([]domain.Specialist, error)
	CreateSpecialist(ctx context.Context, specialist domain.Specialist) (domain.Specialist, error)
	GetSpecialistByID(ctx context.Context, id int64) (domain.Specialist, error)
	CreateClient(ctx context.Context, client domain.Client) (domain.Client, error)
	GetClientByID(ctx context.Context, id int64) (domain.Client, error)
	CreateUser(ctx context.Context, user domain.User) (domain.User, error)
	CreateUserWithClient(ctx context.Context, user domain.User, client domain.Client) (domain.User, error)
	CreateUserWithSpecialist(ctx context.Context, user domain.User, specialist domain.Specialist) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	GetUserByID(ctx context.Context, id int64) (domain.User, error)
	CreateSchedule(ctx context.Context, schedule domain.Schedule) (domain.Schedule, error)
	UpsertSchedule(ctx context.Context, schedule domain.Schedule) (domain.Schedule, error)
	GetScheduleByDate(ctx context.Context, specialistID int64, workDate time.Time) (domain.Schedule, error)
	ListBookedAppointmentsByRange(ctx context.Context, specialistID int64, from time.Time, to time.Time) ([]domain.Appointment, error)
	GetAppointmentByID(ctx context.Context, id int64) (domain.Appointment, error)
	CreateAppointment(ctx context.Context, appt domain.Appointment, event OutboxEvent) (domain.Appointment, error)
	CancelAppointment(ctx context.Context, id int64, reason string, event OutboxEvent) (domain.Appointment, error)
	RescheduleAppointment(ctx context.Context, id int64, newStart time.Time, newEnd time.Time, event OutboxEvent) (domain.Appointment, error)
}

type OutboxRepository interface {
	ClaimOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkOutboxEventPublished(ctx context.Context, id int64) error
	ReleaseOutboxEvent(ctx context.Context, id int64, errText string) error
}
