package service

import (
	"context"
	"fmt"
	"savannah/database"
)

// EnsureSlots reconciles the slots table with the schedule implied by the
// doctor's working hours for the given date and returns every slot stored
// for that doctor and date.
//
// The slots table is the record of what can actually be booked, so it is
// brought in line with the working hours whenever a schedule is consulted:
// slots that dropped out of the schedule are removed unless an appointment
// still refers to them, slots that are missing are inserted, and the
// availability flag is kept in sync with the appointments that exist.
func (s *Service) EnsureSlots(ctx context.Context, doctorID int64, date string) ([]database.Slot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	slots, err := s.ensureSlots(ctx, s.queries.WithTx(tx), doctorID, date)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}
	return slots, nil
}

// ensureSlots performs the reconciliation inside the caller's transaction so
// that booking and rescheduling can materialise the slot they need and use
// it straight away.
func (s *Service) ensureSlots(ctx context.Context, q *database.Queries, doctorID int64, date string) ([]database.Slot, error) {
	workingHours, err := q.GetWorkingHoursByDoctor(ctx, doctorID)
	if err != nil {
		return nil, fmt.Errorf("load working hours: %w", err)
	}
	specs, err := GenerateSlots(workingHours, date)
	if err != nil {
		return nil, err
	}

	existing, err := q.GetSlotsByDoctorAndDate(ctx, database.GetSlotsByDoctorAndDateParams{
		DoctorID: doctorID,
		Date:     date,
	})
	if err != nil {
		return nil, fmt.Errorf("load slots: %w", err)
	}

	bookings, err := q.GetBookingsWithSlotByDoctorAndDate(ctx, database.GetBookingsWithSlotByDoctorAndDateParams{
		DoctorID: doctorID,
		Date:     date,
	})
	if err != nil {
		return nil, fmt.Errorf("load bookings: %w", err)
	}

	confirmed := make(map[int64]bool, len(bookings))
	referenced := make(map[int64]bool, len(bookings))
	for _, booking := range bookings {
		referenced[booking.SlotID] = true
		if booking.Status == BookingStatusConfirmed {
			confirmed[booking.SlotID] = true
		}
	}

	wanted := make(map[string]SlotSpec, len(specs))
	for _, spec := range specs {
		wanted[spec.StartTime] = spec
	}

	stored := make(map[string]bool, len(existing))
	for _, slot := range existing {
		stored[slot.StartTime] = true

		if _, ok := wanted[slot.StartTime]; !ok {
			if referenced[slot.ID] {
				// an appointment still points at the slot, keep the history
				continue
			}
			if err := q.DeleteSlot(ctx, slot.ID); err != nil {
				return nil, fmt.Errorf("delete stale slot %d: %w", slot.ID, err)
			}
			delete(stored, slot.StartTime)
			continue
		}

		available := int64(1)
		if confirmed[slot.ID] {
			available = 0
		}
		if slot.IsAvailable != available {
			if err := q.UpdateSlotAvailability(ctx, database.UpdateSlotAvailabilityParams{
				IsAvailable: available,
				ID:          slot.ID,
			}); err != nil {
				return nil, fmt.Errorf("update availability of slot %d: %w", slot.ID, err)
			}
		}
	}

	for _, spec := range specs {
		if stored[spec.StartTime] {
			continue
		}
		err := q.CreateSlotIgnoringConflict(ctx, database.CreateSlotIgnoringConflictParams{
			StartTime: spec.StartTime,
			EndTime:   spec.EndTime,
			Date:      spec.Date,
			DoctorID:  doctorID,
		})
		if err != nil {
			return nil, fmt.Errorf("create slot %s: %w", spec.StartTime, err)
		}
	}

	return q.GetSlotsByDoctorAndDate(ctx, database.GetSlotsByDoctorAndDateParams{
		DoctorID: doctorID,
		Date:     date,
	})
}
