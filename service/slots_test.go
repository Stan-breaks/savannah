package service

import (
	"database/sql"
	"errors"
	"savannah/database"
	"testing"
	"time"
)

// testDate is an arbitrary date used by the slot tests. The tests derive the
// weekday from it so they do not depend on a hand written day number.
const testDate = "2026-10-22"

func weekdayOf(t *testing.T, date string) int {
	t.Helper()
	day, err := time.Parse(DateFormat, date)
	if err != nil {
		t.Fatalf("parse date %q: %v", date, err)
	}
	return int(day.Weekday())
}

// workingHour builds a working hours row for the tests.
func workingHour(day int, start, end, lunchStart, lunchEnd string) database.WorkingHour {
	wh := database.WorkingHour{
		ID:        1,
		DayOfWeek: int64(day),
		StartTime: start,
		EndTime:   end,
	}
	if lunchStart != "" {
		wh.LunchStart = sql.NullString{String: lunchStart, Valid: true}
	}
	if lunchEnd != "" {
		wh.LunchEnd = sql.NullString{String: lunchEnd, Valid: true}
	}
	return wh
}

// startTimes collects the start time of every generated slot.
func startTimes(specs []SlotSpec) []string {
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.StartTime)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGenerateSlotsSplitsTheShiftInto30MinuteSlots(t *testing.T) {
	day := weekdayOf(t, testDate)
	hours := []database.WorkingHour{workingHour(day, "09:00", "12:00", "", "")}

	specs, err := GenerateSlots(hours, testDate)
	if err != nil {
		t.Fatalf("GenerateSlots: %v", err)
	}

	want := []string{"09:00", "09:30", "10:00", "10:30", "11:00", "11:30"}
	if got := startTimes(specs); !equalStrings(got, want) {
		t.Fatalf("start times = %v, want %v", got, want)
	}
	if specs[0].EndTime != "09:30" {
		t.Errorf("first slot ends at %s, want 09:30", specs[0].EndTime)
	}
	if specs[0].Date != testDate {
		t.Errorf("first slot is on %s, want %s", specs[0].Date, testDate)
	}
}

func TestGenerateSlotsSkipsTheLunchBreak(t *testing.T) {
	day := weekdayOf(t, testDate)
	hours := []database.WorkingHour{workingHour(day, "09:00", "17:00", "13:00", "14:00")}

	specs, err := GenerateSlots(hours, testDate)
	if err != nil {
		t.Fatalf("GenerateSlots: %v", err)
	}

	want := []string{
		"09:00", "09:30", "10:00", "10:30", "11:00", "11:30", "12:00", "12:30",
		"14:00", "14:30", "15:00", "15:30", "16:00", "16:30",
	}
	if got := startTimes(specs); !equalStrings(got, want) {
		t.Fatalf("start times = %v, want %v", got, want)
	}
}

func TestGenerateSlotsIgnoresOtherWeekdays(t *testing.T) {
	day := weekdayOf(t, testDate)
	hours := []database.WorkingHour{workingHour((day+1)%7, "09:00", "17:00", "", "")}

	specs, err := GenerateSlots(hours, testDate)
	if err != nil {
		t.Fatalf("GenerateSlots: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("got %d slots, want 0", len(specs))
	}
}

func TestGenerateSlotsSupportsSplitShifts(t *testing.T) {
	day := weekdayOf(t, testDate)
	hours := []database.WorkingHour{
		workingHour(day, "09:00", "12:00", "", ""),
		workingHour(day, "14:00", "15:00", "", ""),
	}

	specs, err := GenerateSlots(hours, testDate)
	if err != nil {
		t.Fatalf("GenerateSlots: %v", err)
	}

	want := []string{"09:00", "09:30", "10:00", "10:30", "11:00", "11:30", "14:00", "14:30"}
	if got := startTimes(specs); !equalStrings(got, want) {
		t.Fatalf("start times = %v, want %v", got, want)
	}
}

func TestGenerateSlotsRejectsIncompleteWorkingHours(t *testing.T) {
	day := weekdayOf(t, testDate)

	t.Run("shift ends before it starts", func(t *testing.T) {
		hours := []database.WorkingHour{workingHour(day, "17:00", "09:00", "", "")}
		if _, err := GenerateSlots(hours, testDate); !errors.Is(err, ErrValidation) {
			t.Fatalf("error = %v, want a validation error", err)
		}
	})

	t.Run("half a lunch break", func(t *testing.T) {
		hours := []database.WorkingHour{workingHour(day, "09:00", "17:00", "13:00", "")}
		if _, err := GenerateSlots(hours, testDate); !errors.Is(err, ErrValidation) {
			t.Fatalf("error = %v, want a validation error", err)
		}
	})

	t.Run("malformed date", func(t *testing.T) {
		hours := []database.WorkingHour{workingHour(day, "09:00", "17:00", "", "")}
		if _, err := GenerateSlots(hours, "22-10-2026"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("error = %v, want an invalid request error", err)
		}
	})
}

func TestValidateSlot(t *testing.T) {
	day := weekdayOf(t, testDate)
	hours := []database.WorkingHour{workingHour(day, "09:00", "17:00", "13:00", "14:00")}

	tests := []struct {
		name      string
		date      string
		startTime string
		wantCode  string
	}{
		{name: "a slot inside the shift", date: testDate, startTime: "09:00"},
		{name: "the last slot of the shift", date: testDate, startTime: "16:30"},
		{name: "the slot right after lunch", date: testDate, startTime: "14:00"},
		{name: "before the shift starts", date: testDate, startTime: "08:30", wantCode: "outside_working_hours"},
		{name: "running past the end of the shift", date: testDate, startTime: "17:00", wantCode: "outside_working_hours"},
		{name: "inside the lunch break", date: testDate, startTime: "13:00", wantCode: "slot_in_lunch_break"},
		{name: "not on the 30 minute grid", date: testDate, startTime: "09:15", wantCode: "slot_not_aligned"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSlot(hours, tt.date, tt.startTime)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("ValidateSlot: %v", err)
				}
				return
			}

			var svcErr *Error
			if !errors.As(err, &svcErr) {
				t.Fatalf("error = %v, want a service error", err)
			}
			if svcErr.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q (message: %s)", svcErr.Code, tt.wantCode, svcErr.Message)
			}
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v, want a validation error", err)
			}
		})
	}
}

func TestValidateSlotRejectsDaysTheDoctorDoesNotWork(t *testing.T) {
	day := weekdayOf(t, testDate)
	hours := []database.WorkingHour{workingHour(day, "09:00", "17:00", "", "")}

	other := "2026-10-23"
	for weekdayOf(t, other) == day {
		other = "2026-10-24"
	}

	err := ValidateSlot(hours, other, "09:00")
	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("error = %v, want a service error", err)
	}
	if svcErr.Code != "doctor_not_working" {
		t.Fatalf("code = %q, want doctor_not_working", svcErr.Code)
	}
}
