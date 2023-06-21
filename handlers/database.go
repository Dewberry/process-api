package handlers

import (
	"database/sql"

	"github.com/labstack/gommon/log"
	_ "github.com/mattn/go-sqlite3"
)

// Create tables in the database if they do not exist already
func createTables(db *sql.DB) {

	// SQLite does not have a built-in ENUM type or array type.
	// SQLite doesn't enforce the length of the VARCHAR datatype, therefore not using something like VARCHAR(30).

	queryJobs := `
	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		status TEXT NOT NULL,
		updated TIMESTAMP NOT NULL,
		mode TEXT NOT NULL,
		host TEXT NOT NULL,
		process_id TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_updated ON jobs(updated);
	CREATE INDEX IF NOT EXISTS idx_jobs_id ON jobs(id);
	CREATE INDEX IF NOT EXISTS idx_jobs_process_id ON jobs(process_id);
	`

	_, err := db.Exec(queryJobs)
	if err != nil {
		log.Fatal(err)
	}

	// Array of VARCHAR is represented as TEXT in SQLite. Client application has to handle conversion

	queryLogs := `
	CREATE TABLE IF NOT EXISTS logs (
		job_id TEXT,
		api_logs TEXT,
		container_logs TEXT,
		FOREIGN KEY (job_id) REFERENCES jobs(id)
	);

	CREATE INDEX IF NOT EXISTS idx_logs_job_id ON logs (job_id);
	`

	_, err = db.Exec(queryLogs)
	if err != nil {
		log.Fatal(err)
	}
}

// Initialize the database
func initDB(filepath string) *sql.DB {
	db, err := sql.Open("sqlite3", filepath)
	if err != nil {
		log.Fatalf("could not open %s delete the existing database to start with a fresh datbase: %s", filepath, err.Error())
	}

	if db == nil {
		log.Fatal("db nil")
	}

	db.SetMaxOpenConns(1)
	return db
}
