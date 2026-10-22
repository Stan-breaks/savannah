package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"savannah/database"
	"testing"
	"time"
)

// testClock is the moment the service believes it is. The test date is a
// Thursday, so the weekday of the seeded working hours is derived from it.
var testClock = time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)

// newTestService opens a throwaway SQLite database, applies the schema and
// returns a service that reads time from the given clock.
func newTestService(t *testing.T, now time.Time) *Service {
	t.Helper()

	path := filepath.Join(t.TempDir(), "savannah-test.db")
	dsn := "file:" + path + "?_foreign_keys=on&_busy_timeout=5000&_txlock=immediate"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema, err := os.ReadFile(filepath.Join("..", "sqlc", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	clock := now
	return New(db, database.New(db),
		WithClock(func() time.Time { return clock }),
		WithLocation(time.UTC),
	)
}

// seedDoctor creates a doctor.
func seedDoctor(t *testing.T, svc *Service, name string) database.Doctor {
	t.Helper()
	doctor, err := svc.queries.CreateDoctor(context.Background(), name)
	if err != nil {
		t.Fatalf("create doctor: %v", err)
	}
	return doctor
}

// seedPatient creates a patient.
func seedPatient(t *testing.T, svc *Service, name string) database.Patient {
	t.Helper()
	patient, err := svc.queries.CreatePatient(context.Background(), name)
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}
	return patient
}

// seedWorkingHours gives a doctor a shift on the weekday of the test date.
func seedWorkingHours(t *testing.T, svc *Service, doctor database.Doctor, start, end, lunchStart, lunchEnd string) {
	t.Helper()
	ctx := context.Background()

	wh, err := svc.queries.CreateWorkingHours(ctx, database.CreateWorkingHoursParams{
		DayOfWeek:  int64(weekdayOf(t, testDate)),
		StartTime:  start,
		EndTime:    end,
		LunchStart: sql.NullString{String: lunchStart, Valid: lunchStart != ""},
		LunchEnd:   sql.NullString{String: lunchEnd, Valid: lunchEnd != ""},
	})
	if err != nil {
		t.Fatalf("create working hours: %v", err)
	}
	if err := svc.queries.AssignWorkingHoursToDoctor(ctx, database.AssignWorkingHoursToDoctorParams{
		DoctorID:       doctor.ID,
		WorkingHoursID: wh.ID,
	}); err != nil {
		t.Fatalf("assign working hours: %v", err)
	}
}

// codeOf returns the service error code of an error, if it has one.
func codeOf(t *testing.T, err error) string {
	t.Helper()
	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("error %v is not a service error", err)
	}
	return svcErr.Code
}

// seedClinic creates a doctor working 09:00 to 17:00 with a lunch break and
// two patients.
func seedClinic(t *testing.T, svc *Service) (database.Doctor, database.Patient, database.Patient) {
	t.Helper()
	doctor := seedDoctor(t, svc, "Dr. House")
	seedWorkingHours(t, svc, doctor, "09:00", "17:00", "13:00", "14:00")
	return doctor, seedPatient(t, svc, "Alice"), seedPatient(t, svc, "Bob")
}

func TestBookCreatesAConfirmedAppointment(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if appointment.Status != BookingStatusConfirmed {
		t.Errorf("status = %s, want %s", appointment.Status, BookingStatusConfirmed)
	}
	if appointment.PatientID != alice.ID {
		t.Errorf("patient id = %d, want %d", appointment.PatientID, alice.ID)
	}
	if appointment.DoctorID != doctor.ID {
		t.Errorf("doctor id = %d, want %d", appointment.DoctorID, doctor.ID)
	}
	if appointment.StartTime != "11:00" || appointment.EndTime != "11:30" {
		t.Errorf("slot = %s-%s, want 11:00-11:30", appointment.StartTime, appointment.EndTime)
	}

	// the slot must not be offered again
	slots, err := svc.Availability(ctx, doctor.ID, testDate)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	for _, slot := range slots {
		if slot.StartTime == "11:00" {
			t.Fatal("the booked slot is still available")
		}
	}
}

