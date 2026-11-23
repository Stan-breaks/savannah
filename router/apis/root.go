package apis

import (
	"fmt"
	"net/http"
)

// LiveURL is the public address of the deployed API.
const LiveURL = "https://savannah.stanbreaks.com"

// apiInfo is the response of GET /, a small entry point for anyone landing on
// the API root.
type apiInfo struct {
	Name      string   `json:"name"`
	LiveURL   string   `json:"live_url"`
	Docs      string   `json:"docs"`
	Endpoints []string `json:"endpoints"`
}

// Root answers GET / with pointers to the live API and its documentation.
func Root(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, apiInfo{
		Name:    "Savannah clinic booking API",
		LiveURL: LiveURL,
		Docs:    "https://github.com/Stan-breaks/savannah#readme",
		Endpoints: []string{
			"POST /appointments",
			"GET /doctors/{id}/availability?date=YYYY-MM-DD",
			"PATCH /appointments/{id}/cancel",
			"PATCH /appointments/{id}/reschedule",
			"GET /patients/{id}/appointments",
		},
	})
}

// NotFound answers every unmatched route with the same JSON error envelope
// as the rest of the API.
func NotFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusNotFound, "not_found",
		fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
}
