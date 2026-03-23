package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"appointment-service/internal/apperr"
	"appointment-service/internal/booking/application"
	"appointment-service/internal/booking/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListSpecialists(ctx context.Context) ([]domain.Specialist, error) {
	const q = `
		SELECT id, full_name, profession, slot_duration_minutes, timezone, created_at, updated_at
		FROM specialists
		ORDER BY id`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, apperr.Internal("failed to list specialists", err)
	}
	defer rows.Close()

	result := make([]domain.Specialist, 0)
	for rows.Next() {
		var s domain.Specialist
		err = rows.Scan(&s.ID, &s.FullName, &s.Profession, &s.SlotDurationMinutes, &s.Timezone, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, apperr.Internal("failed to scan specialists", err)
		}
		result = append(result, s)
	}
	if rows.Err() != nil {
		return nil, apperr.Internal("failed while iterating specialists", rows.Err())
	}
	return result, nil
}

func (r *Repository) CreateSpecialist(ctx context.Context, specialist domain.Specialist) (domain.Specialist, error) {
	const q = `
		INSERT INTO specialists (full_name, profession, slot_duration_minutes, timezone)
		VALUES ($1, $2, $3, $4)
		RETURNING id, full_name, profession, slot_duration_minutes, timezone, created_at, updated_at`

	var created domain.Specialist
	err := r.db.QueryRow(ctx, q, specialist.FullName, specialist.Profession, specialist.SlotDurationMinutes, specialist.Timezone).
		Scan(&created.ID, &created.FullName, &created.Profession, &created.SlotDurationMinutes, &created.Timezone, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return domain.Specialist{}, apperr.Internal("failed to create specialist", err)
	}
	return created, nil
}

func (r *Repository) GetSpecialistByID(ctx context.Context, id int64) (domain.Specialist, error) {
	const q = `
		SELECT id, full_name, profession, slot_duration_minutes, timezone, created_at, updated_at
		FROM specialists
		WHERE id = $1`

	var s domain.Specialist
	err := r.db.QueryRow(ctx, q, id).Scan(&s.ID, &s.FullName, &s.Profession, &s.SlotDurationMinutes, &s.Timezone, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Specialist{}, apperr.NotFound("specialist not found")
		}
		return domain.Specialist{}, apperr.Internal("failed to get specialist", err)
	}
	return s, nil
}

func (r *Repository) GetClientByID(ctx context.Context, id int64) (domain.Client, error) {
	const q = `
		SELECT id, full_name, phone, telegram_chat_id, created_at
		FROM clients
		WHERE id = $1`

	var c domain.Client
	err := r.db.QueryRow(ctx, q, id).Scan(&c.ID, &c.FullName, &c.Phone, &c.TelegramChatID, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Client{}, apperr.NotFound("client not found")
		}
		return domain.Client{}, apperr.Internal("failed to get client", err)
	}
	return c, nil
}

func (r *Repository) CreateClient(ctx context.Context, client domain.Client) (domain.Client, error) {
	const q = `
		INSERT INTO clients (full_name, phone, telegram_chat_id)
		VALUES ($1, $2, $3)
		RETURNING id, full_name, phone, telegram_chat_id, created_at`

	var created domain.Client
	err := r.db.QueryRow(ctx, q, client.FullName, client.Phone, client.TelegramChatID).
		Scan(&created.ID, &created.FullName, &created.Phone, &created.TelegramChatID, &created.CreatedAt)
	if err != nil {
		return domain.Client{}, apperr.Internal("failed to create client", err)
	}
	return created, nil
}

func (r *Repository) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	const q = `
		INSERT INTO users (email, password_hash, role, client_id, specialist_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, email, password_hash, role, client_id, specialist_id, created_at, updated_at`

	created, err := scanUser(r.db.QueryRow(ctx, q, user.Email, user.PasswordHash, user.Role, user.ClientID, user.SpecialistID))
	if err != nil {
		if mapped := mapUserWriteError(err); mapped != nil {
			return domain.User{}, mapped
		}
		return domain.User{}, apperr.Internal("failed to create user", err)
	}
	return created, nil
}

