package jobs

import (
	"app/controllers"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
)

type DockerJob struct {
	ctx            context.Context
	ctxCancel      context.CancelFunc
	UUID           string `json:"jobID"`
	ContainerID    string
	Image          string `json:"image"`
	ProcessName    string `json:"processID"`
	ProcessVersion string `json:"processVersion"`
	EnvVars        []string
	Cmd            []string `json:"commandOverride"`
	UpdateTime     time.Time
	Status         string `json:"status"`
	apiLogs        []string
	containerLogs  []string
	results        map[string]interface{}
	Resources
	DB       *DB
	MinioSvc *s3.S3
}

func (j *DockerJob) JobID() string {
	return j.UUID
}

func (j *DockerJob) ProcessID() string {
	return j.ProcessName
}

func (j *DockerJob) ProcessVersionID() string {
	return j.ProcessVersion
}

func (j *DockerJob) CMD() []string {
	return j.Cmd
}

func (j *DockerJob) IMAGE() string {
	return j.Image
}

// Return current logs of the job
func (j *DockerJob) Logs() (JobLogs, error) {
	var logs JobLogs

	logs.JobID = j.UUID
	logs.ProcessID = j.ProcessName
	logs.ContainerLogs = j.containerLogs
	logs.APILogs = j.apiLogs
	return logs, nil
}

// stripResultsFromLog convenience function
func StripResultsFromLog(containerLogs []string, jid string) (map[string]interface{}, error) {
	lastLogIdx := len(containerLogs) - 1
	if lastLogIdx < 0 {
		return nil, fmt.Errorf("no contnainer logs available")
	}

	lastLog := containerLogs[lastLogIdx]
	lastLog = strings.ReplaceAll(lastLog, "'", "\"")

	// fmt.Println("containerLogs....", containerLogs)

	var data map[string]interface{}
	err := json.Unmarshal([]byte(lastLog), &data)
	if err != nil {
		return nil, fmt.Errorf(`unable to parse results, expected {"plugin_results": {....}}, found : %s`, err.Error())
	}

	pluginResults, ok := data["plugin_results"]
	if !ok {
		return nil, fmt.Errorf("'plugin_results' key not found")
	}

	apiOutputResults := map[string]interface{}{
		"jobID":   jid,
		"results": pluginResults,
	}

	return apiOutputResults, nil
}

func (j *DockerJob) Results() (map[string]interface{}, error) {

	results, err := StripResultsFromLog(j.containerLogs, j.JobID())
	if err != nil {
		return nil, err
	}
	return results, nil

}

func (j *DockerJob) Messages(includeErrors bool) []string {
	return j.apiLogs
}

func (j *DockerJob) NewMessage(m string) {
	j.apiLogs = append(j.apiLogs, m)
}

// Append error to apiLogs, cancelCtx, update Status, and time, write logs to database
func (j *DockerJob) HandleError(m string) {
	j.apiLogs = append(j.apiLogs, m)
	j.ctxCancel()
	if j.Status != DISMISSED { // if job dismissed then the error is because of dismissing job
		j.NewStatusUpdate(FAILED)
		go j.DB.upsertLogs(j.UUID, j.ProcessID(), j.apiLogs, j.containerLogs)
	}
	go j.DB.upsertLogs(j.UUID, j.ProcessID(), j.apiLogs, j.containerLogs)
}

func (j *DockerJob) LastUpdate() time.Time {
	return j.UpdateTime
}

func (j *DockerJob) NewStatusUpdate(s string) {
	j.Status = s
	now := time.Now()
	j.UpdateTime = now
	j.DB.updateJobRecord(j.UUID, s, now)
}

func (j *DockerJob) CurrentStatus() string {
	return j.Status
}

func (j *DockerJob) ProviderID() string {
	return j.ContainerID
}

func (j *DockerJob) Equals(job Job) bool {
	switch jj := job.(type) {
	case *DockerJob:
		return j.ctx == jj.ctx
	default:
		return false
	}
}

