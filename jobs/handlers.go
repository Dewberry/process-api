package jobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type RESTHandler struct {
	JobsCache   *JobsCache
	ProcessList *ProcessList
}

func NewRESTHander(processesDir string, maxCacheSize uint64) (*RESTHandler, error) {
	processList, err := LoadProcesses(processesDir)
	if err != nil {
		return nil, err
	}
	var jc JobsCache = JobsCache{MaxSizeBytes: uint64(maxCacheSize),
		CurrentSizeBytes: 0, Jobs: make(Jobs, 0), TrimThreshold: 0.80}
	return &RESTHandler{ProcessList: &processList, JobsCache: &jc}, nil
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// LandingPage godoc
// @Summary Landing Page
// @Description [LandingPage Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_landing_page)
// @Tags info
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router / [get]
func (rh *RESTHandler) LandingPage(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"title":       "process-api",
		"description": "ogc process api written in Golang for use with cloud service controllers to manage asynchronous requests",
	})
}

// Conformance godoc
// @Summary API Conformance List
// @Description [Conformance Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_conformance_classes)
// @Tags info
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{} "hello:["dolly"]"
// @Router /conformance [get]
func (rh *RESTHandler) Conformance(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string][]string{
		"conformsTo": {
			"http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/ogc-process-description",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/core",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/json"},
	})
}

// ProcessListHandler godoc
// @Summary List Available Processes
// @Description [Process List Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_process_list)
// @Tags processes
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /processes [get]
func (rh *RESTHandler) ProcessListHandler(c echo.Context) error {
	processList, err := rh.ProcessList.ListAll()
	if err != nil {
		return err
	}

	outputFormat := c.QueryParam("f")

	switch outputFormat {
	case "html":
		return c.Render(http.StatusOK, "processes", processList)
	case "json":
		return c.JSON(http.StatusOK, processList)
	case "":
		return c.JSON(http.StatusOK, processList)
	default:
		return c.JSON(http.StatusBadRequest, "valid format options are 'html' or 'json'. default (i.e. not specified) is json)")
	}

}

// ProcessDescribeHandler godoc
// @Summary Describe Process Information
// @Description [Process Description Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_process_description)
// @Tags processes
// @Param processID path string true "processID"
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /processes/{processID} [get]
func (rh *RESTHandler) ProcessDescribeHandler(c echo.Context) error {
	processID := c.Param("processID")
	p, err := rh.ProcessList.Get(processID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	description, err := p.Describe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}

	return c.JSON(http.StatusOK, description)
}

// @Summary Execute Process
// @Description [Execute Process Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_create_job)
// @Tags processes
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /processes/{processID}/execution [post]
func (rh *RESTHandler) Execution(c echo.Context) error {

	processID := c.Param("processID")

	if processID == "" {
		return c.JSON(http.StatusBadRequest, "'processID' parameter is required")
	}

	p, err := rh.ProcessList.Get(processID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, fmt.Sprintf("processID '%s' is not an available process", processID))
	}

	var params RunRequestBody
	err = c.Bind(&params)
	if err != nil {
		return c.JSON(http.StatusBadRequest, err.Error())
	}

	// Review this section
	if params.Inputs == nil {
		return c.JSON(http.StatusBadRequest, "'inputs' is required in the body of the request")
	}

	cmd, op, err := p.convInpsToCommand(params.Inputs)
	if err != nil {
		return c.JSON(http.StatusBadRequest, err.Error())
	}

	var j Job
	jobType := p.Info.JobControlOptions[0]
	jobID := uuid.New().String()

	for i := 0; i < len(cmd); i++ {
		cmd[i] = strings.Replace(cmd[i], "_jobID_", jobID, 1)
	}

	if p.Runtime.EntryPoint != "" {
		cmd = append([]string{p.Runtime.EntryPoint}, cmd...)
	}

	if jobType == "sync-execute" {
		j = &DockerJob{
			Ctx:         context.TODO(),
			UUID:        jobID,
			ProcessName: processID,
			Repository:  p.Runtime.Repository,
			EnvVars:     p.Runtime.EnvVars,
			ImgTag:      fmt.Sprintf("%s:%s", p.Runtime.Image, p.Runtime.Tag),
			Cmd:         cmd,
			Outputs:     op,
		}

	} else {
		runtime := p.Runtime.Provider.Type
		switch runtime {
		case "aws-batch":
			j = &AWSBatchJob{
				Ctx:         context.TODO(),
				UUID:        jobID,
				ProcessName: processID,
				ImgTag:      fmt.Sprintf("%s:%s", p.Runtime.Image, p.Runtime.Tag),
				Cmd:         cmd,
				Outputs:     op,
				JobDef:      p.Runtime.Provider.JobDefinition,
				JobQueue:    p.Runtime.Provider.JobQueue,
				JobName:     p.Runtime.Provider.Name,
			}
		default:
			return c.JSON(http.StatusBadRequest, fmt.Sprintf("unsupported type %s", jobType))
		}
	}

	// Add to cache
	rh.JobsCache.Add(j)

	// Send job to go routine
	err = j.Create()
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			fmt.Sprintf("submission errorr %s", err))
	}

	switch p.Info.JobControlOptions[0] {
	case "sync-execute":
		j.Run()
		outputs := j.JobOutputs()
		// prevent large caches for synchronous jobs
		j.ClearOutputs()
		return c.JSON(http.StatusAccepted, outputs)
	case "async-execute":
		go j.Run()
		output := map[string]string{"jobID": j.JobID(), "value": "created"}
		return c.JSON(http.StatusAccepted, output)
	default:
		return c.JSON(http.StatusBadRequest, "JobControlOptions not allowed")
	}
}

