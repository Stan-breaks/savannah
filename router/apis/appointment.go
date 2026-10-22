package apis

import (
	"net/http"
	"savannah/service"
)

// AppointmentApi serves the appointment endpoints.
type AppointmentApi struct {
	svc *service.Service
}

// NewAppointmentApi builds the appointment handlers.
func NewAppointmentApi(svc *service.Service) *AppointmentApi {
	return &AppointmentApi{svc: svc}
}

// bookRequest is the body of POST /appointments.
type bookRequest struct {
	PatientID int64  `json:"patient_id"`
	DoctorID  int64  `json:"doctor_id"`
	Date      string `json:"date"`
	StartTime string `json:"start_time"`
}

// cancelRequest is the body of PATCH /appointments/{id}/cancel.
type cancelRequest struct {
	Reason string `json:"reason"`
}

// rescheduleRequest is the body of PATCH /appointments/{id}/reschedule.
// DoctorID is optional and defaults to the doctor of the appointment.
type rescheduleRequest struct {
	DoctorID  int64  `json:"doctor_id"`
	Date      string `json:"date"`
	StartTime string `json:"start_time"`
}

// Book handles POST /appointments.
func (h *AppointmentApi) Book(w http.ResponseWriter, r *http.Request) {
	var body bookRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	appointment, err := h.svc.Book(r.Context(), service.BookRequest{
		PatientID: body.PatientID,
		DoctorID:  body.DoctorID,
		Date:      body.Date,
		StartTime: body.StartTime,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, appointment)
}

// Cancel handles PATCH /appointments/{id}/cancel.
func (h *AppointmentApi) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_appointment_id", err.Error())
		return
	}

	var body cancelRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	appointment, err := h.svc.Cancel(r.Context(), service.CancelRequest{
		BookingID: id,
		Reason:    body.Reason,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, appointment)
}

// Reschedule handles PATCH /appointments/{id}/reschedule.
func (h *AppointmentApi) Reschedule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_appointment_id", err.Error())
		return
	}

	var body rescheduleRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	appointment, err := h.svc.Reschedule(r.Context(), service.RescheduleRequest{
		BookingID: id,
		DoctorID:  body.DoctorID,
		Date:      body.Date,
		StartTime: body.StartTime,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, appointment)
}
