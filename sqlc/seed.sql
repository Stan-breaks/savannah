-- Seed data for the clinic.
--
-- The schedule rows are shared between doctors on purpose: the
-- doctor_working_hours table exists so several doctors can follow the same
-- timetable without duplicating it.
--
-- day_of_week follows the SQLite convention: 0 = Sunday ... 6 = Saturday.

-- Doctors
INSERT OR IGNORE INTO doctors (id, name) VALUES (1, 'Dr. House');
INSERT OR IGNORE INTO doctors (id, name) VALUES (2, 'Dr. Wilson');
INSERT OR IGNORE INTO doctors (id, name) VALUES (3, 'Dr. Grey');
INSERT OR IGNORE INTO doctors (id, name) VALUES (4, 'Dr. Trudy');
INSERT OR IGNORE INTO doctors (id, name) VALUES (5, 'Dr. Chase');

-- Working hours
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (1, 1, '09:00', '17:00', '13:00', '14:00');
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (2, 2, '09:00', '17:00', '13:00', '14:00');
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (3, 3, '09:00', '17:00', '13:00', '14:00');
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (4, 4, '09:00', '17:00', '13:00', '14:00');
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (5, 5, '09:00', '17:00', '13:00', '14:00');
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (6, 1, '08:00', '12:00', NULL, NULL);
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (7, 3, '08:00', '12:00', NULL, NULL);
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (8, 5, '08:00', '12:00', NULL, NULL);
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (9, 2, '14:00', '18:00', NULL, NULL);
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (10, 4, '14:00', '18:00', NULL, NULL);
INSERT OR IGNORE INTO working_hours (id, day_of_week, start_time, end_time, lunch_start, lunch_end) VALUES (11, 6, '09:00', '13:00', NULL, NULL);

-- Doctor schedules
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (1, 1);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (1, 2);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (1, 3);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (1, 4);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (1, 5);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (1, 11);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (2, 1);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (2, 2);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (2, 3);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (2, 4);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (2, 5);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (3, 6);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (3, 7);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (3, 8);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (3, 9);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (3, 10);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (4, 1);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (4, 2);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (4, 3);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (4, 4);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (4, 5);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (5, 6);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (5, 7);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (5, 8);
INSERT OR IGNORE INTO doctor_working_hours (doctor_id, working_hours_id) VALUES (5, 11);

-- Patients
INSERT OR IGNORE INTO patients (id, name) VALUES (1, 'Alice');
INSERT OR IGNORE INTO patients (id, name) VALUES (2, 'Bob');
INSERT OR IGNORE INTO patients (id, name) VALUES (3, 'Carol');
INSERT OR IGNORE INTO patients (id, name) VALUES (4, 'Dave');
INSERT OR IGNORE INTO patients (id, name) VALUES (5, 'Eve');
