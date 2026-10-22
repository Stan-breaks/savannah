package service

import "time"

// BookingLeadTime is how far in advance an appointment has to be booked. A
// slot that starts sooner than this is not offered and not accepted.
const BookingLeadTime = 60 * time.Minute
