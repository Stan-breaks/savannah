# Savannah Informatics Take Home Assessment

## 1. Overview

A small clinic booking system. Patients pick a doctor and a date, see the free 30-minute slots, book one, and cancel or reschedule later if they need to.

It starts with five doctors, but nothing in the design is tied to that number. Adding doctors or patients is just adding rows.

Stack:
- Go for the application logic and the business rules
- SQLite for storage
- sqlc to generate type-safe Go code from the SQL queries

## 2. Data models and relationships

Six tables:

| Model | Responsibility |
|---|---|
| `doctors` | Doctor information. |
| `working_hours` | Recurring weekly schedules: start time, end time, optional lunch break. |
| `doctor_working_hours` | Links doctors to schedules, so a schedule can be shared. |
| `patients` | Patient information. |
| `slots` | Individual 30-minute slots for a doctor on a specific date. |
| `bookings` | Appointments made by patients, and their status. |

### Relationships

- A doctor can have several working-hour schedules, and a schedule can belong to several doctors.
- A doctor has many slots.
- A patient can have many bookings.
- A slot has at most one **active** booking. A cancelled booking stays in the table as history but doesn't block the slot from being booked again (see section 10).

Primary keys, foreign keys and unique constraints handle integrity and stop double reservations.

## 3. System components

There are three layers. The HTTP layer deals with status codes, the service layer deals with clinic rules, and the data layer deals with SQL. Nothing below the HTTP layer imports `net/http`, so the rules can be tested without starting a server.

**HTTP layer (`router/`)**

Routes requests, checks that the JSON body has the right shape, and turns the result into a response. The service says what went wrong as a typed error, and this layer picks the status code (section 7).

**Service layer (`service/`)**

The clinic rules live here: turning working hours into slots, deciding whether a requested time can be booked, and running booking, cancelling and rescheduling as transactions. It also owns the clock and the timezone. Both can be injected, which is what makes the time-based rules testable.

**Data access layer (`database/`, generated)**

sqlc turns the queries in `sqlc/query.sql` into Go methods. `database/` is generated and never edited by hand. The SQL files are the source of truth.

**Availability**

This is the part that answers "which slots can this patient book?". It works in three steps:

1. Take the doctor's working hours for that weekday and cut them into 30-minute slots, leaving out lunch.
2. Bring the `slots` table in line with that list.
3. Return the slots that are free, haven't started, and are at least an hour away.

Steps 1 and 2 make the schedule the source of truth. Step 3 is a plain read of the `slots` table.

## 4. Booking workflow

1. **Pick a doctor and date.**
2. **Get availability.** The system looks up the slots for that doctor and date.
3. **Show the free slots.** Booked or unavailable ones are left out.
4. **Book.** The service checks availability again and records the booking inside a database transaction.
5. **Confirm.** The slot is now taken for everyone else.
6. **Cancel.** The status becomes `CANCELLED`, the reason is stored, and the slot is free again.
7. **Reschedule.** The old slot is released and the new one is validated like a fresh booking. The appointment keeps its id.

Booking, cancelling and rescheduling are all atomic, so a failure halfway can't leave things inconsistent and two patients can't end up with the same slot.

## 5. Design decisions

### Pre-generated slots

Slots are stored in the database instead of being computed on every availability request. They're generated from a doctor's working hours in 30-minute steps, minus lunch breaks.

That keeps availability queries simple and makes each slot easy to book. The cost is that slots have to be adjusted when a schedule changes. I handle that by materialising the slots of a date on demand: whenever a schedule is looked at, missing slots are inserted and slots that no longer belong to the schedule are removed, unless an appointment still points at them.

### Shared working-hour schedules

Working hours sit in their own table, linked to doctors through `doctor_working_hours`. Doctors with the same timetable share one row instead of each having a copy. It costs an extra join when reading a doctor's schedule. If every schedule ends up being doctor-specific, this could be simplified.

### Separate slots and bookings

A slot is an opportunity and a booking is a patient's claim on it. Keeping them apart lets me keep cancelled bookings without deleting the slot. A unique constraint on `bookings.slot_id` stops two bookings from claiming one slot, though as section 10 explains, that constraint had to change.

### Business rules in Go

Go checks appointment length, working hours, lunch breaks, booking status and cancellation rules. SQLite only keeps the basic integrity constraints, like foreign keys and uniqueness. I find the rules easier to test this way.

### The schedule is the source of truth

Slots aren't edited by hand. They're derived from working hours, so changing a schedule can't leave stale bookable slots around. The 30-minute grid starts at the beginning of a shift.