func (r *Repository) CreateUserWithClient(ctx context.Context, user domain.User, client domain.Client) (domain.User, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return domain.User{}, apperr.Internal("failed to start transaction", err)
	}
	defer tx.Rollback(ctx)

	const createClientQuery = `
		INSERT INTO clients (full_name, phone, telegram_chat_id)
		VALUES ($1, $2, $3)
		RETURNING id, full_name, phone, telegram_chat_id, created_at`

	var createdClient domain.Client
	err = tx.QueryRow(ctx, createClientQuery, client.FullName, client.Phone, client.TelegramChatID).
		Scan(&createdClient.ID, &createdClient.FullName, &createdClient.Phone, &createdClient.TelegramChatID, &createdClient.CreatedAt)
	if err != nil {
		return domain.User{}, apperr.Internal("failed to create client for user", err)
	}

	user.ClientID = &createdClient.ID
	user.SpecialistID = nil

	const createUserQuery = `
		INSERT INTO users (email, password_hash, role, client_id, specialist_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, email, password_hash, role, client_id, specialist_id, created_at, updated_at`

	createdUser, err := scanUser(tx.QueryRow(ctx, createUserQuery, user.Email, user.PasswordHash, user.Role, user.ClientID, user.SpecialistID))
	if err != nil {
		if mapped := mapUserWriteError(err); mapped != nil {
			return domain.User{}, mapped
		}
		return domain.User{}, apperr.Internal("failed to create user", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, apperr.Internal("failed to commit user+client transaction", err)
	}
	return createdUser, nil
}

func (r *Repository) CreateUserWithSpecialist(ctx context.Context, user domain.User, specialist domain.Specialist) (domain.User, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return domain.User{}, apperr.Internal("failed to start transaction", err)
	}
	defer tx.Rollback(ctx)

	const createSpecialistQuery = `
		INSERT INTO specialists (full_name, profession, slot_duration_minutes, timezone)
		VALUES ($1, $2, $3, $4)
		RETURNING id, full_name, profession, slot_duration_minutes, timezone, created_at, updated_at`

	var createdSpecialist domain.Specialist
	err = tx.QueryRow(ctx, createSpecialistQuery, specialist.FullName, specialist.Profession, specialist.SlotDurationMinutes, specialist.Timezone).
		Scan(
			&createdSpecialist.ID,
			&createdSpecialist.FullName,
			&createdSpecialist.Profession,
			&createdSpecialist.SlotDurationMinutes,
			&createdSpecialist.Timezone,
			&createdSpecialist.CreatedAt,
			&createdSpecialist.UpdatedAt,
		)
	if err != nil {
		return domain.User{}, apperr.Internal("failed to create specialist for user", err)
	}

	user.SpecialistID = &createdSpecialist.ID
	user.ClientID = nil

	const createUserQuery = `
		INSERT INTO users (email, password_hash, role, client_id, specialist_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, email, password_hash, role, client_id, specialist_id, created_at, updated_at`

	createdUser, err := scanUser(tx.QueryRow(ctx, createUserQuery, user.Email, user.PasswordHash, user.Role, user.ClientID, user.SpecialistID))
	if err != nil {
		if mapped := mapUserWriteError(err); mapped != nil {
			return domain.User{}, mapped
		}
		return domain.User{}, apperr.Internal("failed to create user", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, apperr.Internal("failed to commit user+specialist transaction", err)
	}
	return createdUser, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	const q = `
		SELECT id, email, password_hash, role, client_id, specialist_id, created_at, updated_at
		FROM users
		WHERE email = $1`

	user, err := scanUser(r.db.QueryRow(ctx, q, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, apperr.NotFound("user not found")
		}
		return domain.User{}, apperr.Internal("failed to get user by email", err)
	}
	return user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (domain.User, error) {
	const q = `
		SELECT id, email, password_hash, role, client_id, specialist_id, created_at, updated_at
		FROM users
		WHERE id = $1`

	user, err := scanUser(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, apperr.NotFound("user not found")
		}
		return domain.User{}, apperr.Internal("failed to get user by id", err)
	}
	return user, nil
}

