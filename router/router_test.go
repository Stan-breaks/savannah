package router

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"savannah/database"
	"savannah/service"
	"testing"
	"time"
)

// testClock is the moment the service believes it is. The test date is a
// Thursday.
var testClock = time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)

// testDate is the date the seeded doctor works on.
const testDate = "2026-10-22"

// newTestRouter returns a router backed by a throwaway database that holds a
// doctor working 09:00 to 17:00 with a lunch break and two patients.
func newTestRouter(t *testing.T) (*Router, database.Doctor, database.Patient, database.Patient) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "savannah-test.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on&_busy_timeout=5000&_txlock=immediate")
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

	queries := database.New(db)
	clock := testClock
	svc := service.New(db, queries,
		service.WithClock(func() time.Time { return clock }),
		service.WithLocation(time.UTC),
	)

	doctor, err := queries.CreateDoctor(context.Background(), "Dr. House")
	if err != nil {
		t.Fatalf("create doctor: %v", err)
	}
	weekday := int(time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC).Weekday())
	workingHours, err := queries.CreateWorkingHours(context.Background(), database.CreateWorkingHoursParams{
		DayOfWeek:  int64(weekday),
		StartTime:  "09:00",
		EndTime:    "17:00",
		LunchStart: sql.NullString{String: "13:00", Valid: true},
		LunchEnd:   sql.NullString{String: "14:00", Valid: true},
	})
	if err != nil {
		t.Fatalf("create working hours: %v", err)
	}
	if err := queries.AssignWorkingHoursToDoctor(context.Background(), database.AssignWorkingHoursToDoctorParams{
		DoctorID:       doctor.ID,
		WorkingHoursID: workingHours.ID,
	}); err != nil {
		t.Fatalf("assign working hours: %v", err)
	}

	alice, err := queries.CreatePatient(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}
	bob, err := queries.CreatePatient(context.Background(), "Bob")
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}

	return NewRouter(svc), doctor, alice, bob
}

// call runs a request against the router and returns the recorded response.
func call(t *testing.T, r *Router, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	request := httptest.NewRequest(method, target, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	r.Handler().ServeHTTP(recorder, request)
	return recorder
}

// decode reads the JSON body of a response.
func decode(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return body
}

// book books the 11:00 slot of the given patient and returns the recorded
// response.
func book(t *testing.T, r *Router, patientID, doctorID int64, startTime string) *httptest.ResponseRecorder {
	t.Helper()
	return call(t, r, http.MethodPost, "/appointments", map[string]any{
		"patient_id": patientID,
		"doctor_id":  doctorID,
		"date":       testDate,
		"start_time": startTime,
	})
}

func TestBookEndpointCreatesAnAppointment(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	recorder := book(t, r, alice.ID, doctor.ID, "11:00")
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, http.StatusCreated, recorder.Body)
	}

	body := decode(t, recorder)
	if body["status"] != "CONFIRMED" {
		t.Errorf("status = %v, want CONFIRMED", body["status"])
	}
	if body["start_time"] != "11:00" {
		t.Errorf("start time = %v, want 11:00", body["start_time"])
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("content type = %q, want application/json", contentType)
	}
}

func TestBookEndpointReportsFailures(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	if recorder := book(t, r, alice.ID, doctor.ID, "11:00"); recorder.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", recorder.Code, recorder.Body)
	}

	tests := []struct {
		name     string
		body     any
		wantCode int
	}{
		{
			name:     "a malformed body",
			body:     map[string]any{"patient_id": "not a number"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "a slot outside the working hours",
			body:     map[string]any{"patient_id": alice.ID, "doctor_id": doctor.ID, "date": testDate, "start_time": "20:00"},
			wantCode: http.StatusUnprocessableEntity,
		},
		{
			name:     "a slot inside the lunch break",
			body:     map[string]any{"patient_id": alice.ID, "doctor_id": doctor.ID, "date": testDate, "start_time": "13:00"},
			wantCode: http.StatusUnprocessableEntity,
		},
		{
			name:     "an unknown doctor",
			body:     map[string]any{"patient_id": alice.ID, "doctor_id": 999, "date": testDate, "start_time": "15:00"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "an unknown patient",
			body:     map[string]any{"patient_id": 999, "doctor_id": doctor.ID, "date": testDate, "start_time": "15:00"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "a slot that is already taken",
			body:     map[string]any{"patient_id": alice.ID, "doctor_id": doctor.ID, "date": testDate, "start_time": "11:00"},
			wantCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := call(t, r, http.MethodPost, "/appointments", tt.body)
			if recorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, tt.wantCode, recorder.Body)
			}
			if body := decode(t, recorder); body["error"] == nil || body["error"] == "" {
				t.Errorf("body = %v, want an error message", body)
			}
		})
	}
}

