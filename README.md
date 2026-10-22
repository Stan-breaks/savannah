# Savannah Informatics Take Home Assesement

## 1. Overview

The system allows patients to book 30-minute appointments with doctors at a small clinic. Patients can select a doctor and date, view available time slots, make a booking, and cancel an existing appointment.

The initial system supports five doctors but is designed to accommodate additional doctors and patients as the clinic grows.

**Technology stack**
- **Go:** Application logic and business-rule enforcement.
- **SQLite:** Relational database for persistent storage.
- **sqlc:** Generates type-safe Go code from SQL queries.

## 2. Data Models and Relationships

The system uses six tables, each with a specific responsibility.

| Model | Responsibility |
|---|---|
| `doctors` | Stores doctor information. |
| `working_hours` | Defines recurring weekly schedules, including start time, end time, and optional lunch breaks. |
| `doctor_working_hours` | Links doctors to working-hour schedules, allowing schedules to be shared. |
| `patients` | Stores patient information. |
| `slots` | Stores individual 30-minute appointment slots for a doctor on a specific date. |
| `bookings` | Records appointments made by patients and their booking status. |

### Relationships

- A doctor can have multiple working-hour schedules, and a schedule can be shared by multiple doctors.
- A doctor can have many appointment slots.
- A patient can make multiple bookings.
- A slot can have at most one **active** booking. A cancelled booking keeps the slot's history without blocking a new booking on it (section 9).

The database uses primary keys, foreign keys, and uniqueness constraints to maintain referential integrity and prevent duplicate reservations.

## 3. System Components

Three layers, one responsibility each. The HTTP layer knows about status codes, the service layer knows about clinic rules, the data layer knows about SQL. Nothing below the HTTP layer imports `net/http`, so every rule is testable without starting a server.

**HTTP layer (`router/`)**

Routes the request, validates the *shape* of the JSON body, and translates the outcome into a response. The error taxonomy lives here: the service reports *what* went wrong as a typed error, this layer decides the status code (section 7).

**Service layer (`service/`)**

Holds the clinic rules: deriving slots from working hours, deciding whether a requested time is bookable, and running booking, cancellation and rescheduling as atomic transactions. It owns the clock and the timezone, both injectable, so the time-sensitive rules are testable.

**Data access layer (`database/`, generated)**

sqlc turns the queries in `sqlc/query.sql` into type-safe Go methods. `database/` is generated and never edited by hand; the SQL files are the source of truth.

**Availability component**

The part of the design that deserves its own mention, because it answers "which slots can this patient book?":

1. The doctor's working hours for that weekday are turned into the 30-minute slots of the date, with lunch breaks carved out.
2. The `slots` table is reconciled with that schedule, so the table is a faithful picture of what exists.
3. The slots that are still free, have not started, and respect the one-hour lead time are returned.

Steps 1 and 2 make the schedule the single source of truth; step 3 is a plain read of the `slots` table.

## 4. Booking Workflow

1. **Select a doctor and date:** The patient chooses whom to see and when.
2. **Retrieve availability:** The system queries the slots for that doctor and date.
3. **Display available slots:** The application excludes unavailable or already-booked slots.
4. **Create a booking:** The application rechecks availability and records the booking within a database transaction.
5. **Confirm the appointment:** The selected slot becomes unavailable to other patients.
6. **Cancel a booking:** The booking status changes to `CANCELLED`, the reason is recorded, and the slot becomes available again.
7. **Reschedule a booking:** The appointment moves to another slot. The old slot is released, the new one is validated exactly like a fresh booking, and the appointment keeps its id.

Booking, cancellation and rescheduling must be atomic to avoid inconsistent states or two patients reserving the same slot concurrently.

## 5. Key Design Decisions

### Pre-generated appointment slots

Slots are stored in the database rather than generated for every availability request. The application generates slots from a doctor's working hours in 30-minute intervals, excluding lunch breaks and other unavailable periods.

This simplifies availability queries and makes individual slots easy to book. The trade-off is that slots must be regenerated or adjusted when a doctor's schedule changes.

The slots of a date are materialised on demand: whenever a schedule is consulted, the slots table is brought in line with the working hours, missing slots are inserted and slots that dropped out of the schedule are removed unless an appointment still refers to them.

### Shared working-hour schedules

Working hours are separated from doctors through the `doctor_working_hours` junction table. This avoids duplicating identical schedules when several doctors follow the same timetable.

The trade-off is an additional join when retrieving a doctor's schedule. If schedules eventually become doctor-specific, the relationship can be simplified.

### Separate slots and bookings

A slot represents an appointment opportunity, while a booking represents a patient's reservation. Keeping them separate allows the system to preserve cancelled booking records without deleting the underlying slot.

A unique constraint on `bookings.slot_id` prevents multiple booking records from claiming the same slot. The application must also manage slot availability consistently during booking and cancellation. See section 9 for how this constraint had to change.