func (r *Repository) CreateSchedule(ctx context.Context, schedule domain.Schedule) (domain.Schedule, error) {
	const q = `
		INSERT INTO schedules (
			specialist_id,
			work_date,
			start_minute,
			end_minute,
			break_start_minute,
			break_end_minute,
			is_day_off
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, specialist_id, work_date, start_minute, end_minute, break_start_minute, break_end_minute, is_day_off, created_at, updated_at`

	created, err := scanSchedule(r.db.QueryRow(
		ctx,
		q,
		schedule.SpecialistID,
		schedule.WorkDate,
		schedule.StartMinute,
		schedule.EndMinute,
		schedule.BreakStartMinute,
		schedule.BreakEndMinute,
		schedule.IsDayOff,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return domain.Schedule{}, apperr.Conflict("schedule already exists for this specialist and date")
			case "23503":
				return domain.Schedule{}, apperr.NotFound("specialist not found")
			}
		}
		return domain.Schedule{}, apperr.Internal("failed to create schedule", err)
	}
	return created, nil
}

func (r *Repository) UpsertSchedule(ctx context.Context, schedule domain.Schedule) (domain.Schedule, error) {
	const q = `
		INSERT INTO schedules (
			specialist_id,
			work_date,
			start_minute,
			end_minute,
			break_start_minute,
			break_end_minute,
			is_day_off
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (specialist_id, work_date) DO UPDATE
		SET start_minute = EXCLUDED.start_minute,
		    end_minute = EXCLUDED.end_minute,
		    break_start_minute = EXCLUDED.break_start_minute,
		    break_end_minute = EXCLUDED.break_end_minute,
		    is_day_off = EXCLUDED.is_day_off,
		    updated_at = now()
		RETURNING id, specialist_id, work_date, start_minute, end_minute, break_start_minute, break_end_minute, is_day_off, created_at, updated_at`

	saved, err := scanSchedule(r.db.QueryRow(
		ctx,
		q,
		schedule.SpecialistID,
		schedule.WorkDate,
		schedule.StartMinute,
		schedule.EndMinute,
		schedule.BreakStartMinute,
		schedule.BreakEndMinute,
		schedule.IsDayOff,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return domain.Schedule{}, apperr.NotFound("specialist not found")
		}
		return domain.Schedule{}, apperr.Internal("failed to save schedule", err)
	}
	return saved, nil
}

func (r *Repository) GetScheduleByDate(ctx context.Context, specialistID int64, workDate time.Time) (domain.Schedule, error) {
	const q = `
		SELECT id, specialist_id, work_date, start_minute, end_minute, break_start_minute, break_end_minute, is_day_off, created_at, updated_at
		FROM schedules
		WHERE specialist_id = $1 AND work_date = $2`

	var s domain.Schedule
	err := r.db.QueryRow(ctx, q, specialistID, workDate).Scan(&s.ID, &s.SpecialistID, &s.WorkDate, &s.StartMinute, &s.EndMinute, &s.BreakStartMinute, &s.BreakEndMinute, &s.IsDayOff, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Schedule{}, apperr.NotFound("schedule not found for this date")
		}
		return domain.Schedule{}, apperr.Internal("failed to get schedule", err)
	}
	return s, nil
}

func (r *Repository) ListBookedAppointmentsByRange(ctx context.Context, specialistID int64, from time.Time, to time.Time) ([]domain.Appointment, error) {
	const q = `
		SELECT id, specialist_id, client_id, start_time, end_time, status, cancel_reason, canceled_at, created_at, updated_at
		FROM appointments
		WHERE specialist_id = $1
		  AND status = 'booked'
		  AND start_time < $3
		  AND end_time > $2
		ORDER BY start_time`

	rows, err := r.db.Query(ctx, q, specialistID, from, to)
	if err != nil {
		return nil, apperr.Internal("failed to list appointments", err)
	}
	defer rows.Close()

	result := make([]domain.Appointment, 0)
	for rows.Next() {
		appt, scanErr := scanAppointment(rows)
		if scanErr != nil {
			return nil, apperr.Internal("failed to scan appointment", scanErr)
		}
		result = append(result, appt)
	}
	if rows.Err() != nil {
		return nil, apperr.Internal("failed while iterating appointments", rows.Err())
	}
	return result, nil
}

func (r *Repository) GetAppointmentByID(ctx context.Context, id int64) (domain.Appointment, error) {
	const q = `
		SELECT id, specialist_id, client_id, start_time, end_time, status, cancel_reason, canceled_at, created_at, updated_at
		FROM appointments
		WHERE id = $1`

	appt, err := scanAppointment(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Appointment{}, apperr.NotFound("appointment not found")
		}
		return domain.Appointment{}, apperr.Internal("failed to get appointment", err)
	}
	return appt, nil
}

