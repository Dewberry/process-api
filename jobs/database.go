package jobs

import (
	"database/sql"
	"encoding/json"
	"errors"
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

	// Array of VARCHAR is represented as TEXT in SQLite. Client application has to handle conversion

	queryLogs := `
	CREATE TABLE IF NOT EXISTS logs (
		job_id TEXT PRIMARY KEY,
		api_logs TEXT,
		container_logs TEXT,
		FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
	);
	`

	_, err = db.Handle.Exec(queryLogs)
	if err != nil {
		log.Fatal(err)
	}
}

// Initialize the database
func InitDB(filepath string) *DB {
	h, err := sql.Open("sqlite3", filepath+"?mode=rwc")
	// it maybe a good idea to check here if the connections has write privilage https://stackoverflow.com/a/44707371/11428410 https://www.sqlite.org/c3ref/db_readonly.html
	// also maybe we should make db such that only go can write to it

	if err != nil {
		log.Fatalf("could not open %s Delete the existing database to start with a new datbase. Error: %s", filepath, err.Error())
	}

	if h == nil {
		log.Fatal("db nil")
	}

	db := DB{Handle: h}
	db.createTables()
	return &db
}

func (db *DB) addJob(jid string, status string, updated time.Time, mode string, host string, process_id string) error {
	query := `INSERT INTO jobs (id, status, updated, mode, host, process_id) VALUES (?, ?, ?, ?, ?, ?)`

	_, err := db.Handle.Exec(query, jid, status, updated, mode, host, process_id)
	if err != nil {
		return err
	}
	return nil
}

func (db *DB) updateJobRecord(jid string, status string, now time.Time) {
	query := `UPDATE jobs SET status = ?, updated = ? WHERE id = ?`
	_, err := db.Handle.Exec(query, status, now, jid)
	if err != nil {
		log.Error(err)
	}
}

func (db *DB) addLogs(jid string, apiLogs []string, containerLogs []string) {
	query := `
	INSERT INTO logs (job_id, api_logs, container_logs) VALUES (?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET
			api_logs = excluded.api_logs,
			container_logs = excluded.container_logs;
	`

	// Convert APILogs and ContainerLogs from []string to JSON string
	apiLogsJSON, err := json.Marshal(apiLogs)
	if err != nil {
		log.Error(err)
	}
	containerLogsJSON, err := json.Marshal(containerLogs)
	if err != nil {
		log.Error(err)
	}

	_, err = db.Handle.Exec(query, jid, string(apiLogsJSON), string(containerLogsJSON))
	if err != nil {
		log.Error(err)
	}
}

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

func (db *DB) GetJobs(limit int, offset int) ([]JobRecord, error) {
	query := `SELECT id, status, updated, process_id FROM jobs ORDER BY updated DESC LIMIT ? OFFSET ?`

	res := []JobRecord{}

	rows, err := db.Handle.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r JobRecord
		var updated string
		if err := rows.Scan(&r.JobID, &r.Status, &updated, &r.ProcessID); err != nil {
			return nil, err
		}
		r.LastUpdate, err = time.Parse(time.RFC3339, updated)
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

func (db *DB) GetLogs(jid string) (JobLogs, error) {
	query := `SELECT api_logs, container_logs FROM logs WHERE job_id = ?`

	logs := JobLogs{}
	// These will hold the JSON strings from the database
	var apiLogsJSON, containerLogsJSON string

	row := db.Handle.QueryRow(query, jid)
	err := row.Scan(&apiLogsJSON, &containerLogsJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return JobLogs{}, errors.New("not found")
		} else {
			return JobLogs{}, err
		}
	}

	// Convert JSON strings back into arrays of strings
	err = json.Unmarshal([]byte(apiLogsJSON), &logs.APILogs)
	if err != nil {
		return JobLogs{}, errors.New("error decoding api logs")
	}
	err = json.Unmarshal([]byte(containerLogsJSON), &logs.ContainerLogs)
	if err != nil {
		return JobLogs{}, errors.New("error decoding container logs")
	}

	return logs, nil
}