func TestAvailabilityEndpointListsTheFreeSlots(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	if recorder := book(t, r, alice.ID, doctor.ID, "11:00"); recorder.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", recorder.Code, recorder.Body)
	}

	recorder := call(t, r, http.MethodGet, "/doctors/1/availability?date="+testDate, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, http.StatusOK, recorder.Body)
	}

	body := decode(t, recorder)
	slots, ok := body["slots"].([]any)
	if !ok {
		t.Fatalf("body = %v, want a list of slots", body)
	}
	for _, raw := range slots {
		slot := raw.(map[string]any)
		if slot["start_time"] == "11:00" {
			t.Fatal("the booked slot is still offered")
		}
	}
	if len(slots) != 13 {
		t.Errorf("got %d slots, want 13", len(slots))
	}
}

func TestAvailabilityEndpointReportsFailures(t *testing.T) {
	r, doctor, _, _ := newTestRouter(t)

	tests := []struct {
		name     string
		target   string
		wantCode int
	}{
		{
			name:     "a missing date",
			target:   "/doctors/1/availability",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "a malformed doctor id",
			target:   "/doctors/abc/availability?date=" + testDate,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "an unknown doctor",
			target:   "/doctors/999/availability?date=" + testDate,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "a malformed date",
			target:   "/doctors/1/availability?date=22-10-2026",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := call(t, r, http.MethodGet, tt.target, nil)
			if recorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, tt.wantCode, recorder.Body)
			}
		})
	}
	_ = doctor
}

func TestCancelEndpointCancelsTheAppointment(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	booked := book(t, r, alice.ID, doctor.ID, "11:00")
	if booked.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", booked.Code, booked.Body)
	}
	id := int64(decode(t, booked)["id"].(float64))

	recorder := call(t, r, http.MethodPatch, "/appointments/1/cancel", map[string]any{"reason": "patient is ill"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, http.StatusOK, recorder.Body)
	}
	body := decode(t, recorder)
	if body["status"] != "CANCELLED" {
		t.Errorf("status = %v, want CANCELLED", body["status"])
	}
	if body["cancel_reason"] != "patient is ill" {
		t.Errorf("reason = %v, want patient is ill", body["cancel_reason"])
	}
	if body["id"].(float64) != float64(id) {
		t.Errorf("id = %v, want %d", body["id"], id)
	}
}

func TestCancelEndpointReportsFailures(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	booked := book(t, r, alice.ID, doctor.ID, "11:00")
	if booked.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", booked.Code, booked.Body)
	}

	tests := []struct {
		name     string
		target   string
		body     any
		wantCode int
	}{
		{
			name:     "a missing reason",
			target:   "/appointments/1/cancel",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "an unknown appointment",
			target:   "/appointments/404/cancel",
			body:     map[string]any{"reason": "changed plans"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "a malformed id",
			target:   "/appointments/abc/cancel",
			body:     map[string]any{"reason": "changed plans"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := call(t, r, http.MethodPatch, tt.target, tt.body)
			if recorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, tt.wantCode, recorder.Body)
			}
		})
	}
}