func (r *Repository) CreateAppointment(ctx context.Context, appt domain.Appointment, event application.OutboxEvent) (domain.Appointment, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to start transaction", err)
	}
	defer tx.Rollback(ctx)

	const q = `
		INSERT INTO appointments (specialist_id, client_id, start_time, end_time, status)
		VALUES ($1, $2, $3, $4, 'booked')
		RETURNING id, specialist_id, client_id, start_time, end_time, status, cancel_reason, canceled_at, created_at, updated_at`

	created, err := scanAppointment(tx.QueryRow(ctx, q, appt.SpecialistID, appt.ClientID, appt.StartTime, appt.EndTime))
	if err != nil {
		if mapped := mapPostgresError(err); mapped != nil {
			return domain.Appointment{}, mapped
		}
		return domain.Appointment{}, apperr.Internal("failed to create appointment", err)
	}

	if event.EventType == domain.AppointmentEventCreated && len(event.Payload) > 0 {
		var payload domain.AppointmentEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil && payload.AppointmentID == 0 {
			payload.AppointmentID = created.ID
			payload.StartTime = created.StartTime
			payload.EndTime = created.EndTime
			if marshaled, marshalErr := json.Marshal(payload); marshalErr == nil {
				event.Payload = marshaled
			}
		}
	}

	if err = insertOutboxEvent(ctx, tx, event); err != nil {
		return domain.Appointment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Appointment{}, apperr.Internal("failed to commit create appointment transaction", err)
	}

	return created, nil
}

func (r *Repository) RescheduleAppointment(ctx context.Context, id int64, newStart time.Time, newEnd time.Time, event application.OutboxEvent) (domain.Appointment, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to start transaction", err)
	}
	defer tx.Rollback(ctx)

	const q = `
		UPDATE appointments
		SET start_time = $2,
		    end_time = $3,
		    updated_at = now()
		WHERE id = $1
		  AND status = 'booked'
		RETURNING id, specialist_id, client_id, start_time, end_time, status, cancel_reason, canceled_at, created_at, updated_at`

	rescheduled, err := scanAppointment(tx.QueryRow(ctx, q, id, newStart, newEnd))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			exists, existsErr := appointmentExists(ctx, tx, id)
			if existsErr != nil {
				return domain.Appointment{}, apperr.Internal("failed to check appointment existence", existsErr)
			}
			if !exists {
				return domain.Appointment{}, apperr.NotFound("appointment not found")
			}
			return domain.Appointment{}, apperr.Conflict("only active appointment can be rescheduled")
		}
		if mapped := mapPostgresError(err); mapped != nil {
			return domain.Appointment{}, mapped
		}
		return domain.Appointment{}, apperr.Internal("failed to reschedule appointment", err)
	}

	if err = insertOutboxEvent(ctx, tx, event); err != nil {
		return domain.Appointment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Appointment{}, apperr.Internal("failed to commit reschedule transaction", err)
	}
	return rescheduled, nil
}

func (r *Repository) CancelAppointment(ctx context.Context, id int64, reason string, event application.OutboxEvent) (domain.Appointment, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return domain.Appointment{}, apperr.Internal("failed to start transaction", err)
	}
	defer tx.Rollback(ctx)

	const q = `
		UPDATE appointments
		SET status = 'canceled',
		    cancel_reason = NULLIF($2, ''),
		    canceled_at = now(),
		    updated_at = now()
		WHERE id = $1
		  AND status = 'booked'
		RETURNING id, specialist_id, client_id, start_time, end_time, status, cancel_reason, canceled_at, created_at, updated_at`

	canceled, err := scanAppointment(tx.QueryRow(ctx, q, id, reason))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			exists, existsErr := appointmentExists(ctx, tx, id)
			if existsErr != nil {
				return domain.Appointment{}, apperr.Internal("failed to check appointment existence", existsErr)
			}
			if !exists {
				return domain.Appointment{}, apperr.NotFound("appointment not found")
			}
			return domain.Appointment{}, apperr.Conflict("appointment already canceled")
		}
		return domain.Appointment{}, apperr.Internal("failed to cancel appointment", err)
	}

	if err = insertOutboxEvent(ctx, tx, event); err != nil {
		return domain.Appointment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Appointment{}, apperr.Internal("failed to commit cancel transaction", err)
	}
	return canceled, nil
}