### Business rules in Go

Go handles validation of appointment duration, working hours, lunch breaks, booking status, and cancellation rules. SQLite retains essential integrity constraints, such as foreign keys and uniqueness.

This keeps business logic easier to test and maintain while ensuring the database protects critical relationships.

### The schedule is the source of truth

Slots are not maintained by hand; they are derived from a doctor's working hours. A schedule change therefore cannot leave stale bookable slots behind, and the 30-minute grid is defined once, anchored at the start of a shift.

The trade-off is that the `slots` table has to be reconciled whenever a schedule is consulted, which makes the availability endpoint a read that occasionally writes. I accepted that because offering the right slots matters more than keeping the endpoint side-effect free, and the write is a handful of idempotent inserts.

### Booking by doctor, date and start time

A booking request names a doctor, a date and a start time rather than a slot id; the service resolves that to a slot row itself.

The trade-off is one extra lookup per request, in exchange for a client that can book a slot it never fetched, and for bookings that survive a rebuild of the `slots` table.

### Errors carry a machine readable code

Every failure has a status code, a stable code, and a message that says what to do about it. `422` means a well-formed request that breaks a rule, `400` means a request that is malformed. That keeps "your slot is outside working hours" distinguishable from "your date is not a date".

### No authentication

There is no auth layer, so endpoints are scoped by the ids in the path and in the body, and anyone who knows a doctor id can book with them. In a real deployment the patient identity would come from the session and `CancelBooking` would be scoped by `patient_id` as well. That query was scoped that way originally; removing it was a one-line change.

## 6. Running it

```sh
just seed   # creates sqlite.db and fills it with the example clinic
just run    # serves the API on :8080
just test   # runs the test suite
just sqlc   # regenerates database/ from sqlc/schema.sql and sqlc/query.sql
```

