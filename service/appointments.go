package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"savannah/database"
	"strings"

	"github.com/mattn/go-sqlite3"
)

// Appointment is the view of a booking the service layer returns.
type Appointment struct {
	ID           int64  `json:"id"`
	Status       string `json:"status"`
	SlotID       int64  `json:"slot_id"`
	PatientID    int64  `json:"patient_id"`
	DoctorID     int64  `json:"doctor_id"`
	DoctorName   string `json:"doctor_name,omitempty"`
	Date         string `json:"date"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	CancelReason string `json:"cancel_reason,omitempty"`
}

// BookRequest describes an attempt to book an appointment.
type BookRequest struct {
	PatientID int64
	DoctorID  int64
	Date      string
	StartTime string
}

// CancelRequest describes a request to cancel an appointment.
type CancelRequest struct {
	BookingID int64
	Reason    string
}

// RescheduleRequest describes a request to move an appointment to another
// slot. DoctorID is optional and defaults to the doctor of the appointment.
type RescheduleRequest struct {
	BookingID int64
	DoctorID  int64
	Date      string
	StartTime string
}

// Availability returns the slots a patient can book for a doctor on a date.
func (s *Service) Availability(ctx context.Context, doctorID int64, date string) ([]database.Slot, error) {
	if doctorID <= 0 {
		return nil, invalid("invalid_doctor_id", "doctor id must be a positive integer")
	}
	if _, err := ParseDate(date); err != nil {
		return nil, err
	}
	if _, err := s.queries.GetDoctorById(ctx, doctorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, notFound("doctor_not_found", "doctor %d does not exist", doctorID)
		}
		return nil, fmt.Errorf("load doctor: %w", err)
	}

	if _, err := s.EnsureSlots(ctx, doctorID, date); err != nil {
		return nil, err
	}

	slots, err := s.queries.GetAvailableSlotsByDoctorAndDate(ctx, database.GetAvailableSlotsByDoctorAndDateParams{
		DoctorID: doctorID,
		Date:     date,
	})
	if err != nil {
		return nil, fmt.Errorf("load available slots: %w", err)
	}
	return s.bookable(slots), nil
}

// Book books a slot for a patient. The slot has to fall inside the doctor's
// working hours, must not have started yet and must still be free.
func (s *Service) Book(ctx context.Context, req BookRequest) (Appointment, error) {
	if req.PatientID <= 0 {
		return Appointment{}, invalid("invalid_patient_id", "patient id must be a positive integer")
	}
	if req.DoctorID <= 0 {
		return Appointment{}, invalid("invalid_doctor_id", "doctor id must be a positive integer")
	}
	if _, err := ParseDate(req.Date); err != nil {
		return Appointment{}, err
	}
	if _, err := ParseTimeOfDay(req.StartTime); err != nil {
		return Appointment{}, err
	}

	doctor, err := s.queries.GetDoctorById(ctx, req.DoctorID)
	if err != nil {
		return Appointment{}, missingResource("doctor", req.DoctorID, err)
	}
	if _, err := s.queries.GetPatientById(ctx, req.PatientID); err != nil {
		return Appointment{}, missingResource("patient", req.PatientID, err)
	}

	workingHours, err := s.queries.GetWorkingHoursByDoctor(ctx, req.DoctorID)
	if err != nil {
		return Appointment{}, fmt.Errorf("load working hours: %w", err)
	}
	if err := ValidateSlot(workingHours, req.Date, req.StartTime); err != nil {
		return Appointment{}, err
	}
	if err := s.checkBookable(req.Date, req.StartTime); err != nil {
		return Appointment{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Appointment{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)

	slot, err := s.claimableSlot(ctx, q, req.DoctorID, req.Date, req.StartTime)
	if err != nil {
		return Appointment{}, err
	}

	booking, err := q.CreateBooking(ctx, database.CreateBookingParams{
		PatientID: req.PatientID,
		ID:        slot.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Appointment{}, conflict("slot_already_booked",
				"slot %s on %s is already booked", req.StartTime, req.Date)
		}
		return Appointment{}, fmt.Errorf("create booking: %w", err)
	}

	if err := q.UpdateSlotAvailability(ctx, database.UpdateSlotAvailabilityParams{
		IsAvailable: 0,
		ID:          slot.ID,
	}); err != nil {
		return Appointment{}, fmt.Errorf("mark slot unavailable: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Appointment{}, fmt.Errorf("commit transaction: %w", err)
	}

	return Appointment{
		ID:         booking.ID,
		Status:     booking.Status,
		SlotID:     slot.ID,
		PatientID:  booking.PatientID,
		DoctorID:   doctor.ID,
		DoctorName: doctor.Name,
		Date:       slot.Date,
		StartTime:  slot.StartTime,
		EndTime:    slot.EndTime,
	}, nil
}

// Cancel cancels an appointment and releases its slot so it can be booked
// again.
func (s *Service) Cancel(ctx context.Context, req CancelRequest) (Appointment, error) {
	if req.BookingID <= 0 {
		return Appointment{}, invalid("invalid_appointment_id", "appointment id must be a positive integer")
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return Appointment{}, invalid("missing_cancel_reason", "a reason is required to cancel an appointment")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Appointment{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)

	booking, err := q.GetBookingWithSlot(ctx, req.BookingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Appointment{}, notFound("appointment_not_found", "appointment %d does not exist", req.BookingID)
		}
		return Appointment{}, fmt.Errorf("load appointment: %w", err)
	}
	if booking.Status != BookingStatusConfirmed {
		return Appointment{}, conflict("appointment_already_cancelled",
			"appointment %d is already cancelled", req.BookingID)
	}

	affected, err := q.CancelBooking(ctx, database.CancelBookingParams{
		CancelReason: sql.NullString{String: reason, Valid: true},
		ID:           req.BookingID,
	})
	if err != nil {
		return Appointment{}, fmt.Errorf("cancel appointment: %w", err)
	}
	if affected == 0 {
		return Appointment{}, conflict("appointment_already_cancelled",
			"appointment %d is already cancelled", req.BookingID)
	}

	if err := q.UpdateSlotAvailability(ctx, database.UpdateSlotAvailabilityParams{
		IsAvailable: 1,
		ID:          booking.SlotID,
	}); err != nil {
		return Appointment{}, fmt.Errorf("release slot: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Appointment{}, fmt.Errorf("commit transaction: %w", err)
	}

	return Appointment{
		ID:           booking.ID,
		Status:       BookingStatusCancelled,
		SlotID:       booking.SlotID,
		PatientID:    booking.PatientID,
		DoctorID:     booking.DoctorID,
		Date:         booking.Date,
		StartTime:    booking.StartTime,
		EndTime:      booking.EndTime,
		CancelReason: reason,
	}, nil
}

// Reschedule moves an appointment to another slot. The slot it came from is
// released and the new slot is validated the same way a fresh booking is.
func (s *Service) Reschedule(ctx context.Context, req RescheduleRequest) (Appointment, error) {
	if req.BookingID <= 0 {
		return Appointment{}, invalid("invalid_appointment_id", "appointment id must be a positive integer")
	}
	if _, err := ParseDate(req.Date); err != nil {
		return Appointment{}, err
	}
	if _, err := ParseTimeOfDay(req.StartTime); err != nil {
		return Appointment{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Appointment{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)

	booking, err := q.GetBookingWithSlot(ctx, req.BookingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Appointment{}, notFound("appointment_not_found", "appointment %d does not exist", req.BookingID)
		}
		return Appointment{}, fmt.Errorf("load appointment: %w", err)
	}
	if booking.Status != BookingStatusConfirmed {
		return Appointment{}, conflict("appointment_already_cancelled",
			"appointment %d is cancelled and cannot be rescheduled", req.BookingID)
	}

	doctorID := req.DoctorID
	if doctorID == 0 {
		doctorID = booking.DoctorID
	}
	doctor, err := q.GetDoctorById(ctx, doctorID)
	if err != nil {
		return Appointment{}, missingResource("doctor", doctorID, err)
	}

	workingHours, err := q.GetWorkingHoursByDoctor(ctx, doctorID)
	if err != nil {
		return Appointment{}, fmt.Errorf("load working hours: %w", err)
	}
	if err := ValidateSlot(workingHours, req.Date, req.StartTime); err != nil {
		return Appointment{}, err
	}
	if err := s.checkBookable(req.Date, req.StartTime); err != nil {
		return Appointment{}, err
	}

	slots, err := s.ensureSlots(ctx, q, doctorID, req.Date)
	if err != nil {
		return Appointment{}, err
	}
	slot, ok := findSlot(slots, req.StartTime)
	if !ok {
		return Appointment{}, validation("slot_not_available",
			"%s is not a slot of doctor %d on %s", req.StartTime, doctorID, req.Date)
	}
	if slot.ID == booking.SlotID {
		return Appointment{}, validation("same_slot",
			"appointment %d already sits in slot %s on %s", req.BookingID, req.StartTime, req.Date)
	}
	if slot.IsAvailable == 0 {
		return Appointment{}, conflict("slot_already_booked",
			"slot %s on %s is already booked", req.StartTime, req.Date)
	}

	if _, err := q.RescheduleBooking(ctx, database.RescheduleBookingParams{
		SlotID: slot.ID,
		ID:     req.BookingID,
	}); err != nil {
		if isUniqueViolation(err) {
			return Appointment{}, conflict("slot_already_booked",
				"slot %s on %s is already booked", req.StartTime, req.Date)
		}
		return Appointment{}, fmt.Errorf("reschedule appointment: %w", err)
	}

	if err := q.UpdateSlotAvailability(ctx, database.UpdateSlotAvailabilityParams{
		IsAvailable: 1,
		ID:          booking.SlotID,
	}); err != nil {
		return Appointment{}, fmt.Errorf("release slot: %w", err)
	}
	if err := q.UpdateSlotAvailability(ctx, database.UpdateSlotAvailabilityParams{
		IsAvailable: 0,
		ID:          slot.ID,
	}); err != nil {
		return Appointment{}, fmt.Errorf("occupy slot: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Appointment{}, fmt.Errorf("commit transaction: %w", err)
	}

	return Appointment{
		ID:         booking.ID,
		Status:     BookingStatusConfirmed,
		SlotID:     slot.ID,
		PatientID:  booking.PatientID,
		DoctorID:   doctor.ID,
		DoctorName: doctor.Name,
		Date:       slot.Date,
		StartTime:  slot.StartTime,
		EndTime:    slot.EndTime,
	}, nil
}

// PatientAppointments returns the upcoming appointments of a patient,
// ordered by date and start time.
func (s *Service) PatientAppointments(ctx context.Context, patientID int64) ([]Appointment, error) {
	if patientID <= 0 {
		return nil, invalid("invalid_patient_id", "patient id must be a positive integer")
	}
	if _, err := s.queries.GetPatientById(ctx, patientID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, notFound("patient_not_found", "patient %d does not exist", patientID)
		}
		return nil, fmt.Errorf("load patient: %w", err)
	}

	rows, err := s.queries.GetUpcomingBookingsByPatient(ctx, database.GetUpcomingBookingsByPatientParams{
		PatientID: patientID,
		Date:      s.nowIn().Format(DateFormat + " " + TimeFormat),
	})
	if err != nil {
		return nil, fmt.Errorf("load appointments: %w", err)
	}

	appointments := make([]Appointment, 0, len(rows))
	for _, row := range rows {
		appointments = append(appointments, Appointment{
			ID:         row.ID,
			Status:     row.Status,
			SlotID:     row.SlotID,
			PatientID:  row.PatientID,
			DoctorID:   row.DoctorID,
			DoctorName: row.DoctorName,
			Date:       row.Date,
			StartTime:  row.StartTime,
			EndTime:    row.EndTime,
		})
	}
	return appointments, nil
}

// claimableSlot returns the slot of a doctor for a given start time, making
// sure it exists and is still free.
func (s *Service) claimableSlot(ctx context.Context, q *database.Queries, doctorID int64, date, startTime string) (database.Slot, error) {
	slots, err := s.ensureSlots(ctx, q, doctorID, date)
	if err != nil {
		return database.Slot{}, err
	}
	slot, ok := findSlot(slots, startTime)
	if !ok {
		return database.Slot{}, validation("slot_not_available",
			"%s is not a slot of doctor %d on %s", startTime, doctorID, date)
	}
	if slot.IsAvailable == 0 {
		return database.Slot{}, conflict("slot_already_booked",
			"slot %s on %s is already booked", startTime, date)
	}
	return slot, nil
}

// checkBookable rejects slots that have already started and slots that are
// too close to now to be booked.
func (s *Service) checkBookable(date, startTime string) error {
	start, err := s.instant(date, startTime)
	if err != nil {
		return err
	}
	now := s.nowIn()
	if start.Before(now) {
		return validation("slot_in_past", "slot %s on %s has already started", startTime, date)
	}
	if lead := BookingLeadTime; lead > 0 && start.Before(now.Add(lead)) {
		return validation("booking_too_soon",
			"appointments have to be booked at least %d minutes in advance, %s on %s is too close",
			int(lead.Minutes()), startTime, date)
	}
	return nil
}

// bookable keeps the slots that can still be booked right now.
func (s *Service) bookable(slots []database.Slot) []database.Slot {
	cutoff := s.nowIn().Add(BookingLeadTime)
	bookable := make([]database.Slot, 0, len(slots))
	for _, slot := range slots {
		start, err := s.instant(slot.Date, slot.StartTime)
		if err != nil {
			continue
		}
		if start.Before(cutoff) {
			continue
		}
		bookable = append(bookable, slot)
	}
	return bookable
}

// findSlot looks up a slot by its start time.
func findSlot(slots []database.Slot, startTime string) (database.Slot, bool) {
	for _, slot := range slots {
		if slot.StartTime == startTime {
			return slot, true
		}
	}
	return database.Slot{}, false
}

// missingResource turns a failed lookup into a "not found" error when the
// row simply does not exist.
func missingResource(kind string, id int64, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return notFound(kind+"_not_found", "%s %d does not exist", kind, id)
	}
	return fmt.Errorf("load %s: %w", kind, err)
}

// isUniqueViolation reports whether the error is a unique constraint
// violation raised by SQLite.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique ||
			sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey
	}
	return false
}
