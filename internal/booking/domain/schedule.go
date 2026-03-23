package domain

import "time"

type Schedule struct {
	ID               int64     `json:"id"`
	SpecialistID     int64     `json:"specialist_id"`
	WorkDate         time.Time `json:"work_date"`
	StartMinute      int       `json:"start_minute"`
	EndMinute        int       `json:"end_minute"`
	BreakStartMinute *int      `json:"break_start_minute,omitempty"`
	BreakEndMinute   *int      `json:"break_end_minute,omitempty"`
	IsDayOff         bool      `json:"is_day_off"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
