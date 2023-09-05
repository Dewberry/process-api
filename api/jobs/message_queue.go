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
	// to do: acquire lock here so that subsequent calls can not update job status or trigger methods on jobs
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
