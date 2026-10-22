#!/usr/bin/env bash
#
# Fills an empty database with the example clinic.
#
# It does nothing if the database already has doctors, so it is safe to
# run on every deployment. The service runs as web, so the database has
# to belong to web as well.
set -euo pipefail

APP_DIR="${APP_DIR:-/var/www/savannah}"
DB="$APP_DIR/sqlite.db"

if ! command -v sqlite3 >/dev/null; then
    echo "sqlite3 is not installed, skipping the seed"
    exit 0
fi

if [ ! -f "$DB" ]; then
    touch "$DB"
fi

# The database is written by whoever runs this, and the service writes to
# it as web, so it has to belong to web.
chown web:web "$DB" 2>/dev/null || true

if [ -n "$(sqlite3 "$DB" 'SELECT id FROM doctors LIMIT 1;' 2>/dev/null)" ]; then
    exit 0
fi

if ! sqlite3 "$DB" < "$APP_DIR/sqlc/schema.sql" 2>/dev/null; then
    echo "could not write $DB, skipping the seed"
    echo "fix it with: chown web:web $DB"
    exit 0
fi

sqlite3 "$DB" < "$APP_DIR/sqlc/seed.sql"
echo "seeded $DB"