// @Summary Dismiss Job
// @Description [Dismss Job Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#ats_dismiss)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /jobs/{jobID} [delete]
func (rh *RESTHandler) JobDismissHandler(c echo.Context) error {
	jobID := c.Param("jobID")
	for _, job := range rh.JobsCache.Jobs {
		if job.JobID() == jobID {
			err := job.Kill()
			if err != nil {
				return c.JSON(http.StatusInternalServerError, err)
			}
			return c.JSON(http.StatusOK, fmt.Sprintf("job %s dismissed", jobID))
		}
	}
	return c.JSON(http.StatusGone, fmt.Sprintf("job %s not in the active jobs list", jobID))
}

// @Summary Job Status
// @Description [xxx Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_status_info)
// @Tags jobs
// @Info [Format YAML](http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/statusInfo.yaml)
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /jobs/{jobID} [get]
func (rh *RESTHandler) JobStatusHandler(c echo.Context) error {
	jobID := c.Param("jobID")
	for _, j := range rh.JobsCache.Jobs {
		if j.JobID() == jobID {
			output := map[string]interface{}{"jobID": jobID, "last_update": j.LastUpdate(), "status": j.CurrentStatus()}
			return c.JSON(http.StatusOK, output)
		}
	}
	output := map[string]string{"jobID": jobID, "status": "not found"}
	return c.JSON(http.StatusNotFound, output)
}

// @Summary Job Results
// @Description [Job Results Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /jobs/{jobID} [get]
func (rh *RESTHandler) JobResultsHandler(c echo.Context) error {
	jobID := c.Param("jobID")
	for _, j := range rh.JobsCache.Jobs {
		if j.JobID() == jobID {
			switch j.CurrentStatus() {
			case SUCCESSFUL:
				output := map[string]interface{}{
					"jobID":       jobID,
					"status":      j.CurrentStatus(),
					"last_update": j.LastUpdate(),
					"outputs":     j.JobOutputs(),
				}
				return c.JSON(http.StatusOK, output)

			case FAILED:
				output := map[string]interface{}{
					"jobID":       jobID,
					"status":      j.CurrentStatus(),
					"logs":        j.JobLogs(),
					"last_update": j.LastUpdate(),
				}
				return c.JSON(http.StatusOK, output)

			case DISMISSED:
				output := map[string]interface{}{
					"jobID":       jobID,
					"status":      j.CurrentStatus(),
					"logs":        j.JobLogs(),
					"last_update": j.LastUpdate(),
				}
				return c.JSON(http.StatusOK, output)
			default:
				output := map[string]string{"jobID": jobID, "value": "results not yet available"}
				return c.JSON(http.StatusProcessing, output)
			}

		}
	}
	output := map[string]string{"jobID": jobID, "value": "not found"}
	return c.JSON(http.StatusNotFound, output)
}

// @Summary Summary of all (cached) Jobs
// @Description [Job List Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /jobs [get]
func (rh *RESTHandler) JobsCacheHandler(c echo.Context) error {
	// includeErrorMessages := c.QueryParams().Get("include_error_messages")
	// if includeErrorMessages == "" {
	// 	return c.JSON(http.StatusOK, rh.JobsCache.ListJobs(false))
	// }

	// _, err := strconv.ParseBool(includeErrorMessages)
	// if err != nil {
	// 	return c.JSON(http.StatusBadRequest,
	// 		fmt.Sprintf("'include_error_messages' must be true or false, not %s", includeErrorMessages))
	// }

	jobsList := rh.JobsCache.ListJobs(false)

	outputFormat := c.QueryParam("f")

	switch outputFormat {
	case "html":
		return c.Render(http.StatusOK, "jobs", jobsList)
	case "json":
		return c.JSON(http.StatusOK, jobsList)
	case "":
		return c.JSON(http.StatusOK, jobsList)
	default:
		return c.JSON(http.StatusBadRequest, "valid format options are 'html' or 'json'. default (i.e. not specified) is json)")
	}

}
