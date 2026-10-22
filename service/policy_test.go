package service

import (
	"context"
	"testing"
	"time"
)

// leadClock is a moment that sits inside the lead time of the 11:00 slot of
// the seeded shift.
var leadClock = time.Date(2026, 10, 22, 10, 30, 0, 0, time.UTC)

func TestBookRejectsASlotInsideTheLeadTime(t *testing.T) {
	svc := newTestService(t, leadClock)
	doctor, alice, _ := seedClinic(t, svc)

	_, err := svc.Book(context.Background(), BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:00",
	})
	if err == nil {
		t.Fatal("Book succeeded, want an error")
	}
	if got := codeOf(t, err); got != "booking_too_soon" {
		t.Fatalf("code = %q, want booking_too_soon (message: %v)", got, err)
	}
}

func TestBookAcceptsASlotThatIsExactlyTheLeadTimeAway(t *testing.T) {
	svc := newTestService(t, leadClock)
	doctor, alice, _ := seedClinic(t, svc)

	if _, err := svc.Book(context.Background(), BookRequest{
		PatientID: alice.ID,
		DoctorID:  doctor.ID,
		Date:      testDate,
		StartTime: "11:30",
	}); err != nil {
		t.Fatalf("Book: %v", err)
	}
}

func TestAvailabilityOmitsSlotsInsideTheLeadTime(t *testing.T) {
	svc := newTestService(t, leadClock)
	doctor, _, _ := seedClinic(t, svc)

	slots, err := svc.Availability(context.Background(), doctor.ID, testDate)
	if err != nil {
		t.Fatalf("Availability: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("got no slots, want the slots outside the lead time")
	}
	if slots[0].StartTime != "11:30" {
		t.Fatalf("first slot = %s, want 11:30", slots[0].StartTime)
	}
}
