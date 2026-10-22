package apis

import (
	"net/http"
	"savannah/service"
)

// PatientApi serves the patient endpoints.
type PatientApi struct {
	svc *service.Service
}

// NewPatientApi builds the patient handlers.
func NewPatientApi(svc *service.Service) *PatientApi {
	return &PatientApi{svc: svc}
}

// patientAppointmentsResponse is the body of GET /patients/{id}/appointments.
type patientAppointmentsResponse struct {
	PatientID    int64                 `json:"patient_id"`
	Appointments []service.Appointment `json:"appointments"`
}

// Appointments handles GET /patients/{id}/appointments.
func (h *PatientApi) Appointments(w http.ResponseWriter, r *http.Request) {
	patientID, err := pathID(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_patient_id", err.Error())
		return
	}

	appointments, err := h.svc.PatientAppointments(r.Context(), patientID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, patientAppointmentsResponse{
		PatientID:    patientID,
		Appointments: appointments,
	})
}
