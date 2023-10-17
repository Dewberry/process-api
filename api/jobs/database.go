package jobs

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/gommon/log"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	Handle *sql.DB
}

// Create tables in the database if they do not exist already
func (db *DB) createTables() {

	// SQLite does not have a built-in ENUM type or array type.
	// SQLite doesn't enforce the length of the VARCHAR datatype, therefore not using something like VARCHAR(30).
	// SQLite's concurrency control is based on transactions, not connections. A connection to a SQLite database does not inherently acquire a lock.
	// Locks are acquired when a transaction is started and released when the transaction is committed or rolled back.

	// indices needed to speedup
	// fetching jobs for a particular process id
	// providing job-lists ordered by time

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
	CREATE INDEX IF NOT EXISTS idx_jobs_process_id ON jobs(process_id);
	`

	_, err := db.Handle.Exec(queryJobs)
	if err != nil {
		log.Fatal(err)
	}
}

// Initialize the database.
// Creates intermediate directories if not exist.
func InitDB(dbPath string) *DB {

	// Create directory structure if it doesn't exist
	dir := filepath.Dir(dbPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		log.Fatalf(err.Error())
	}

	h, err := sql.Open("sqlite3", dbPath+"?mode=rwc")
	// it maybe a good idea to check here if the connections has write privilege https://stackoverflow.com/a/44707371/11428410 https://www.sqlite.org/c3ref/db_readonly.html
	// also maybe we should make db such that only go can write to it

	if err != nil {
		log.Fatalf("could not open %s Delete the existing database to start with a new database. Error: %s", dbPath, err.Error())
	}

	if h == nil {
		log.Fatal("db nil")
	}

	db := DB{Handle: h}
	db.createTables()
	return &db
}

// Add job the database. Will return error if job exist.
func (db *DB) addJob(jid string, status string, updated time.Time, mode string, host string, process_id string) error {
	query := `INSERT INTO jobs (id, status, updated, mode, host, process_id) VALUES (?, ?, ?, ?, ?, ?)`

	_, err := db.Handle.Exec(query, jid, status, updated, mode, host, process_id)
	if err != nil {
		return err
	}
	return nil
}

// Update status and time of a job.
func (db *DB) updateJobRecord(jid string, status string, now time.Time) {
	query := `UPDATE jobs SET status = ?, updated = ? WHERE id = ?`
	_, err := db.Handle.Exec(query, status, now, jid)
	if err != nil {
		log.Error(err)
	}
}

// Get Job Record from database given a job id.
// If job do not exists, or error encountered bool would be false.
// Similar behavior as key exist in hashmap.
func (db *DB) GetJob(jid string) (JobRecord, bool) {
	query := `SELECT * FROM jobs WHERE id = ?`

	js := JobRecord{}

	row := db.Handle.QueryRow(query, jid)
	err := row.Scan(&js.JobID, &js.Status, &js.LastUpdate, &js.Mode, &js.Host, &js.ProcessID)
	if err != nil {
		if err == sql.ErrNoRows {
			return JobRecord{}, false
		} else {
			log.Error(err)
			return JobRecord{}, false
		}
	}
	return js, true
}

// Check if a job exists in database.
func (db *DB) CheckJobExist(jid string) bool {
	query := `SELECT id FROM jobs WHERE id = ?`

	js := JobRecord{}

	row := db.Handle.QueryRow(query, jid)
	err := row.Scan(&js.JobID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false
		} else {
			log.Error(err)
			return false
		}
	}
	return true
}

// Assumes query parameters are valid
func (db *DB) GetJobs(limit, offset int, processIDs, statuses []string) ([]JobRecord, error) {
	baseQuery := `SELECT id, status, updated, process_id FROM jobs`
	whereClauses := []string{}
	args := []interface{}{}

	if len(processIDs) > 0 {
		placeholders := strings.Repeat("?,", len(processIDs)-1) + "?"
		whereClauses = append(whereClauses, fmt.Sprintf("process_id IN (%s)", placeholders))
		for _, pid := range processIDs {
			args = append(args, pid)
		}
	}

	if len(statuses) > 0 {
		placeholders := strings.Repeat("?,", len(statuses)-1) + "?"
		whereClauses = append(whereClauses, fmt.Sprintf("status IN (%s)", placeholders))
		for _, st := range statuses {
			args = append(args, st)
		}
	}

	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	query := baseQuery + ` ORDER BY updated DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	res := []JobRecord{}

	rows, err := db.Handle.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r JobRecord
		if err := rows.Scan(&r.JobID, &r.Status, &r.LastUpdate, &r.ProcessID); err != nil {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		res = append(res, r)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return res, nil
}