func (r *Repository) ClaimOutboxEvents(ctx context.Context, limit int) ([]application.OutboxEvent, error) {
	const q = `
		WITH claimed AS (
			UPDATE outbox_events o
			SET locked_at = now(), retry_count = retry_count + 1, updated_at = now()
			WHERE o.id IN (
				SELECT id
				FROM outbox_events
				WHERE published_at IS NULL
				  AND (locked_at IS NULL OR locked_at < now() - interval '1 minute')
				ORDER BY id
				LIMIT $1
				FOR UPDATE SKIP LOCKED
			)
			RETURNING o.id, o.event_key, o.topic, o.event_type, o.payload, o.retry_count, o.created_at
		)
		SELECT id, event_key, topic, event_type, payload, retry_count, created_at
		FROM claimed
		ORDER BY id`

	rows, err := r.db.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	events := make([]application.OutboxEvent, 0)
	for rows.Next() {
		var e application.OutboxEvent
		if err := rows.Scan(&e.ID, &e.EventKey, &e.Topic, &e.EventType, &e.Payload, &e.RetryCount, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, e)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate claimed outbox events: %w", rows.Err())
	}
	return events, nil
}

func (r *Repository) MarkOutboxEventPublished(ctx context.Context, id int64) error {
	const q = `
		UPDATE outbox_events
		SET published_at = now(), locked_at = NULL, last_error = NULL, updated_at = now()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	return nil
}

func (r *Repository) ReleaseOutboxEvent(ctx context.Context, id int64, errText string) error {
	const q = `
		UPDATE outbox_events
		SET locked_at = NULL, last_error = LEFT($2, 1000), updated_at = now()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, q, id, errText)
	if err != nil {
		return fmt.Errorf("release outbox event: %w", err)
	}
	return nil
}

func (r *Repository) EnqueueReminderEvents(ctx context.Context, eventType string, topic string, from time.Time, to time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}

	var reminderColumn string
	var keySuffix string
	switch eventType {
	case domain.AppointmentEventReminder24:
		reminderColumn = "reminder_24h_sent_at"
		keySuffix = "reminder24h"
	case domain.AppointmentEventReminder1:
		reminderColumn = "reminder_1h_sent_at"
		keySuffix = "reminder1h"
	default:
		return 0, fmt.Errorf("unsupported reminder event type: %s", eventType)
	}

	selectQuery := fmt.Sprintf(`
		SELECT a.id, a.specialist_id, s.full_name, a.client_id, c.full_name, c.telegram_chat_id, a.start_time, a.end_time
		FROM appointments a
		JOIN specialists s ON s.id = a.specialist_id
		JOIN clients c ON c.id = a.client_id
		WHERE a.status = 'booked'
		  AND a.start_time > $1
		  AND a.start_time <= $2
		  AND a.%s IS NULL
		ORDER BY a.start_time
		LIMIT $3
		FOR UPDATE OF a SKIP LOCKED`, reminderColumn)

	updateQuery := fmt.Sprintf(`
		UPDATE appointments
		SET %s = now(),
		    updated_at = now()
		WHERE id = $1
		  AND %s IS NULL`, reminderColumn, reminderColumn)

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, fmt.Errorf("begin reminder tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, selectQuery, from, to, limit)
	if err != nil {
		return 0, fmt.Errorf("query due reminders: %w", err)
	}
	defer rows.Close()

	due := make([]dueReminder, 0, limit)
	for rows.Next() {
		var item dueReminder
		if err := rows.Scan(
			&item.AppointmentID,
			&item.SpecialistID,
			&item.SpecialistName,
			&item.ClientID,
			&item.ClientName,
			&item.TelegramChatID,
			&item.StartTime,
			&item.EndTime,
		); err != nil {
			return 0, fmt.Errorf("scan due reminder: %w", err)
		}
		due = append(due, item)
	}
	if rows.Err() != nil {
		return 0, fmt.Errorf("iterate due reminders: %w", rows.Err())
	}

	const insertEventQuery = `
		INSERT INTO outbox_events (event_key, topic, event_type, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_key) DO NOTHING`

	enqueued := 0
	for _, item := range due {
		tag, err := tx.Exec(ctx, updateQuery, item.AppointmentID)
		if err != nil {
			return 0, fmt.Errorf("mark reminder flag for appointment %d: %w", item.AppointmentID, err)
		}
		if tag.RowsAffected() == 0 {
			continue
		}

		eventKey := fmt.Sprintf("appointment.%d.%s", item.AppointmentID, keySuffix)
		payload, err := json.Marshal(domain.AppointmentEventPayload{
			EventKey:       eventKey,
			EventType:      eventType,
			AppointmentID:  item.AppointmentID,
			SpecialistID:   item.SpecialistID,
			SpecialistName: item.SpecialistName,
			ClientID:       item.ClientID,
			ClientName:     item.ClientName,
			TelegramChatID: item.TelegramChatID,
			StartTime:      item.StartTime,
			EndTime:        item.EndTime,
			Status:         string(domain.AppointmentStatusBooked),
			OccurredAt:     time.Now().UTC(),
		})
		if err != nil {
			return 0, fmt.Errorf("marshal reminder payload for appointment %d: %w", item.AppointmentID, err)
		}

		insertTag, err := tx.Exec(ctx, insertEventQuery, eventKey, topic, eventType, payload)
		if err != nil {
			return 0, fmt.Errorf("insert reminder outbox event for appointment %d: %w", item.AppointmentID, err)
		}
		if insertTag.RowsAffected() > 0 {
			enqueued++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit reminder tx: %w", err)
	}
	return enqueued, nil
}

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, event application.OutboxEvent) error {
	const q = `INSERT INTO outbox_events (event_key, topic, event_type, payload) VALUES ($1, $2, $3, $4)`
	if _, err := tx.Exec(ctx, q, event.EventKey, event.Topic, event.EventType, event.Payload); err != nil {
		return apperr.Internal("failed to insert outbox event", err)
	}
	return nil
}

