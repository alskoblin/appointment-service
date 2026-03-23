package util

import (
	"fmt"
	"time"
)

const DateLayout = "2006-01-02"

func ParseDate(value string) (time.Time, error) {
	t, err := time.Parse(DateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
	}
	return t, nil
}

func MinuteToHHMM(minute int) string {
	h := minute / 60
	m := minute % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func ParseHHMM(value string) (int, error) {
	if value == "24:00" {
		return 1440, nil
	}
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, fmt.Errorf("invalid time format, expected HH:MM: %w", err)
	}
	return t.Hour()*60 + t.Minute(), nil
}
