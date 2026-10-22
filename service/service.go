// Package service implements the clinic booking rules that sit between the
// HTTP layer and the generated sqlc queries.
package service

import (
	"database/sql"
	"savannah/database"
	"time"
)

// Booking statuses stored in the bookings table.
const (
	BookingStatusConfirmed = "CONFIRMED"
	BookingStatusCancelled = "CANCELLED"
)

// Service coordinates the business rules of the booking system.
type Service struct {
	db       *sql.DB
	queries  *database.Queries
	now      func() time.Time
	location *time.Location
}

// Option customises a Service at construction time.
type Option func(*Service)

// WithClock replaces the clock used by the time sensitive rules. Tests use
// it to pin "now" instead of depending on the wall clock.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		s.now = now
	}
}

// WithLocation sets the timezone that slot dates and clock times are read
// in. It defaults to the server's local timezone.
func WithLocation(loc *time.Location) Option {
	return func(s *Service) {
		s.location = loc
	}
}

// New builds a Service on top of the generated sqlc queries.
func New(db *sql.DB, queries *database.Queries, opts ...Option) *Service {
	s := &Service{
		db:       db,
		queries:  queries,
		now:      time.Now,
		location: time.Local,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// instant turns a slot date and clock time into a point in time, using the
// timezone the service reads slot dates in.
func (s *Service) instant(date, clock string) (time.Time, error) {
	day, err := ParseDate(date)
	if err != nil {
		return time.Time{}, err
	}
	t, err := ParseTimeOfDay(clock)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, s.location), nil
}

// nowIn returns the current time in the service's timezone.
func (s *Service) nowIn() time.Time {
	return s.now().In(s.location)
}
