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
- Each slot can have at most one booking.

The database uses primary keys, foreign keys, and uniqueness constraints to maintain referential integrity and prevent duplicate reservations.

## 3. System Components

**Application layer (Go)**

Handles incoming requests, validates input, applies business rules, and coordinates database operations. It is responsible for checking that appointments fall within working hours, avoid lunch breaks, and are available before booking.

**Database layer (SQLite + sqlc)**

SQLite stores the application data, while sqlc generates type-safe Go methods from SQL queries. This keeps database access explicit and reduces errors caused by manually mapping query results.

**Availability component**

Retrieves the slots for a selected doctor and date and identifies which ones remain available for booking. Only slots that are available and have no active booking should be offered to patients.

## 4. Booking Workflow

1. **Select a doctor and date:** The patient chooses whom to see and when.
2. **Retrieve availability:** The system queries the slots for that doctor and date.
3. **Display available slots:** The application excludes unavailable or already-booked slots.
4. **Create a booking:** The application rechecks availability and records the booking within a database transaction.
5. **Confirm the appointment:** The selected slot becomes unavailable to other patients.
6. **Cancel a booking:** The booking status changes to `CANCELLED`, and the slot becomes available again.

Booking and cancellation operations must be atomic to avoid inconsistent states or two patients reserving the same slot concurrently.

## 5. Key Design Decisions

### Pre-generated appointment slots

Slots are stored in the database rather than generated for every availability request. The application generates slots from a doctor's working hours in 30-minute intervals, excluding lunch breaks and other unavailable periods.

This simplifies availability queries and makes individual slots easy to book. The trade-off is that slots must be regenerated or adjusted when a doctor's schedule changes.

### Shared working-hour schedules

Working hours are separated from doctors through the `doctor_working_hours` junction table. This avoids duplicating identical schedules when several doctors follow the same timetable.

The trade-off is an additional join when retrieving a doctor's schedule. If schedules eventually become doctor-specific, the relationship can be simplified.

### Separate slots and bookings

A slot represents an appointment opportunity, while a booking represents a patient's reservation. Keeping them separate allows the system to preserve cancelled booking records without deleting the underlying slot.

A unique constraint on `bookings.slot_id` prevents multiple booking records from claiming the same slot. The application must also manage slot availability consistently during booking and cancellation.

### Business rules in Go

Go handles validation of appointment duration, working hours, lunch breaks, booking status, and cancellation rules. SQLite retains essential integrity constraints, such as foreign keys and uniqueness.

This keeps business logic easier to test and maintain while ensuring the database protects critical relationships.

