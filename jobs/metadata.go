package jobs

import (
	"app/controllers"
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
	ImageURI                 string `json:"imageURI"`
	ImageDigest              string `json:"imageDigest"`
	ComputeEnvironmentURI    string // ARN
	ComputeEnvironmentDigest string // required for reproducibility, will need to be custom implemented
	Commands                 []string
	TimeCompleted            time.Time
}

func (j *AWSBatchJob) WriteMeta(cAws *controllers.AWSBatchController) {

	if j.MetaDataLocation == "" {
		return
	}

	imgURI, err := cAws.GetImageURI(j.JobDef)
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	cD, err := controllers.NewDockerController()
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	imgDgst, err := cD.GetImageDigest(imgURI)
	if err != nil {
		j.NewMessage(fmt.Sprintf("error writing metadata: %s", err.Error()))
		return
	}

	md := metaData{
		Context:       "http://schema.org/",
		JobID:         j.UUID,
		ProcessID:     j.ProcessID(),
		ImageURI:      imgURI,
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