func TestBookRejectsInvalidRequests(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	tests := []struct {
		name     string
		request  BookRequest
		wantCode string
	}{
		{
			name:     "outside the working hours",
			request:  BookRequest{PatientID: alice.ID, DoctorID: doctor.ID, Date: testDate, StartTime: "20:00"},
			wantCode: "outside_working_hours",
		},
		{
			name:     "inside the lunch break",
			request:  BookRequest{PatientID: alice.ID, DoctorID: doctor.ID, Date: testDate, StartTime: "13:00"},
			wantCode: "slot_in_lunch_break",
		},
		{
			name:     "not on the 30 minute grid",
			request:  BookRequest{PatientID: alice.ID, DoctorID: doctor.ID, Date: testDate, StartTime: "11:15"},
			wantCode: "slot_not_aligned",
		},
		{
			name:     "a day the doctor does not work",
			request:  BookRequest{PatientID: alice.ID, DoctorID: doctor.ID, Date: "2026-10-23", StartTime: "11:00"},
			wantCode: "doctor_not_working",
		},
		{
			name:     "a malformed date",
			request:  BookRequest{PatientID: alice.ID, DoctorID: doctor.ID, Date: "22-10-2026", StartTime: "11:00"},
			wantCode: "invalid_date",
		},
		{
			name:     "a malformed time",
			request:  BookRequest{PatientID: alice.ID, DoctorID: doctor.ID, Date: testDate, StartTime: "11h00"},
			wantCode: "invalid_time",
		},
		{
			name:     "an unknown doctor",
			request:  BookRequest{PatientID: alice.ID, DoctorID: 999, Date: testDate, StartTime: "11:00"},
			wantCode: "doctor_not_found",
		},
		{
			name:     "an unknown patient",
			request:  BookRequest{PatientID: 999, DoctorID: doctor.ID, Date: testDate, StartTime: "11:00"},
			wantCode: "patient_not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Book(ctx, tt.request)
			if err == nil {
				t.Fatal("Book succeeded, want an error")
			}
			if got := codeOf(t, err); got != tt.wantCode {
				t.Fatalf("code = %q, want %q (message: %v)", got, tt.wantCode, err)
			}
		})
	}
}

func TestBookRejectsASlotInThePast(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	svc := newTestService(t, now)
	doctor, alice, _ := seedClinic(t, svc)

	_, err := svc.Book(context.Background(), BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "09:00",
	})
	if err == nil {
		t.Fatal("Book succeeded, want an error")
	}
	if got := codeOf(t, err); got != "slot_in_past" {
		t.Fatalf("code = %q, want slot_in_past (message: %v)", got, err)
	}
}

func TestBookRejectsASlotThatIsAlreadyTaken(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, bob := seedClinic(t, svc)
	ctx := context.Background()

	request := BookRequest{PatientID: alice.ID, DoctorID: doctor.ID, Date: testDate, StartTime: "11:00"}
	if _, err := svc.Book(ctx, request); err != nil {
		t.Fatalf("Book: %v", err)
	}

	request.PatientID = bob.ID
	_, err := svc.Book(ctx, request)
	if err == nil {
		t.Fatal("Book succeeded, want an error")
	}
	if got := codeOf(t, err); got != "slot_already_booked" {
		t.Fatalf("code = %q, want slot_already_booked (message: %v)", got, err)
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want a conflict", err)
	}
}

func TestCancelReleasesTheSlotForBookingAgain(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, bob := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	cancelled, err := svc.Cancel(ctx, CancelRequest{BookingID: appointment.ID, Reason: "patient is ill"})
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != BookingStatusCancelled {
		t.Errorf("status = %s, want %s", cancelled.Status, BookingStatusCancelled)
	}
	if cancelled.CancelReason != "patient is ill" {
		t.Errorf("reason = %q, want %q", cancelled.CancelReason, "patient is ill")
	}

	// the slot has to be offered again
	slots, err := svc.Availability(ctx, doctor.ID, testDate)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	found := false
	for _, slot := range slots {
		if slot.StartTime == "11:00" {
			found = true
		}
	}
	if !found {
		t.Fatal("the cancelled slot is not available again")
	}

	// and it has to be bookable again
	if _, err := svc.Book(ctx, BookRequest{
		PatientID: bob.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	}); err != nil {
		t.Fatalf("Book after cancel: %v", err)
	}
}

