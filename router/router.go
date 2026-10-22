package router

import (
	"net/http"
	"savannah/router/apis"
	"savannah/service"
)

type Router struct {
	svc *service.Service
	mux *http.ServeMux
}

func NewRouter(svc *service.Service) *Router {
	r := &Router{
		svc: svc,
		mux: http.NewServeMux(),
	}
	r.setupRoutes()
	return r
}

func (r *Router) setupRoutes() {
	appointments := apis.NewAppointmentApi(r.svc)
	doctors := apis.NewDoctorApi(r.svc)
	patients := apis.NewPatientApi(r.svc)

	// Appointments
	r.mux.HandleFunc("POST /appointments", appointments.Book)
	r.mux.HandleFunc("PATCH /appointments/{id}/cancel", appointments.Cancel)
	r.mux.HandleFunc("PATCH /appointments/{id}/reschedule", appointments.Reschedule)

	// Doctors
	r.mux.HandleFunc("GET /doctors/{id}/availability", doctors.Availability)

	// Patients
	r.mux.HandleFunc("GET /patients/{id}/appointments", patients.Appointments)

	// The same paths with a different method are answered with the same JSON
	// error format as the rest of the API.
	r.mux.HandleFunc("/appointments", apis.MethodNotAllowed)
	r.mux.HandleFunc("/appointments/{id}/cancel", apis.MethodNotAllowed)
	r.mux.HandleFunc("/appointments/{id}/reschedule", apis.MethodNotAllowed)
	r.mux.HandleFunc("/doctors/{id}/availability", apis.MethodNotAllowed)
	r.mux.HandleFunc("/patients/{id}/appointments", apis.MethodNotAllowed)
}

func (r *Router) Handler() http.Handler {
	return r.mux
}