`database/` is generated by [sqlc](https://sqlc.dev) and must not be edited by hand. Change `sqlc/schema.sql` or `sqlc/query.sql` and run `just sqlc`.

The database path can be overridden with the `DB_PATH` environment variable.

## 7. API

All dates are `YYYY-MM-DD` and all times are `HH:MM` in the server's timezone.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/appointments` | Book a slot |
| `GET` | `/doctors/{id}/availability?date=YYYY-MM-DD` | List the bookable slots of a doctor on a date |
| `PATCH` | `/appointments/{id}/cancel` | Cancel an appointment and release its slot |
| `PATCH` | `/appointments/{id}/reschedule` | Move an appointment to another slot |
| `GET` | `/patients/{id}/appointments` | List the upcoming appointments of a patient |

### Booking an appointment

```sh
curl -X POST http://localhost:8080/appointments \
  -d '{"patient_id": 1, "doctor_id": 1, "date": "2026-10-24", "start_time": "10:00"}'
```

```json
{
  "id": 1,
  "status": "CONFIRMED",
  "slot_id": 3,
  "patient_id": 1,
  "doctor_id": 1,
  "doctor_name": "Dr. House",
  "date": "2026-10-24",
  "start_time": "10:00",
  "end_time": "10:30"
}
```

The slot has to fall inside the doctor's working hours, must not overlap a lunch break, must start on a 30-minute boundary, must be at least an hour away and must still be free.

### Availability

```sh
curl "http://localhost:8080/doctors/1/availability?date=2026-10-24"
```

```json
{
  "doctor_id": 1,
  "date": "2026-10-24",
  "slots": [
    {"id": 1, "start_time": "09:00", "end_time": "09:30", "date": "2026-10-24", "doctor_id": 1}
  ]
}
```

Slots that already started or that start within the next hour are left out, because they cannot be booked.

### Cancelling and rescheduling

```sh
curl -X PATCH http://localhost:8080/appointments/1/cancel \
  -d '{"reason": "patient is ill"}'

curl -X PATCH http://localhost:8080/appointments/1/reschedule \
  -d '{"date": "2026-10-24", "start_time": "11:00"}'
```

Cancelling records the reason and releases the slot, so it can be booked again. Rescheduling releases the old slot and validates the new one exactly like a fresh booking. The `doctor_id` of a reschedule is optional and defaults to the doctor of the appointment.

### Errors

Every failure answers with a machine readable code and a message that says what went wrong.

```json
{"error": "slot 10:00 on 2026-10-24 is already booked", "code": "slot_already_booked"}
```

| Status | When |
|---|---|
| `400` | The request is malformed: bad JSON, unknown field, missing date, unparseable id |
| `404` | The doctor, patient or appointment does not exist |
| `409` | The slot is already taken, or the appointment was cancelled before |
| `422` | The request breaks a rule: outside working hours, inside a lunch break, in the past, too close to now |

## 8. Layout

```
main.go            bootstraps the database, the service and the router
service/           the booking rules, independent of HTTP
  slots.go           deriving and validating slots from working hours
  slotstore.go       keeping the slots table in line with the schedule
  appointments.go    booking, cancelling, rescheduling, availability
  errors.go          typed failures, mapped to status codes by the router
  policy.go          the one-hour booking lead time
  *_test.go          the rules, tested against a real SQLite file
router/            the HTTP layer
  apis/              one file per resource, shared JSON helpers
  router_test.go     the endpoints, tested for status codes and bodies
sqlc/              schema.sql, query.sql and seed.sql, the source of truth
database/          generated by sqlc, do not edit
```

## 9. Changes to the original design

Two things in the original schema did not allow the required behaviour:

- `bookings.slot_id` was declared `UNIQUE` at the table level, so a slot could only ever be claimed once. Cancelling a booking left the row behind and the slot could never be booked again. The constraint is now a partial unique index that only covers active bookings, so cancelled bookings keep their history while their slot returns to the pool.
- A cancellation had nowhere to be recorded, so `bookings` gained a `cancel_reason` column.

Working hours use the SQLite convention for `day_of_week`: `0` is Sunday and `6` is Saturday.

## 10. How I used AI

Short version: I used AI as an implementation assistant and as a second pair of eyes. The design, the schema and the component boundaries are my own work — they were written before I used an AI tool, and they drove everything that followed.

### 1. What did you use AI for across the four sections?

Across the four areas of the workflow, AI wrote the implementation. I wrote the rules it implements.

| Area | AI produced | I decided |
|---|---|---|
| Availability | Slot generation from working hours, reconciliation of the `slots` table, the availability query and its tests | That working hours are the source of truth, that slots are materialised on demand, and that the endpoint hides slots inside the lead time |
| Booking | The transaction, the validation calls and the tests | That the request is identified by doctor + date + start time, and that booking flips the slot inside the same transaction |
| Cancellation | The update, the slot release and the tests | That a reason is mandatory, that a second cancellation is a conflict, and that the slot has to become bookable again |
| Rescheduling | The move, the validation of the new slot and the tests | That the new slot is validated exactly like a fresh booking and that the appointment keeps its id |

Not delegated at all: the data model, the shared `doctor_working_hours` junction table, sqlc as the query layer, the error taxonomy, and dropping auth.

### 2. Give one example where an AI suggestion improved your work. What did you prompt it with?

**Prompt:** *"The slot must become bookable again after a cancellation. Here is my schema: `bookings.slot_id INTEGER NOT NULL UNIQUE`."*

**Suggestion:** the table-level `UNIQUE` is the blocker, because the cancelled row keeps the slot claimed forever. Replace it with a partial unique index:

```sql
CREATE UNIQUE INDEX idx_bookings_confirmed_slot
  ON bookings(slot_id) WHERE status = 'CONFIRMED';
```

**Why it was better:** enforcing "one booking per slot" in Go, inside a transaction, still leaves a window between two concurrent requests. Moving the guarantee into a partial index closes that window in the database, keeps cancelled bookings as history, and still lets a cancelled slot be booked again. I kept it and wrote two tests for it: a second active booking on a slot is rejected, and a cancelled slot can be booked again.

### 3. Give one example where AI output was wrong or incomplete and how you caught it

**Wrong:** the check for a duplicate booking against SQLite's error code.

```go
// AI produced this
if sqliteErr.Code == sqlite3.ErrConstraintUnique { ... }
```

It does not compile: `Error.Code` is `ErrNo`, while `ErrConstraintUnique` is an `ErrNoExtended`. `go vet` rejected it. The fix was to compare `ExtendedCode`, and I added a test that moves a second appointment onto a taken slot and asserts the error is recognised as a constraint violation rather than surfacing as a 500.

**Other things I had to correct:**

- sqlc generated `Datetime interface{}` for the cutoff parameter of the upcoming-appointments query — untyped and unusable. I reproduced the variants in a scratch project, compared them, and settled on the string comparison sqlc names `Date string`, documented in `sqlc/query.sql` so the next reader is not confused by the name.
- A test I had written expected `422` for a malformed date. The service returned `400`, which is the right answer: a malformed date is not a broken business rule. The failing test corrected my assumption, not the code.

### 4. Name two decisions you made without AI. Why did you trust your own judgment there?

**A junction table for working hours.** I put the schedule in its own table and linked it to doctors, instead of putting `day_of_week` and `start_time` columns on `doctors`. Five doctors who share the same weekday timetable should not mean five copies of it, and a doctor with a split shift needs two rows. I trusted this because the duplication would be permanent while the join cost is one extra query on a table with a handful of rows.

**sqlc, with hand-written SQL as the source of truth.** Queries live in `sqlc/query.sql`, `database/` is generated and must never be edited by hand. I trusted this because the queries stay reviewable as SQL and the result types are checked at compile time, with no runtime mapping layer to get wrong. The cost — a generated directory that must not be touched — I documented in the README and enforced by committing the generated code instead of ignoring it.
