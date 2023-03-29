package jobs

import (
	"app/controllers"
	"context"
	"fmt"
	"os"
	"time"
	"unsafe"
)

type DockerJob struct {
	Ctx         context.Context
	CtxCancel   context.CancelFunc
	UUID        string `json:"jobID"`
	ContainerID string
	Repository  string `json:"repository"` // for local repositories leave empty
	ImgTag      string `json:"imageAndTag"`
	ProcessName string `json:"processID"`
	EntryPoint  string `json:"entrypoint"`
	EnvVars     []string
	Cmd         []string `json:"commandOverride"`
	UpdateTime  time.Time
	Status      string `json:"status"`
	MessageList []string
	LogInfo     string
	Links       []Link      `json:"links"`
	Outputs     interface{} `json:"outputs"`
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

func (j *DockerJob) IMGTAG() string {
	return j.ImgTag
}

func (j *DockerJob) JobLogs() string {
	return j.LogInfo
}

func (j *DockerJob) JobOutputs() interface{} {
	return j.Outputs
}

func (j *DockerJob) ClearOutputs() {
	j.Outputs = []interface{}{}

}

func (j *DockerJob) Messages(includeErrors bool) []string {
	return j.MessageList
}

func (j *DockerJob) NewMessage(m string) {
	j.MessageList = append(j.MessageList, m)
}

func (j *DockerJob) NewErrorMessage(m string) {
	j.MessageList = append(j.MessageList, m)
	j.NewStatusUpdate(FAILED)
	j.CtxCancel()
}

func (j *DockerJob) LastUpdate() time.Time {
	return j.UpdateTime
}

func (j *DockerJob) NewStatusUpdate(s string) {
	j.Status = s
	j.UpdateTime = time.Now()
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
		return j.Ctx == jj.Ctx
	default:
		return false
	}
}

func (j *DockerJob) Create() error {
	ctx, cancelFunc := context.WithCancel(j.Ctx)
	j.Ctx = ctx
	j.CtxCancel = cancelFunc

	c, err := controllers.NewDockerController()
	if err != nil {
		j.NewErrorMessage(err.Error())
		return err
	}

	// verify command in body
	if j.Cmd == nil {
		j.NewErrorMessage(err.Error())
		return err
	}

	// pull image
	if j.Repository != "" {
		err = c.EnsureImage(ctx, j.ImgTag, false)
		if err != nil {
			j.NewErrorMessage(err.Error())
			return err
		}
	}

	j.NewStatusUpdate(ACCEPTED)
	return nil
}

func (j *DockerJob) Run() {
	c, err := controllers.NewDockerController()
	if err != nil {
		j.NewErrorMessage(err.Error())
		return
	}

	// get environment variables
	envVars := map[string]string{}
	for _, eVar := range j.EnvVars {
		envVars[eVar] = os.Getenv(eVar)
	}

	// start container
	j.NewStatusUpdate(RUNNING)
	containerID, err := c.ContainerRun(j.Ctx, j.ImgTag, j.Cmd, []controllers.VolumeMount{}, envVars)
	if err != nil {
		j.NewErrorMessage(err.Error())
		return
	}

	j.ContainerID = containerID

	// wait for process to finish
	statusCode, err1 := c.ContainerWait(j.Ctx, j.ContainerID)
	logs, err2 := c.ContainerLog(j.Ctx, j.ContainerID)
	if err1 != nil {
		j.NewErrorMessage(err1.Error())
	} else if statusCode != 0 {
		if err2 == nil {
			var data string
			for _, v := range logs {
				data += string(v.(byte))
			}
			j.LogInfo = data
		}
		j.NewErrorMessage(fmt.Sprintf("container exit code: %d", statusCode))
	} else if statusCode == 0 {
		var data string
		for _, v := range logs {
			data += string(v.(byte))
		}

		j.Outputs = data
		j.NewStatusUpdate(SUCCESSFUL)
	}

	// clean up the finished job
	err = c.ContainerRemove(j.Ctx, j.ContainerID)
	if err != nil {
		j.NewErrorMessage(err.Error())
	}

	j.CtxCancel()
}

// kill local container
func (j *DockerJob) Kill() error {
	c, err := controllers.NewDockerController()
	if err != nil {
		j.NewMessage(err.Error())
	}

	err = c.ContainerKillAndRemove(j.Ctx, j.ContainerID, "KILL")
	if err != nil {
		j.NewErrorMessage(err.Error())
	}

	j.NewStatusUpdate(DISMISSED)
	j.CtxCancel()
	return nil
}

func (j *DockerJob) GetSizeinCache() int {
	cmdData := int(unsafe.Sizeof(j.Cmd))
	for _, item := range j.Cmd {
		cmdData += len(item)
	}

	messageData := int(unsafe.Sizeof(j.MessageList))
	for _, item := range j.MessageList {
		messageData += len(item)
	}

	// not calculated appropriately, add method...
	linkData := int(unsafe.Sizeof(j.Links))

	totalMemory := cmdData + messageData + linkData +
		int(unsafe.Sizeof(j.Ctx)) +
		int(unsafe.Sizeof(j.CtxCancel)) +
		int(unsafe.Sizeof(j.UUID)) + len(j.UUID) +
		int(unsafe.Sizeof(j.ContainerID)) + len(j.ContainerID) +
		int(unsafe.Sizeof(j.ImgTag)) + len(j.ImgTag) +
		int(unsafe.Sizeof(j.UpdateTime)) +
		int(unsafe.Sizeof(j.Status)) +
		int(unsafe.Sizeof(j.LogInfo)) + len(j.LogInfo)

	return totalMemory
}