func (j *DockerJob) Create() error {
	ctx, cancelFunc := context.WithCancel(context.TODO())
	j.ctx = ctx
	j.ctxCancel = cancelFunc

	c, err := controllers.NewDockerController()
	if err != nil {
		j.ctxCancel()
		return fmt.Errorf("error creating NewDockerController %s ", err.Error())
	}

	// verify command in body
	if j.Cmd == nil {
		fmt.Println("CMD")
		j.ctxCancel()
		return fmt.Errorf("unable to execute docker_job CMD %s ", err.Error())
	}

	// TODO: TURN ON
	_ = c
	// pull image
	// if j.Image != "" {
	// 	err = c.EnsureImage(ctx, j.Image, false)
	// 	if err != nil {
	// 		j.ctxCancel()
	// 		return fmt.Errorf("unable to EnsureImage avaailable, comment this check for offline dev.")
	// 	}
	// }

	// At this point job is ready to be added to database
	err = j.DB.addJob(j.UUID, "accepted", time.Now(), "", "local", j.ProcessName)
	if err != nil {
		fmt.Println("j.DB.addJob")
		j.ctxCancel()
		return err
	}

	j.NewStatusUpdate(ACCEPTED)
	return nil
}

func (j *DockerJob) Run() {
	c, err := controllers.NewDockerController()
	if err != nil {
		j.NewMessage("failed creating NewDockerController")
		j.HandleError(err.Error())
		return
	}

	// get environment variables
	envVars := map[string]string{}
	for _, eVar := range j.EnvVars {
		envVars[eVar] = os.Getenv(eVar)
	}

	j.NewMessage(fmt.Sprintf("Registered %v env vars", len(envVars)))
	resources := controllers.DockerResources{}
	resources.NanoCPUs = int64(j.Resources.CPUs * 1e9)         // Docker controller needs cpu in nano ints
	resources.Memory = int64(j.Resources.Memory * 1024 * 1024) // Docker controller needs memory in bytes

	// start container
	j.NewStatusUpdate(RUNNING)
	containerID, err := c.ContainerRun(j.ctx, j.Image, j.Cmd, []controllers.VolumeMount{}, envVars, resources)
	if err != nil {
		j.NewMessage("failed to run container")
		j.HandleError(err.Error())
		return
	}

	j.ContainerID = containerID
	j.NewMessage(fmt.Sprintf("ContainerID = %v", containerID))

	// wait for process to finish
	statusCode, errWait := c.ContainerWait(j.ctx, j.ContainerID)

	// todo: get logs while container running so that logs or running containers is visible by users this would only be needed when docker jobs can also be async
	containerLogs, errLog := c.ContainerLog(j.ctx, j.ContainerID)
	if err != nil {
		j.NewMessage("failed fetching conttainer logs")
		j.HandleError(err.Error())
		return
	}
	j.containerLogs = containerLogs

	// If there are error messages remove container before cancelling context inside Handle Error
	for _, err := range []error{errWait, errLog} {
		if err != nil {
			errRem := c.ContainerRemove(j.ctx, j.ContainerID)
			if errRem != nil {
				j.NewMessage("failed removing container")
				j.HandleError(err.Error() + " " + errRem.Error())
				return
			}
			j.NewMessage("failed wating for container")
			j.HandleError(err.Error())
			return
		}
	}

	if statusCode != 0 {
		errRem := c.ContainerRemove(j.ctx, j.ContainerID)
		if errRem != nil {
			j.HandleError(fmt.Sprintf("container failure, exit code: %d", statusCode) + " " + errRem.Error())
			return
		}
		j.HandleError(fmt.Sprintf("container failure, exit code: %d", statusCode))
		return
	}

	j.WriteMeta(c)

	// clean up the finished job
	err = c.ContainerRemove(j.ctx, j.ContainerID)
	if err != nil {
		j.NewMessage("failed removing for container")
		j.HandleError(err.Error())
		return
	}

	results, err := j.Results()
	if err != nil {
		j.NewMessage("unable to fetch results")
		j.HandleError(err.Error())
		return
	}
	j.results = results
	j.NewStatusUpdate(SUCCESSFUL)
	j.NewMessage("process completed successfully.")

	go j.DB.upsertLogs(j.UUID, j.ProcessID(), j.apiLogs, j.containerLogs)
	j.ctxCancel()
}

// kill local container
func (j *DockerJob) Kill() error {
	switch j.CurrentStatus() {
	case SUCCESSFUL, FAILED, DISMISSED:
		// if these jobs have been loaded from previous snapshot they would not have context etc
		return fmt.Errorf("can't call delete on an already completed, failed, or dismissed job")
	}

	j.NewMessage("`received dismiss signal`")

	c, err := controllers.NewDockerController()
	if err != nil {
		j.HandleError(err.Error())
	}

	err = c.ContainerKillAndRemove(j.ctx, j.ContainerID, "KILL")
	if err != nil {
		j.HandleError(err.Error())
	}

	j.NewStatusUpdate(DISMISSED)
	go j.DB.upsertLogs(j.UUID, j.ProcessID(), j.apiLogs, j.containerLogs)
	j.ctxCancel()
	return nil
}
