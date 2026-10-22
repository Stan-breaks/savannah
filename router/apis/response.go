package apis

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"savannah/service"
)

// errorResponse is the body of every failed request.
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// WriteError answers a request with an error body.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: message, Code: code})
}

// MethodNotAllowed answers a request that used the wrong HTTP method.
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed",
		fmt.Sprintf("%s is not allowed on %s", r.Method, r.URL.Path))
}

// writeJSON answers a request with a JSON body.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write response: %v", err)
	}
}

// writeServiceError maps a failure of the service layer onto the matching
// HTTP status code.
func writeServiceError(w http.ResponseWriter, err error) {
	var svcErr *service.Error
	if !errors.As(err, &svcErr) {
		log.Printf("internal error: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "the request could not be processed")
		return
	}
	WriteError(w, statusFor(svcErr.Kind), svcErr.Code, svcErr.Message)
}

// statusFor maps the kind of a service failure onto an HTTP status code.
func statusFor(kind error) int {
	switch {
	case errors.Is(kind, service.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(kind, service.ErrConflict):
		return http.StatusConflict
	case errors.Is(kind, service.ErrValidation):
		return http.StatusUnprocessableEntity
	case errors.Is(kind, service.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// decodeJSON reads the request body into the given value.
func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("the request body must not be empty")
		}
		return fmt.Errorf("the request body must be a JSON object: %w", err)
	}
	return nil
}