func TestCancelRejectsAnAlreadyCancelledAppointment(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if _, err := svc.Cancel(ctx, CancelRequest{BookingID: appointment.ID, Reason: "changed plans"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	_, err = svc.Cancel(ctx, CancelRequest{BookingID: appointment.ID, Reason: "changed plans again"})
	if err == nil {
		t.Fatal("Cancel succeeded, want an error")
	}
	if got := codeOf(t, err); got != "appointment_already_cancelled" {
		t.Fatalf("code = %q, want appointment_already_cancelled (message: %v)", got, err)
	}
}

func TestCancelRequiresAReason(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	_, err = svc.Cancel(ctx, CancelRequest{BookingID: appointment.ID, Reason: "   "})
	if err == nil {
		t.Fatal("Cancel succeeded, want an error")
	}
	if got := codeOf(t, err); got != "missing_cancel_reason" {
		t.Fatalf("code = %q, want missing_cancel_reason (message: %v)", got, err)
	}
}

func TestCancelRejectsAnUnknownAppointment(t *testing.T) {
	svc := newTestService(t, testClock)

	_, err := svc.Cancel(context.Background(), CancelRequest{BookingID: 404, Reason: "changed plans"})
	if err == nil {
		t.Fatal("Cancel succeeded, want an error")
	}
	if got := codeOf(t, err); got != "appointment_not_found" {
		t.Fatalf("code = %q, want appointment_not_found (message: %v)", got, err)
	}
}

func TestRescheduleMovesTheAppointment(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	moved, err := svc.Reschedule(ctx, RescheduleRequest{
		BookingID: appointment.ID,
		Date:      testDate,
		StartTime: "15:00",
	})
	if err != nil {
		t.Fatalf("Reschedule: %v", err)
	}
	if moved.ID != appointment.ID {
		t.Errorf("appointment id = %d, want %d", moved.ID, appointment.ID)
	}
	if moved.StartTime != "15:00" {
		t.Errorf("start time = %s, want 15:00", moved.StartTime)
	}
	if moved.SlotID == appointment.SlotID {
		t.Error("the appointment kept its slot")
	}

	// the old slot is free again, the new one is taken
	slots, err := svc.Availability(ctx, doctor.ID, testDate)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	availability := map[string]bool{}
	for _, slot := range slots {
		availability[slot.StartTime] = true
	}
	if !availability["11:00"] {
		t.Error("the slot the appointment left is not available again")
	}
	if availability["15:00"] {
		t.Error("the slot the appointment moved into is still available")
	}
}

func TestRescheduleRejectsATakenSlot(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, bob := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if _, err := svc.Book(ctx, BookRequest{
		PatientID: bob.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "15:00",
	}); err != nil {
		t.Fatalf("Book: %v", err)
	}

	_, err = svc.Reschedule(ctx, RescheduleRequest{
		BookingID: appointment.ID,
		Date:      testDate,
		StartTime: "15:00",
	})
	if err == nil {
		t.Fatal("Reschedule succeeded, want an error")
	}
	if got := codeOf(t, err); got != "slot_already_booked" {
		t.Fatalf("code = %q, want slot_already_booked (message: %v)", got, err)
	}
}

func TestRescheduleRejectsACancelledAppointment(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if _, err := svc.Cancel(ctx, CancelRequest{BookingID: appointment.ID, Reason: "changed plans"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	_, err = svc.Reschedule(ctx, RescheduleRequest{
		BookingID: appointment.ID,
		Date:      testDate,
		StartTime: "15:00",
	})
	if err == nil {
		t.Fatal("Reschedule succeeded, want an error")
	}
	if got := codeOf(t, err); got != "appointment_already_cancelled" {
		t.Fatalf("code = %q, want appointment_already_cancelled (message: %v)", got, err)
	}
}

func TestRescheduleRejectsASlotOutsideWorkingHours(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	_, err = svc.Reschedule(ctx, RescheduleRequest{
		BookingID: appointment.ID,
		Date:      testDate,
		StartTime: "13:30",
	})
	if err == nil {
		t.Fatal("Reschedule succeeded, want an error")
	}
	if got := codeOf(t, err); got != "slot_in_lunch_break" {
		t.Fatalf("code = %q, want slot_in_lunch_break (message: %v)", got, err)
	}
}

func TestRescheduleRejectsTheSlotTheAppointmentAlreadyHas(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	_, err = svc.Reschedule(ctx, RescheduleRequest{
		BookingID: appointment.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err == nil {
		t.Fatal("Reschedule succeeded, want an error")
	}
	if got := codeOf(t, err); got != "same_slot" {
		t.Fatalf("code = %q, want same_slot (message: %v)", got, err)
	}
}

func TestRescheduleCanMoveAnAppointmentToAnotherDoctor(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	other := seedDoctor(t, svc, "Dr. Wilson")
	seedWorkingHours(t, svc, other, "09:00", "17:00", "", "")

	appointment, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	moved, err := svc.Reschedule(ctx, RescheduleRequest{
		BookingID: appointment.ID,
		DoctorID:  other.ID,
		Date:      testDate,
		StartTime: "15:00",
	})
	if err != nil {
		t.Fatalf("Reschedule: %v", err)
	}
	if moved.DoctorID != other.ID {
		t.Errorf("doctor id = %d, want %d", moved.DoctorID, other.ID)
	}
	if moved.ID != appointment.ID {
		t.Errorf("appointment id = %d, want %d", moved.ID, appointment.ID)
	}

	// the slot of the old doctor is free again
	slots, err := svc.Availability(ctx, doctor.ID, testDate)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	for _, slot := range slots {
		if slot.StartTime == "11:00" {
			return
		}
	}
	t.Fatal("the slot the appointment left is not available again")
}

func TestAvailabilityListsTheFreeSlots(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	if _, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	}); err != nil {
		t.Fatalf("Book: %v", err)
	}

	slots, err := svc.Availability(ctx, doctor.ID, testDate)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}

	var got []string
	for _, slot := range slots {
		got = append(got, slot.StartTime)
	}
	want := []string{
		"09:00", "09:30", "10:00", "10:30",
		"11:30", "12:00", "12:30",
		"14:00", "14:30", "15:00", "15:30", "16:00", "16:30",
	}
	if !equalStrings(got, want) {
		t.Fatalf("slots = %v, want %v", got, want)
	}
}

func TestAvailabilityRejectsAnUnknownDoctor(t *testing.T) {
	svc := newTestService(t, testClock)

	_, err := svc.Availability(context.Background(), 404, testDate)
	if err == nil {
		t.Fatal("Availability succeeded, want an error")
	}
	if got := codeOf(t, err); got != "doctor_not_found" {
		t.Fatalf("code = %q, want doctor_not_found (message: %v)", got, err)
	}
}

func TestPatientAppointmentsAreUpcomingAndSortedByDate(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	for _, start := range []string{"15:00", "11:00"} {
		if _, err := svc.Book(ctx, BookRequest{
			PatientID: alice.ID,
			DoctorID:  doctor.ID,
			Date:      testDate,
			StartTime: start,
		}); err != nil {
			t.Fatalf("Book %s: %v", start, err)
		}
	}

	appointments, err := svc.PatientAppointments(ctx, alice.ID)
	if err != nil {
		t.Fatalf("PatientAppointments: %v", err)
	}
	if len(appointments) != 2 {
		t.Fatalf("got %d appointments, want 2", len(appointments))
	}
	if appointments[0].StartTime != "11:00" || appointments[1].StartTime != "15:00" {
		t.Fatalf("appointments = %s, %s, want 11:00 then 15:00",
			appointments[0].StartTime, appointments[1].StartTime)
	}
	if appointments[0].DoctorName != "Dr. House" {
		t.Errorf("doctor name = %q, want Dr. House", appointments[0].DoctorName)
	}
}

func TestPatientAppointmentsSkipCancelledAndPastAppointments(t *testing.T) {
	svc := newTestService(t, testClock)
	doctor, alice, _ := seedClinic(t, svc)
	ctx := context.Background()

	kept, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "15:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	cancelled, err := svc.Book(ctx, BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if _, err := svc.Cancel(ctx, CancelRequest{BookingID: cancelled.ID, Reason: "changed plans"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	appointments, err := svc.PatientAppointments(ctx, alice.ID)
	if err != nil {
		t.Fatalf("PatientAppointments: %v", err)
	}
	if len(appointments) != 1 {
		t.Fatalf("got %d appointments, want 1", len(appointments))
	}
	if appointments[0].ID != kept.ID {
		t.Fatalf("appointment id = %d, want %d", appointments[0].ID, kept.ID)
	}
}

func TestPatientAppointmentsRejectAnUnknownPatient(t *testing.T) {
	svc := newTestService(t, testClock)

	_, err := svc.PatientAppointments(context.Background(), 404)
	if err == nil {
		t.Fatal("PatientAppointments succeeded, want an error")
	}
	if got := codeOf(t, err); got != "patient_not_found" {
		t.Fatalf("code = %q, want patient_not_found (message: %v)", got, err)
	}
}
