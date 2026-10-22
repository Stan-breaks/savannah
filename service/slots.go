package service

import (
	"database/sql"
	"savannah/database"
	"sort"
	"time"
)

// SlotSpec is a bookable window on a given date, derived from a doctor's
// working hours.
type SlotSpec struct {
	Date      string `json:"date"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// shift is one contiguous stretch of working time on a single weekday.
type shift struct {
	start      time.Time
	end        time.Time
	lunchStart time.Time
	lunchEnd   time.Time
	hasLunch   bool
}

// overlapsLunch reports whether [start,end) collides with the lunch break.
func (s shift) overlapsLunch(start, end time.Time) bool {
	if !s.hasLunch {
		return false
	}
	return start.Before(s.lunchEnd) && end.After(s.lunchStart)
}

// shiftsForWeekday collects the shifts a doctor works on the given weekday.
// Weekdays follow the SQLite convention where 0 is Sunday.
func shiftsForWeekday(workingHours []database.WorkingHour, weekday int) ([]shift, error) {
	var shifts []shift
	for _, wh := range workingHours {
		if int(wh.DayOfWeek) != weekday {
			continue
		}

		start, err := ParseTimeOfDay(wh.StartTime)
		if err != nil {
			return nil, err
		}
		end, err := ParseTimeOfDay(wh.EndTime)
		if err != nil {
			return nil, err
		}
		if !end.After(start) {
			return nil, validation("invalid_working_hours",
				"working hours %d end at %s, before they start at %s", wh.ID, wh.EndTime, wh.StartTime)
		}

		lunchStart, lunchEnd, hasLunch, err := lunchBreak(wh)
		if err != nil {
			return nil, err
		}

		shifts = append(shifts, shift{
			start:      start,
			end:        end,
			lunchStart: lunchStart,
			lunchEnd:   lunchEnd,
			hasLunch:   hasLunch,
		})
	}
	return shifts, nil
}

// lunchBreak reads the optional lunch break of a working hours row.
func lunchBreak(wh database.WorkingHour) (start, end time.Time, hasLunch bool, err error) {
	lunchStart, hasStart, err := clockTime(nullString(wh.LunchStart))
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	lunchEnd, hasEnd, err := clockTime(nullString(wh.LunchEnd))
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}

	switch {
	case !hasStart && !hasEnd:
		return time.Time{}, time.Time{}, false, nil
	case hasStart != hasEnd:
		return time.Time{}, time.Time{}, false, validation("invalid_working_hours",
			"working hours %d must define both lunch_start and lunch_end", wh.ID)
	case !lunchEnd.After(lunchStart):
		return time.Time{}, time.Time{}, false, validation("invalid_working_hours",
			"the lunch break of working hours %d ends before it starts", wh.ID)
	}
	return lunchStart, lunchEnd, true, nil
}

// GenerateSlots derives the 30 minute slots implied by the given working
// hours for the supplied date. Working hours belonging to other weekdays are
// ignored and lunch breaks are carved out of the resulting schedule.
func GenerateSlots(workingHours []database.WorkingHour, date string) ([]SlotSpec, error) {
	day, err := ParseDate(date)
	if err != nil {
		return nil, err
	}

	shifts, err := shiftsForWeekday(workingHours, int(day.Weekday()))
	if err != nil {
		return nil, err
	}

	var specs []SlotSpec
	seen := make(map[string]bool)
	for _, sh := range shifts {
		for start := sh.start; !start.After(sh.end.Add(-SlotDuration)); start = start.Add(SlotDuration) {
			end := start.Add(SlotDuration)
			if sh.overlapsLunch(start, end) {
				continue
			}
			key := formatTime(start)
			if seen[key] {
				continue
			}
			seen[key] = true
			specs = append(specs, SlotSpec{
				Date:      date,
				StartTime: key,
				EndTime:   formatTime(end),
			})
		}
	}

	sort.Slice(specs, func(i, j int) bool {
		return specs[i].StartTime < specs[j].StartTime
	})
	return specs, nil
}

// ValidateSlot checks that the requested start time is a slot the doctor
// actually works: it has to fit inside one of their shifts, avoid the lunch
// break and sit on the 30 minute grid the shifts are laid out on.
func ValidateSlot(workingHours []database.WorkingHour, date, startTime string) error {
	day, err := ParseDate(date)
	if err != nil {
		return err
	}
	start, err := ParseTimeOfDay(startTime)
	if err != nil {
		return err
	}
	end := start.Add(SlotDuration)

	shifts, err := shiftsForWeekday(workingHours, int(day.Weekday()))
	if err != nil {
		return err
	}
	if len(shifts) == 0 {
		return validation("doctor_not_working", "the doctor does not work on %s", date)
	}

	for _, sh := range shifts {
		if start.Before(sh.start) || end.After(sh.end) {
			continue
		}
		if sh.overlapsLunch(start, end) {
			return validation("slot_in_lunch_break", "%s falls inside the doctor's lunch break on %s", startTime, date)
		}
		if offset := int(start.Sub(sh.start).Minutes()); offset%SlotGridMinutes != 0 {
			return validation("slot_not_aligned",
				"%s is not on a %d minute boundary, appointments start on :00 or :30", startTime, SlotGridMinutes)
		}
		return nil
	}

	return validation("outside_working_hours",
		"%s-%s falls outside the doctor's working hours on %s", startTime, formatTime(end), date)
}

// nullString unwraps a nullable text column.
func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
