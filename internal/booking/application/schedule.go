package application

import (
	"context"
	"time"

	"appointment-service/internal/apperr"
	"appointment-service/internal/booking/domain"
)

func (s *Service) CreateSchedule(ctx context.Context, input SaveScheduleInput) (domain.Schedule, error) {
	if err := validateScheduleInput(input); err != nil {
		return domain.Schedule{}, err
	}
	if _, err := s.repo.GetSpecialistByID(ctx, input.SpecialistID); err != nil {
		return domain.Schedule{}, err
	}

	return s.repo.CreateSchedule(ctx, domain.Schedule{
		SpecialistID:     input.SpecialistID,
		WorkDate:         normalizeDate(input.WorkDate),
		StartMinute:      input.StartMinute,
		EndMinute:        input.EndMinute,
		BreakStartMinute: input.BreakStartMinute,
		BreakEndMinute:   input.BreakEndMinute,
		IsDayOff:         input.IsDayOff,
	})
}

func (s *Service) UpsertSchedule(ctx context.Context, input SaveScheduleInput) (domain.Schedule, error) {
	if err := validateScheduleInput(input); err != nil {
		return domain.Schedule{}, err
	}
	if _, err := s.repo.GetSpecialistByID(ctx, input.SpecialistID); err != nil {
		return domain.Schedule{}, err
	}

	return s.repo.UpsertSchedule(ctx, domain.Schedule{
		SpecialistID:     input.SpecialistID,
		WorkDate:         normalizeDate(input.WorkDate),
		StartMinute:      input.StartMinute,
		EndMinute:        input.EndMinute,
		BreakStartMinute: input.BreakStartMinute,
		BreakEndMinute:   input.BreakEndMinute,
		IsDayOff:         input.IsDayOff,
	})
}

func (s *Service) GetSpecialistSchedule(ctx context.Context, specialistID int64, date time.Time) (ScheduleView, error) {
	specialist, err := s.repo.GetSpecialistByID(ctx, specialistID)
	if err != nil {
		return ScheduleView{}, err
	}

	loc := specialistLocation(specialist)
	workDate := normalizeDate(date)

	schedule, err := s.repo.GetScheduleByDate(ctx, specialistID, workDate)
	if err != nil {
		return ScheduleView{}, err
	}

	fromUTC, toUTC := dayRangeUTC(workDate, loc)
	appointments, err := s.repo.ListBookedAppointmentsByRange(ctx, specialistID, fromUTC, toUTC)
	if err != nil {
		return ScheduleView{}, err
	}

	return ScheduleView{Specialist: specialist, Schedule: schedule, Appointments: appointments}, nil
}

func (s *Service) GetFreeSlots(ctx context.Context, specialistID int64, date time.Time) (FreeSlotsView, error) {
	view, err := s.GetSpecialistSchedule(ctx, specialistID, date)
	if err != nil {
		return FreeSlotsView{}, err
	}

	if view.Schedule.IsDayOff {
		return FreeSlotsView{
			SpecialistID:        specialistID,
			Date:                view.Schedule.WorkDate,
			SlotDurationMinutes: view.Specialist.SlotDurationMinutes,
			FreeSlots:           []domain.TimeSlot{},
		}, nil
	}

	loc := specialistLocation(view.Specialist)
	workDate := normalizeDate(date)
	free := make([]domain.TimeSlot, 0)
	duration := time.Duration(view.Specialist.SlotDurationMinutes) * time.Minute

	for minute := view.Schedule.StartMinute; minute+view.Specialist.SlotDurationMinutes <= view.Schedule.EndMinute; minute += view.Specialist.SlotDurationMinutes {
		slotLocalStart := time.Date(workDate.Year(), workDate.Month(), workDate.Day(), 0, 0, 0, 0, loc).Add(time.Duration(minute) * time.Minute)
		slotLocalEnd := slotLocalStart.Add(duration)
		if inBreak(view.Schedule, minute, minute+view.Specialist.SlotDurationMinutes) {
			continue
		}

		slotUTCStart := slotLocalStart.UTC()
		slotUTCEnd := slotLocalEnd.UTC()
		busy := false
		for _, a := range view.Appointments {
			if intervalsOverlap(slotUTCStart, slotUTCEnd, a.StartTime, a.EndTime) {
				busy = true
				break
			}
		}
		if !busy {
			free = append(free, domain.TimeSlot{Start: slotUTCStart, End: slotUTCEnd})
		}
	}

	return FreeSlotsView{
		SpecialistID:        specialistID,
		Date:                view.Schedule.WorkDate,
		SlotDurationMinutes: view.Specialist.SlotDurationMinutes,
		FreeSlots:           free,
	}, nil
}

func validateScheduleInput(input SaveScheduleInput) error {
	if input.SpecialistID <= 0 {
		return apperr.Validation("specialist_id must be positive")
	}
	if input.WorkDate.IsZero() {
		return apperr.Validation("date is required")
	}
	if input.StartMinute < 0 || input.StartMinute >= 1440 {
		return apperr.Validation("start must be in HH:MM range 00:00-23:59")
	}
	if input.EndMinute <= 0 || input.EndMinute > 1440 {
		return apperr.Validation("end must be in HH:MM range 00:01-24:00")
	}
	if input.StartMinute >= input.EndMinute {
		return apperr.Validation("start must be earlier than end")
	}

	if (input.BreakStartMinute == nil) != (input.BreakEndMinute == nil) {
		return apperr.Validation("break_start and break_end must be provided together")
	}
	if input.BreakStartMinute != nil && input.BreakEndMinute != nil {
		breakStart := *input.BreakStartMinute
		breakEnd := *input.BreakEndMinute
		if breakStart < input.StartMinute || breakEnd > input.EndMinute {
			return apperr.Validation("break must be inside working hours")
		}
		if breakStart >= breakEnd {
			return apperr.Validation("break_start must be earlier than break_end")
		}
	}
	return nil
}

func specialistLocation(specialist domain.Specialist) *time.Location {
	loc, err := time.LoadLocation(specialist.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func normalizeDate(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
}

func localDateInLocation(ts time.Time, loc *time.Location) time.Time {
	inLoc := ts.In(loc)
	return time.Date(inLoc.Year(), inLoc.Month(), inLoc.Day(), 0, 0, 0, 0, time.UTC)
}

func dayRangeUTC(workDate time.Time, loc *time.Location) (time.Time, time.Time) {
	localDayStart := time.Date(workDate.Year(), workDate.Month(), workDate.Day(), 0, 0, 0, 0, loc)
	localDayEnd := localDayStart.Add(24 * time.Hour)
	return localDayStart.UTC(), localDayEnd.UTC()
}

func intervalsOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

func inBreak(schedule domain.Schedule, startMinute int, endMinute int) bool {
	if schedule.BreakStartMinute == nil || schedule.BreakEndMinute == nil {
		return false
	}
	return startMinute < *schedule.BreakEndMinute && *schedule.BreakStartMinute < endMinute
}
