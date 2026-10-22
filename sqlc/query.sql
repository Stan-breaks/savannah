-- DOCTORS

-- name: CreateDoctor :one
INSERT INTO doctors (name)
VALUES (?)
RETURNING *;

-- name: GetDoctorById :one
SELECT * FROM doctors
WHERE id = ? LIMIT 1;

-- name: GetAllDoctors :many
SELECT * FROM doctors
ORDER BY name;

-- name: DeleteDoctor :exec
DELETE FROM doctors
WHERE id = ?;


-- WORKING HOURS

-- name: CreateWorkingHours :one
INSERT INTO working_hours (
    day_of_week,
    start_time,
    end_time,
    lunch_start,
    lunch_end
)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWorkingHoursById :one
SELECT * FROM working_hours
WHERE id = ? LIMIT 1;

-- name: GetWorkingHoursByDoctor :many
SELECT wh.*
FROM working_hours wh
JOIN doctor_working_hours dwh
    ON dwh.working_hours_id = wh.id
WHERE dwh.doctor_id = ?
ORDER BY wh.day_of_week, wh.start_time;

-- name: GetAllWorkingHours :many
SELECT * FROM working_hours
ORDER BY day_of_week, start_time;

-- name: UpdateWorkingHours :one
UPDATE working_hours
SET day_of_week = ?,
    start_time = ?,
    end_time = ?,
    lunch_start = ?,
    lunch_end = ?
WHERE id = ?
RETURNING *;

-- name: DeleteWorkingHours :exec
DELETE FROM working_hours
WHERE id = ?;


-- DOCTOR-WORKING-HOURS RELATIONSHIP

-- name: AssignWorkingHoursToDoctor :exec
INSERT INTO doctor_working_hours (
    doctor_id,
    working_hours_id
)
VALUES (?, ?)
ON CONFLICT (doctor_id, working_hours_id) DO NOTHING;

-- name: RemoveWorkingHoursFromDoctor :exec
DELETE FROM doctor_working_hours
WHERE doctor_id = ?
  AND working_hours_id = ?;


-- PATIENTS

-- name: CreatePatient :one
INSERT INTO patients (name)
VALUES (?)
RETURNING *;

-- name: GetPatientById :one
SELECT * FROM patients
WHERE id = ? LIMIT 1;

-- name: GetAllPatients :many
SELECT * FROM patients
ORDER BY name;

-- name: DeletePatient :exec
DELETE FROM patients
WHERE id = ?;


-- SLOTS

-- name: CreateSlot :one
INSERT INTO slots (
    start_time,
    end_time,
    date,
    is_available,
    doctor_id
)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSlotById :one
SELECT * FROM slots
WHERE id = ? LIMIT 1;

-- name: GetSlotsByDoctorAndDate :many
SELECT *
FROM slots
WHERE doctor_id = ?
  AND date = ?
ORDER BY start_time;

-- name: GetAvailableSlotsByDoctorAndDate :many
SELECT s.*
FROM slots s
WHERE s.doctor_id = ?
  AND s.date = ?
  AND s.is_available = 1
  AND NOT EXISTS (
      SELECT 1
      FROM bookings b
      WHERE b.slot_id = s.id
        AND b.status = 'CONFIRMED'
  )
ORDER BY s.start_time;

-- name: GetAvailableSlotsByDate :many
SELECT s.*
FROM slots s
WHERE s.date = ?
  AND s.is_available = 1
  AND NOT EXISTS (
      SELECT 1
      FROM bookings b
      WHERE b.slot_id = s.id
        AND b.status = 'CONFIRMED'
  )
ORDER BY s.start_time;

-- name: UpdateSlotAvailability :exec
UPDATE slots
SET is_available = ?
WHERE id = ?;

-- name: DeleteSlot :exec
DELETE FROM slots
WHERE id = ?;


-- BOOKINGS

-- name: CreateBooking :one
INSERT INTO bookings (
    status,
    slot_id,
    patient_id
)
SELECT
    'CONFIRMED',
    s.id,
    ?
FROM slots s
WHERE s.id = ?
  AND s.is_available = 1
  AND NOT EXISTS (
      SELECT 1
      FROM bookings b
      WHERE b.slot_id = s.id
        AND b.status = 'CONFIRMED'
  )
RETURNING *;

-- name: GetBookingById :one
SELECT * FROM bookings
WHERE id = ? LIMIT 1;

-- name: GetBookingBySlotId :one
SELECT * FROM bookings
WHERE slot_id = ?
  AND status = 'CONFIRMED'
LIMIT 1;

-- name: GetBookingsByPatient :many
SELECT
    b.*,
    s.doctor_id,
    s.date,
    s.start_time,
    s.end_time
FROM bookings b
JOIN slots s ON s.id = b.slot_id
WHERE b.patient_id = ?
ORDER BY s.date DESC, s.start_time DESC;

-- name: GetBookingsByDoctorAndDate :many
SELECT
    b.*,
    s.doctor_id,
    s.date,
    s.start_time,
    s.end_time,
    p.name AS patient_name
FROM bookings b
JOIN slots s ON s.id = b.slot_id
JOIN patients p ON p.id = b.patient_id
WHERE s.doctor_id = ?
  AND s.date = ?
  AND b.status = 'CONFIRMED'
ORDER BY s.start_time;

-- name: CancelBooking :execrows
UPDATE bookings
SET status = 'CANCELLED'
WHERE id = ?
  AND patient_id = ?
  AND status = 'CONFIRMED';

-- name: GetAllBookings :many
SELECT
    b.*,
    p.name AS patient_name,
    s.doctor_id,
    s.date,
    s.start_time,
    s.end_time
FROM bookings b
JOIN patients p ON p.id = b.patient_id
JOIN slots s ON s.id = b.slot_id
ORDER BY s.date DESC, s.start_time DESC;

