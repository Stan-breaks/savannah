package apis

import (
	"net/http"
	"savannah/service"
)

// DoctorApi serves the doctor endpoints.
type DoctorApi struct {
	svc *service.Service
}

// NewDoctorApi builds the doctor handlers.
func NewDoctorApi(svc *service.Service) *DoctorApi {
	return &DoctorApi{svc: svc}
}

// slotResponse is the JSON view of a bookable slot.
type slotResponse struct {
	ID        int64  `json:"id"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Date      string `json:"date"`
	DoctorID  int64  `json:"doctor_id"`
}

// availabilityResponse is the body of GET /doctors/{id}/availability.
type availabilityResponse struct {
	DoctorID int64          `json:"doctor_id"`
	Date     string         `json:"date"`
	Slots    []slotResponse `json:"slots"`
}

// Availability handles GET /doctors/{id}/availability.
func (h *DoctorApi) Availability(w http.ResponseWriter, r *http.Request) {
	doctorID, err := pathID(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_doctor_id", err.Error())
		return
	}

	date := r.URL.Query().Get("date")
	if date == "" {
		WriteError(w, http.StatusBadRequest, "missing_date",
			"the date query parameter is required and has to be formatted as YYYY-MM-DD")
		return
	}

	slots, err := h.svc.Availability(r.Context(), doctorID, date)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	body := availabilityResponse{
		DoctorID: doctorID,
		Date:     date,
		Slots:    make([]slotResponse, 0, len(slots)),
	}
	for _, slot := range slots {
		body.Slots = append(body.Slots, slotResponse{
			ID:        slot.ID,
			StartTime: slot.StartTime,
			EndTime:   slot.EndTime,
			Date:      slot.Date,
			DoctorID:  slot.DoctorID,
		})
	}
	writeJSON(w, http.StatusOK, body)
}
