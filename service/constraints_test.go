package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"savannah/database"
	"testing"
)

// TestIsUniqueViolationDetectsThePartialUniqueIndex proves that the error
// SQLite raises when two active bookings claim the same slot is recognised,
// so a race between two reschedules ends in a conflict and not in a 500.
func TestIsUniqueViolationDetectsThePartialUniqueIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "savannah-test.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	schema, err := os.ReadFile(filepath.Join("..", "sqlc", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	queries := database.New(db)
	ctx := context.Background()

	doctor, err := queries.CreateDoctor(ctx, "Dr. House")
	if err != nil {
		t.Fatalf("create doctor: %v", err)
	}
	patient, err := queries.CreatePatient(ctx, "Alice")
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}
	slot, err := queries.CreateSlot(ctx, database.CreateSlotParams{
		StartTime:   "09:00",
		EndTime:     "09:30",
		Date:        testDate,
		IsAvailable: 1,
		DoctorID:    doctor.ID,
	})
	if err != nil {
		t.Fatalf("create slot: %v", err)
	}

	first, err := queries.CreateBooking(ctx, database.CreateBookingParams{PatientID: patient.ID, ID: slot.ID})
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	// a second active booking on the same slot has to be rejected
	_, err = queries.CreateBooking(ctx, database.CreateBookingParams{PatientID: patient.ID, ID: slot.ID})
	if err == nil {
		t.Fatal("the second booking was accepted, want it to be rejected")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("error = %v, want no rows", err)
	}

	// moving another appointment onto a slot that is already taken has to be
	// rejected by the partial unique index
	otherSlot, err := queries.CreateSlot(ctx, database.CreateSlotParams{
		StartTime:   "09:30",
		EndTime:     "10:00",
		Date:        testDate,
		IsAvailable: 1,
		DoctorID:    doctor.ID,
	})
	if err != nil {
		t.Fatalf("create slot: %v", err)
	}
	other, err := queries.CreateBooking(ctx, database.CreateBookingParams{PatientID: patient.ID, ID: otherSlot.ID})
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	_, err = queries.RescheduleBooking(ctx, database.RescheduleBookingParams{SlotID: slot.ID, ID: other.ID})
	if err == nil {
		t.Fatal("the appointment was moved onto a taken slot, want a unique violation")
	}
	if !isUniqueViolation(err) {
		t.Fatalf("error %v is not recognised as a unique violation", err)
	}

	// but a cancelled booking does not block the slot
	if _, err := queries.CancelBooking(ctx, database.CancelBookingParams{ID: first.ID}); err != nil {
		t.Fatalf("cancel booking: %v", err)
	}
	if _, err := queries.RescheduleBooking(ctx, database.RescheduleBookingParams{SlotID: slot.ID, ID: other.ID}); err != nil {
		t.Fatalf("moving onto a cancelled slot: %v", err)
	}
}

// TestIsUniqueViolationIgnoresOtherErrors makes sure unrelated failures are
// not mistaken for a conflict.
func TestIsUniqueViolationIgnoresOtherErrors(t *testing.T) {
	if isUniqueViolation(nil) {
		t.Error("nil was reported as a unique violation")
	}
	if isUniqueViolation(errors.New("connection reset")) {
		t.Error("a plain error was reported as a unique violation")
	}
}
