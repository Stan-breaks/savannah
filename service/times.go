package service

import (
	"time"
)

const (
	// DateFormat is the layout of the `date` column.
	DateFormat = "2006-01-02"
	// TimeFormat is the layout of the `start_time` and `end_time` columns.
	TimeFormat = "15:04"
	// SlotDuration is the length of a single appointment slot.
	SlotDuration = 30 * time.Minute
	// SlotGridMinutes is the spacing between the start of two consecutive
	// slots.
	SlotGridMinutes = 30
)

// ParseDate parses a YYYY-MM-DD date.
func ParseDate(value string) (time.Time, error) {
	day, err := time.Parse(DateFormat, value)
	if err != nil {
		return time.Time{}, invalid("invalid_date", "date must be formatted as YYYY-MM-DD, got %q", value)
	}
	return day, nil
}

// ParseTimeOfDay parses a HH:MM clock time.
func ParseTimeOfDay(value string) (time.Time, error) {
	t, err := time.Parse(TimeFormat, value)
	if err != nil {
		return time.Time{}, invalid("invalid_time", "time must be formatted as HH:MM, got %q", value)
	}
	return t, nil
}

// formatTime renders a clock time in the layout used by the database.
func formatTime(t time.Time) string {
	return t.Format(TimeFormat)
}

// clockTime parses a HH:MM clock time and reports whether it is set.
func clockTime(value string) (time.Time, bool, error) {
	if value == "" {
		return time.Time{}, false, nil
	}
	t, err := ParseTimeOfDay(value)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}
