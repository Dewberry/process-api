package jobs

import (
	"fmt"
	"time"
)

type StatusMessage struct {
	JobID      string    `json:"jobID"`
	Status     string    `json:"status"`
	LastUpdate time.Time `json:"updated"`
}

type ResultsMessage struct {
	JobID      string      `json:"jobID"`
	Results    interface{} `json:"outputs"`
	LastUpdate time.Time   `json:"updated"`
}

type MessageQueue struct {
	StatusTopic  chan StatusMessage
	ResultsTopic chan ResultsMessage
}

func ProcessStatusMessage(sm StatusMessage) {
	fmt.Printf("Processed status for job %s: %s\n", sm.JobID, sm.Status)
}

func ProcessResultsMessage(rm ResultsMessage) {
	fmt.Printf("Processed result for job %s: %s\n", rm.JobID, rm.Results)
}
