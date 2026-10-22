package service

import (
	"errors"
	"fmt"
)

// The kinds of failure the service layer reports. The transport layer maps
// them onto HTTP status codes.
var (
	// ErrNotFound means the referenced resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict means the request collides with the current state of a
	// resource, e.g. booking a slot that is already taken.
	ErrConflict = errors.New("conflict")
	// ErrValidation means the request is well formed but breaks a business
	// rule, e.g. booking outside the doctor's working hours.
	ErrValidation = errors.New("validation failed")
	// ErrInvalid means the request itself is malformed.
	ErrInvalid = errors.New("invalid request")
)

// Error is the error type the service layer returns. It carries a stable
// machine readable code, a message meant for the caller and the kind of
// failure so the transport layer can pick a status code.
type Error struct {
	// Code identifies the failure, e.g. "outside_working_hours".
	Code string
	// Message explains the failure in a way a caller can act on.
	Message string
	// Kind is one of the Err constants above.
	Kind error
}

func (e *Error) Error() string {
	return e.Message
}

// Unwrap exposes the failure kind so that errors.Is keeps working.
func (e *Error) Unwrap() error {
	return e.Kind
}

// invalid reports a malformed request.
func invalid(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Kind: ErrInvalid}
}

// validation reports a request that breaks a business rule.
func validation(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Kind: ErrValidation}
}

// conflict reports a request that collides with the current state of a
// resource.
func conflict(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Kind: ErrConflict}
}

// notFound reports a missing resource.
func notFound(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Kind: ErrNotFound}
}
