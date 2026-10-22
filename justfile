run:
	go run .

sqlc:
	sqlc generate

# Create the database and fill it with the example clinic.
seed:
	sqlite3 sqlite.db < sqlc/schema.sql
	sqlite3 sqlite.db < sqlc/seed.sql

watch:
	bash -c 'find . \( -name "*.go" -o -name "*.sql" \) -not -path "./database/*" | entr -r just compile'

compile: sqlc run

test:
	go test ./...
