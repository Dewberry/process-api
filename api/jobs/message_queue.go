package jobs

import (
	"time"
)

type StatusMessage struct {
	Job        *Job
	Status     string    `json:"status"`
	LastUpdate time.Time `json:"updated"`
}

type ResultsMessage struct {
	JobID   string      `json:"jobID"`
	Results interface{} `json:"outputs"`
}

type MessageQueue struct {
	StatusChan chan StatusMessage
	JobDone    chan Job
}

// Job should not be a docker job
func ProcessStatusMessageUpdate(sm StatusMessage) {

	// Multiple calls should not trigger multiple close or metadata routines
	// A better way to achieve this would be through locks.
	switch (*sm.Job).CurrentStatus() {
	case SUCCESSFUL, DISMISSED, FAILED:
		return
	}
	(*sm.Job).NewStatusUpdate(sm.Status, sm.LastUpdate)

	switch sm.Status {
	case SUCCESSFUL:
		go (*sm.Job).WriteMetaData()
		fallthrough
	case DISMISSED, FAILED:
		// swap the order of following if results are posted/written by the container, and run close as a coroutine
		(*sm.Job).Close()
		(*sm.Job).RunFinished()
	}
}