func appointmentExists(ctx context.Context, tx pgx.Tx, id int64) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM appointments WHERE id = $1)`, id).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func mapPostgresError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}

	switch pgErr.Code {
	case "23P01", "23505":
		return apperr.Conflict("selected time slot is already occupied")
	case "23503":
		return apperr.Validation("invalid reference to specialist or client")
	default:
		return apperr.Internal("database operation failed", err)
	}
}

type dueReminder struct {
	AppointmentID  int64
	SpecialistID   int64
	SpecialistName string
	ClientID       int64
	ClientName     string
	TelegramChatID *int64
	StartTime      time.Time
	EndTime        time.Time
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSchedule(s scanner) (domain.Schedule, error) {
	var schedule domain.Schedule
	err := s.Scan(
		&schedule.ID,
		&schedule.SpecialistID,
		&schedule.WorkDate,
		&schedule.StartMinute,
		&schedule.EndMinute,
		&schedule.BreakStartMinute,
		&schedule.BreakEndMinute,
		&schedule.IsDayOff,
		&schedule.CreatedAt,
		&schedule.UpdatedAt,
	)
	return schedule, err
}

func scanAppointment(s scanner) (domain.Appointment, error) {
	var appt domain.Appointment
	err := s.Scan(&appt.ID, &appt.SpecialistID, &appt.ClientID, &appt.StartTime, &appt.EndTime, &appt.Status, &appt.CancelReason, &appt.CanceledAt, &appt.CreatedAt, &appt.UpdatedAt)
	return appt, err
}

func scanUser(s scanner) (domain.User, error) {
	var user domain.User
	var roleRaw string
	err := s.Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&roleRaw,
		&user.ClientID,
		&user.SpecialistID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return domain.User{}, err
	}
	role, parseErr := domain.ParseUserRole(roleRaw)
	if parseErr != nil {
		return domain.User{}, parseErr
	}
	user.Role = role
	return user, nil
}

func mapUserWriteError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}

	switch pgErr.Code {
	case "23505":
		if pgErr.ConstraintName == "users_email_key" {
			return apperr.Conflict("email is already registered")
		}
		return apperr.Conflict("user already exists")
	case "23503":
		return apperr.Validation("invalid user role reference")
	case "23514":
		return apperr.Validation("invalid role binding to specialist/client")
	default:
		return apperr.Internal("database operation failed", err)
	}
}