func TestCancelEndpointRefusesToCancelTwice(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	if recorder := book(t, r, alice.ID, doctor.ID, "11:00"); recorder.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", recorder.Code, recorder.Body)
	}
	if recorder := call(t, r, http.MethodPatch, "/appointments/1/cancel", map[string]any{"reason": "changed plans"}); recorder.Code != http.StatusOK {
		t.Fatalf("cancel failed: %d %s", recorder.Code, recorder.Body)
	}

	recorder := call(t, r, http.MethodPatch, "/appointments/1/cancel", map[string]any{"reason": "changed plans again"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, http.StatusConflict, recorder.Body)
	}
}

func TestRescheduleEndpointMovesTheAppointment(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	if recorder := book(t, r, alice.ID, doctor.ID, "11:00"); recorder.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", recorder.Code, recorder.Body)
	}

	recorder := call(t, r, http.MethodPatch, "/appointments/1/reschedule", map[string]any{
		"date":       testDate,
		"start_time": "15:00",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, http.StatusOK, recorder.Body)
	}
	body := decode(t, recorder)
	if body["start_time"] != "15:00" {
		t.Errorf("start time = %v, want 15:00", body["start_time"])
	}
	if body["status"] != "CONFIRMED" {
		t.Errorf("status = %v, want CONFIRMED", body["status"])
	}
}

func TestRescheduleEndpointReportsFailures(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	if recorder := book(t, r, alice.ID, doctor.ID, "11:00"); recorder.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", recorder.Code, recorder.Body)
	}

	tests := []struct {
		name     string
		target   string
		body     any
		wantCode int
	}{
		{
			name:     "a slot outside the working hours",
			target:   "/appointments/1/reschedule",
			body:     map[string]any{"date": testDate, "start_time": "20:00"},
			wantCode: http.StatusUnprocessableEntity,
		},
		{
			name:     "a slot inside the lunch break",
			target:   "/appointments/1/reschedule",
			body:     map[string]any{"date": testDate, "start_time": "13:00"},
			wantCode: http.StatusUnprocessableEntity,
		},
		{
			name:     "an unknown appointment",
			target:   "/appointments/404/reschedule",
			body:     map[string]any{"date": testDate, "start_time": "15:00"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := call(t, r, http.MethodPatch, tt.target, tt.body)
			if recorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, tt.wantCode, recorder.Body)
			}
		})
	}
}

func TestPatientAppointmentsEndpointListsTheUpcomingAppointments(t *testing.T) {
	r, doctor, alice, _ := newTestRouter(t)

	if recorder := book(t, r, alice.ID, doctor.ID, "15:00"); recorder.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", recorder.Code, recorder.Body)
	}
	if recorder := book(t, r, alice.ID, doctor.ID, "11:00"); recorder.Code != http.StatusCreated {
		t.Fatalf("setup booking failed: %d %s", recorder.Code, recorder.Body)
	}

	recorder := call(t, r, http.MethodGet, "/patients/1/appointments", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, http.StatusOK, recorder.Body)
	}

	body := decode(t, recorder)
	appointments, ok := body["appointments"].([]any)
	if !ok {
		t.Fatalf("body = %v, want a list of appointments", body)
	}
	if len(appointments) != 2 {
		t.Fatalf("got %d appointments, want 2", len(appointments))
	}
	first := appointments[0].(map[string]any)
	if first["start_time"] != "11:00" {
		t.Errorf("first appointment = %v, want the 11:00 one", first["start_time"])
	}
}

func TestPatientAppointmentsEndpointReportsFailures(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	tests := []struct {
		name     string
		target   string
		wantCode int
	}{
		{
			name:     "an unknown patient",
			target:   "/patients/999/appointments",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "a malformed id",
			target:   "/patients/abc/appointments",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := call(t, r, http.MethodGet, tt.target, nil)
			if recorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, tt.wantCode, recorder.Body)
			}
		})
	}
}

func TestUnknownRoutesAndMethodsAnswerWithJSON(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	t.Run("an unknown route", func(t *testing.T) {
		recorder := call(t, r, http.MethodGet, "/nope", nil)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
	})

	t.Run("a wrong method", func(t *testing.T) {
		recorder := call(t, r, http.MethodGet, "/appointments", nil)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
		}
		if body := decode(t, recorder); body["error"] == nil {
			t.Errorf("body = %v, want an error message", body)
		}
	})
}
