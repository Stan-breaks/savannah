package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"savannah/database"
	"savannah/router"
	"savannah/service"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

const (
	// defaultDBPath is the SQLite file the application works with.
	defaultDBPath = "sqlite.db"
	// schemaPath holds the schema the database is initialised from.
	schemaPath = "sqlc/schema.sql"
	// listenAddress is the address the server binds to.
	listenAddress = ":8089"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Println("warning: no .env file found, relying on environment variables")
	}

	db, err := sql.Open("sqlite3", dsn())
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	if err := initializeSchema(db); err != nil {
		log.Fatalf("Error initializing schema: %v", err)
	}

	queries := database.New(db)
	svc := service.New(db, queries)
	route := router.NewRouter(svc)

	log.Printf("savannah server started on %s", listenAddress)
	log.Fatal(http.ListenAndServe(listenAddress, route.Handler()))
}

// dsn builds the SQLite connection string. The pragmas are part of the DSN
// because SQLite applies them per connection and the pool hands out more
// than one connection.
func dsn() string {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = defaultDBPath
	}
	return fmt.Sprintf(
		"file:%s?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL&_txlock=immediate",
		path,
	)
}

// initializeSchema applies the schema to an empty database.
func initializeSchema(db *sql.DB) error {
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		return fmt.Errorf("failed to apply schema: %w", err)
	}
	log.Println("Schema initialized successfully")
	return nil
}