The downside is that the availability endpoint is a read that sometimes writes, because it reconciles the table when it's called. I'm fine with that. The writes are a few idempotent inserts, and showing the wrong slots is worse than an endpoint that isn't purely read-only.

### Booking by doctor, date and start time

A booking request gives a doctor, a date and a start time, not a slot id. The service finds the slot itself. That's one extra lookup per request, but a client can book a time it never fetched, and bookings still work if the `slots` table gets rebuilt.

### Errors carry a code

Every error has a status code, a stable `code`, and a message saying what to do about it. `400` means the request is malformed and `422` means it's well-formed but breaks a rule. That way "your slot is outside working hours" can't be confused with "your date isn't a date".

### No authentication

There's no auth. Endpoints are scoped by the ids in the path and body, so anyone who knows a doctor id can book with it. In a real deployment the patient would come from the session, and `CancelBooking` would also be scoped by `patient_id`. That query was scoped that way at first, and removing it was a one-line change.

## 6. Running it

```sh
just seed   # creates sqlite.db and fills it with the example clinic
just run    # serves the API on :8089
just test   # runs the test suite
just sqlc   # regenerates database/ from sqlc/schema.sql and sqlc/query.sql
```

`database/` is generated by [sqlc](https://sqlc.dev) and shouldn't be edited by hand. Change `sqlc/schema.sql` or `sqlc/query.sql` and run `just sqlc`.

Set `DB_PATH` to use a different database file.

## 7. API

Dates are `YYYY-MM-DD` and times are `HH:MM`, in the server's timezone.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/appointments` | Book a slot |
| `GET` | `/doctors/{id}/availability?date=YYYY-MM-DD` | List the bookable slots of a doctor on a date |
| `PATCH` | `/appointments/{id}/cancel` | Cancel an appointment and release its slot |
| `PATCH` | `/appointments/{id}/reschedule` | Move an appointment to another slot |
| `GET` | `/patients/{id}/appointments` | List the upcoming appointments of a patient |

### Booking an appointment

```sh
curl -X POST http://localhost:8089/appointments \
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

The slot has to be inside the doctor's working hours, can't overlap a lunch break, must start on a 30-minute boundary, must be at least an hour away, and must still be free.

### Availability

```sh
curl "http://localhost:8089/doctors/1/availability?date=2026-10-24"
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

Slots that already started, or start within the next hour, are left out because they can't be booked.

### Cancelling and rescheduling

```sh
curl -X PATCH http://localhost:8089/appointments/1/cancel \
  -d '{"reason": "patient is ill"}'

curl -X PATCH http://localhost:8089/appointments/1/reschedule \
  -d '{"date": "2026-10-24", "start_time": "11:00"}'
```

Cancelling stores the reason and frees the slot so it can be booked again. Rescheduling frees the old slot and validates the new one like a fresh booking. `doctor_id` is optional on a reschedule and defaults to the appointment's current doctor.

### Errors

Every failure returns a code and a message.

```json
{"error": "slot 10:00 on 2026-10-24 is already booked", "code": "slot_already_booked"}
```

| Status | When |
|---|---|
| `400` | Malformed request: bad JSON, unknown field, missing date, unparseable id |
| `404` | The doctor, patient or appointment doesn't exist |
| `409` | The slot is already taken, or the appointment was already cancelled |
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
.github/workflows/ ci.yml and deploy.yml, the pipeline
deploy/            the systemd unit, the deploy script, the seed script
                  and the setup script
```

## 9. Deployment

The API runs on a VPS as a systemd service, and a merge into `main` is what deploys it.

**Live URL:** <https://savannah.stanbreaks.com>

### The pipeline

Two workflows, both in `.github/workflows/`:

| Workflow | Trigger | What it does |
|---|---|---|
| `ci.yml` | every pull request to `main` | regenerates the sqlc code and fails if the result is out of date, then runs `go vet`, `go test` and `go build` |
| `deploy.yml` | every push to `main` | runs the same checks, then sshes into the server and runs `deploy/deploy.sh` |

A merge into `main` produces exactly one push to `main`, so `deploy.yml` runs once per merge. It can also be triggered by hand from the Actions tab.

### How the deployment works

The runner holds an SSH key in the `DEPLOY_SSH_KEY` secret. It logs in as `web` and runs `deploy/deploy.sh` on the server, which does four things:

```sh
cd /var/www/savannah
git pull --ff-only
go build -o savannah .
bash deploy/seed.sh
sudo systemctl restart savannah.service
```

The script runs on the server, so it pulls the same checkout the service runs from. `go build` swaps the new binary in with a rename, which works even while the old one is still running, and the service is only restarted once the new binary is in place. If the build fails, the old one keeps serving.

`deploy/seed.sh` fills an empty database with the example clinic. It does nothing once the database has doctors in it, so it is safe on every deployment, and a server that lost its data gets it back on the next merge.

The `web` user is allowed to restart the service and nothing else, through a single sudoers rule.

`deploy/install.sh` sets a server up the first time, and `deploy/README.md` has the details.

## 10. Changes to the original design

Two things in the first schema didn't allow the behaviour the task asks for:

- `bookings.slot_id` was `UNIQUE` at the table level, so a slot could only ever be claimed once. Cancelling left the row behind and the slot could never be booked again. It's now a partial unique index that only covers active bookings, so cancelled bookings stay as history and their slot goes back into the pool.
- Cancellations had nowhere to record a reason, so `bookings` got a `cancel_reason` column.

Working hours use SQLite's `day_of_week` convention: `0` is Sunday and `6` is Saturday.

## 11. How I used AI

I used AI as a pair programmer. It wrote most of the code. I wrote the schema, the rules and the design decisions, and I read every diff and ran the tests before accepting anything.

For the schema, I designed the six tables myself, mainly the split between `working_hours` and `doctor_working_hours`, because a lot of the rest depends on it. I used AI afterwards to review it for missing foreign keys and columns the queries would need. It also wrote the sqlc queries, since once the schema is fixed that's mostly translation.

For the service layer, I wrote the rules in plain prose first: what the slots of a shift are, what happens to a slot when its appointment is cancelled, and in what order the checks run. AI turned that into Go, including the loop that walks a shift in 30-minute steps, the lunch-break overlap check, and the transaction bodies for book, cancel and reschedule. The reconciliation in `service/slotstore.go` is the part I had to push on. The first draft inserted missing slots but never removed ones that had dropped out of a doctor's schedule, which is the exact case the "schedule is the source of truth" decision is about.

The HTTP layer was almost all AI, since it's a direct translation of the endpoint table. The `400` versus `422` split and `DisallowUnknownFields` were my decisions. For tests, I listed what had to be true and AI wrote the bodies. The service tests use a real SQLite file in `t.TempDir()` with the real generated queries, and no fake database.

The best AI suggestion was the injectable clock. My first version called `time.Now()` inline in the booking path, so any test of the lead-time rule only passed at the right time of day. I asked for the clock and the timezone to be passed in as functional options, defaulting to `time.Now` and `time.Local`. I would have stopped at a fake clock and left the timezone as `time.Local`, which would make the tests depend on the machine, because the seeded hours are by weekday. Injecting the location pins the weekday too. My prompt was roughly:

> The service calls time.Now() in three places: the lead time check, the "has already started" check, and the filter that drops slots from the availability response. Rewrite it so the clock and the timezone are both injected through functional options on the constructor, defaulting to time.Now and time.Local. Keep the method signatures that do not need to change, do not introduce an interface for the clock, a func() time.Time is enough, and give me a fake clock in the test file so the lead time rule can be tested at a fixed moment.

The first answer included a `Clock` interface anyway, which I removed.

The clearest wrong output was the unique constraint. The first schema draft had `slot_id INTEGER NOT NULL UNIQUE`, which is the obvious way to say one booking per slot, and it's wrong here because a cancelled row keeps the slot claimed forever. Nothing complained until I wrote the cancel-then-rebook test, which failed with a unique violation. I replaced the constraint with a partial index:

```sql
CREATE UNIQUE INDEX idx_bookings_confirmed_slot
  ON bookings(slot_id)
  WHERE status = 'CONFIRMED';
```

`isUniqueViolation` in `service/appointments.go` turns that error into a `409` instead of a `500`, and `service/constraints_test.go` checks both cases. The other miss was `service.Error`, which had a `Cause` field, a `withCause` helper and an `Unwrap() []error`. Nothing ever set the cause, so I deleted it and kept a single `Unwrap() error` that exposes the kind, which is what the router checks.

Two decisions were mine. First, the `slots` table is built from working hours, and the availability endpoint writes to it. The alternative was computing slots on every request and storing only bookings. I went the other way because a schedule change shouldn't leave a bookable slot behind, and the cost is a few inserts. Second, booking takes a doctor, date and start time instead of a slot id. Slots get deleted and recreated when schedules change, so a client holding a slot id can end up asking for something that no longer exists. Naming the doctor and the time is what the patient actually means.



