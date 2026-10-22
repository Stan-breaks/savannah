CREATE TABLE IF NOT EXISTS doctors(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS working_hours (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  day_of_week INTEGER NOT NULL,
  start_time TEXT NOT NULL,
  end_time TEXT NOT NULL,
  lunch_start TEXT,
  lunch_end TEXT
);
CREATE TABLE IF NOT EXISTS doctor_working_hours (
  doctor_id INTEGER NOT NULL,
  working_hours_id INTEGER NOT NULL,
  PRIMARY KEY (doctor_id, working_hours_id),
  FOREIGN KEY (doctor_id) REFERENCES doctors(id),
  FOREIGN KEY (working_hours_id) REFERENCES working_hours(id)
);
CREATE TABLE IF NOT EXISTS patients(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS slots(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  start_time TEXT NOT NULL,
  end_time TEXT NOT NULL,
  date TEXT NOT NULL,
  is_available INTEGER NOT NULL DEFAULT 1,
  doctor_id INTEGER NOT NULL,
  FOREIGN KEY (doctor_id) REFERENCES doctors(id),
  UNIQUE (doctor_id, date, start_time)
);
CREATE TABLE IF NOT EXISTS bookings(
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  status TEXT NOT NULL DEFAULT 'CONFIRMED',
  slot_id INTEGER NOT NULL UNIQUE,
  patient_id INTEGER NOT NULL,
  FOREIGN KEY (slot_id) REFERENCES slots(id),
  FOREIGN KEY (patient_id) REFERENCES patients(id)
);
