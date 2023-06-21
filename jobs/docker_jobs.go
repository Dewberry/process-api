package jobs

import (
	"app/controllers"
	"context"
	"fmt"
	"os"
	"time"
)

type DockerJob struct {
	ctx           context.Context
	ctxCancel     context.CancelFunc
	UUID          string `json:"jobID"`
	ContainerID   string
	Image         string `json:"image"`
	ProcessName   string `json:"processID"`
	EnvVars       []string
	Cmd           []string `json:"commandOverride"`
	UpdateTime    time.Time
	Status        string `json:"status"`
	apiLogs       []string
	containerLogs []string
	Resources
	DB *DB
}

func (j *DockerJob) JobID() string {
	return j.UUID
}

func (j *DockerJob) ProcessID() string {
	return j.ProcessName
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

	logs.ContainerLogs = j.containerLogs
	logs.APILogs = j.apiLogs
	return logs, nil
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
		go j.DB.addLogs(j.UUID, j.apiLogs, j.containerLogs)
	}
}

func (j *DockerJob) LastUpdate() time.Time {
	return j.UpdateTime
}

func (j *DockerJob) NewStatusUpdate(s string) {
	j.Status = s
	now := time.Now()
	j.UpdateTime = now
	j.DB.updateJobStatus(j.UUID, FAILED, now)
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
		return err
	}

	// verify command in body
	if j.Cmd == nil {
		return err
	}

	// pull image
	if j.Image != "" {
		err = c.EnsureImage(ctx, j.Image, false)
		if err != nil {
			return err
		}
	}

	// At this point job is ready to be added to database
	err = j.DB.addJob(j.UUID, "accepted", time.Now(), "", "local", j.ProcessName)
	if err != nil {
		return err
	}

	j.NewStatusUpdate(ACCEPTED)
	return nil
}

func (j *DockerJob) Run() {
	c, err := controllers.NewDockerController()
	if err != nil {
		j.HandleError(err.Error())
		return
	}

	// get environment variables
	envVars := map[string]string{}
	for _, eVar := range j.EnvVars {
		envVars[eVar] = os.Getenv(eVar)
	}

	resources := controllers.DockerResources{}
	resources.NanoCPUs = int64(j.Resources.CPUs * 1e9)         // Docker controller needs cpu in nano ints
	resources.Memory = int64(j.Resources.Memory * 1024 * 1024) // Docker controller needs memory in bytes

	// start container
	j.NewStatusUpdate(RUNNING)
	containerID, err := c.ContainerRun(j.ctx, j.Image, j.Cmd, []controllers.VolumeMount{}, envVars, resources)
	if err != nil {
		j.HandleError(err.Error())
		return
	}

	j.ContainerID = containerID

	// wait for process to finish
	statusCode, errWait := c.ContainerWait(j.ctx, j.ContainerID)
	// todo: get logs while container running so that logs or running containers is visible by users
	containerLogs, errLog := c.ContainerLog(j.ctx, j.ContainerID)
	j.containerLogs = containerLogs

	// If there are error messages remove container before cancelling context inside Handle Error
	for _, err := range []error{errWait, errLog} {
		if err != nil {
			errRem := c.ContainerRemove(j.ctx, j.ContainerID)
			if errRem != nil {
				j.HandleError(err.Error() + " " + errRem.Error())
				return
			}
			j.HandleError(err.Error())
			return
		}
	}

	if statusCode != 0 {
		errRem := c.ContainerRemove(j.ctx, j.ContainerID)
		if errRem != nil {
			j.HandleError(fmt.Sprintf("container exit code: %d", statusCode) + " " + errRem.Error())
			return
		}
		j.HandleError(fmt.Sprintf("container exit code: %d", statusCode))
		return
	} else if statusCode == 0 {
		j.NewStatusUpdate(SUCCESSFUL)
	}

	// clean up the finished job
	err = c.ContainerRemove(j.ctx, j.ContainerID)
	if err != nil {
		j.HandleError(err.Error())
		return
	}

	go j.DB.addLogs(j.UUID, j.apiLogs, j.containerLogs)
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
		j.NewMessage(err.Error())
	}

	err = c.ContainerKillAndRemove(j.ctx, j.ContainerID, "KILL")
	if err != nil {
		j.HandleError(err.Error())
	}

	j.NewStatusUpdate(DISMISSED)
	go j.DB.addLogs(j.UUID, j.apiLogs, j.containerLogs)
	j.ctxCancel()
	return nil
}
