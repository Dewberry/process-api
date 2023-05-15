package jobs

import (
	"app/utils"
	"encoding/json"
	"fmt"
	"time"
)

// Define a metaData object
type metaData struct {
	Context                  string `json:"@context"`
	JobID                    string `json:"apiJobId"`
	User                     string
	ProcessID                string `json:"apiProcessId"`
	ProcessVersion           string
	ImageDigest              string `json:"imageDigest"`
	ImageURI                 string `json:"imageURI"`
	ComputeEnvironmentURI    string // ARN
	ComputeEnvironmentDigest string // required for reproducibility, will need to be custom implemented
	Commands                 []string
	TimeCompleted            time.Time
}

func (j *AWSBatchJob) WriteMeta(imgDgst string) {

	if j.MetaDataLocation == "" {
		return
	}

	md := metaData{
		Context:       "http://schema.org/",
		JobID:         j.UUID,
		ProcessID:     j.ProcessID(),
		ImageDigest:   imgDgst,
		TimeCompleted: j.UpdateTime,
		Commands:      j.Cmd,
	}

	jsonBytes, err := json.Marshal(md)
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	utils.WriteToS3(jsonBytes, j.MetaDataLocation, &j.APILogs, "application/json", 0)

	return
}
