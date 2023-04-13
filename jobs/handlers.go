package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/template"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

type RESTHandler struct {
	JobsCache   *JobsCache
	ProcessList *ProcessList
	S3Svc       *s3.S3
}

func NewRESTHander(processesDir string, maxCacheSize uint64) (*RESTHandler, error) {
	processList, err := LoadProcesses(processesDir)
	if err != nil {
		return nil, err
	}

	var jc JobsCache = JobsCache{MaxSizeBytes: uint64(maxCacheSize),
		CurrentSizeBytes: 0, TrimThreshold: 0.80}

	// load from previous snapshot if it exist
	err = jc.LoadCacheFromFile()
	if err != nil {
		fmt.Printf("Error loading snapshot: %s \n", err.Error())
		jc.Jobs = make(map[string]*Job)
	}

	// Set up a session with AWS credentials and region
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := s3.New(sess)

	return &RESTHandler{ProcessList: &processList, JobsCache: &jc, S3Svc: svc}, nil
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

	outputFormat := c.QueryParam("f")

	p, err := rh.ProcessList.Get(processID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, err.Error())
	}

	description, err := p.Describe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	switch outputFormat {
	case "html":
		return c.Render(http.StatusOK, "process", description)
	case "json":
		return c.JSON(http.StatusOK, description)
	case "":
		return c.JSON(http.StatusOK, description)
	default:
		return c.JSON(http.StatusBadRequest, "valid format options are 'html' or 'json'. default (i.e. not specified) is json)")
	}

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

	log.Debug("processID", processID)
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

	err = p.verifyInputs(params.Inputs)
	if err != nil {
		return c.JSON(http.StatusBadRequest, err.Error())
	}

	var j Job
	jobType := p.Info.JobControlOptions[0]
	jobID := uuid.New().String()

	params.Inputs["jobID"] = jobID
	params.Inputs["resultsDir"] = os.Getenv("S3_RESULTS_DIR")
	params.Inputs["expDays"] = os.Getenv("EXPIRY_DAYS")
	jsonParams, err := json.Marshal(params.Inputs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	var cmd []string
	if p.Runtime.Command == nil {
		cmd = []string{string(jsonParams)}
	} else {
		cmd = append(p.Runtime.Command, string(jsonParams))
	}

	if jobType == "sync-execute" {
		j = &DockerJob{
			ctx:         context.TODO(),
			UUID:        jobID,
			ProcessName: processID,
			Repository:  p.Runtime.Repository,
			EnvVars:     p.Runtime.EnvVars,
			ImgTag:      fmt.Sprintf("%s:%s", p.Runtime.Image, p.Runtime.Tag),
			Cmd:         cmd,
		}

	} else {
		runtime := p.Runtime.Provider.Type
		switch runtime {
		case "aws-batch":
			j = &AWSBatchJob{
				ctx:         context.TODO(),
				UUID:        jobID,
				ProcessName: processID,
				ImgTag:      fmt.Sprintf("%s:%s", p.Runtime.Image, p.Runtime.Tag),
				Cmd:         cmd,
				JobDef:      p.Runtime.Provider.JobDefinition,
				JobQueue:    p.Runtime.Provider.JobQueue,
				JobName:     p.Runtime.Provider.Name,
			}
		default:
			return c.JSON(http.StatusBadRequest, fmt.Sprintf("unsupported type %s", jobType))
		}
	}

	// Add to cache
	rh.JobsCache.Add(&j)

	// Create job
	err = j.Create()
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			fmt.Sprintf("submission errorr %s", err.Error()))
	}

	var outputs interface{}

	switch p.Info.JobControlOptions[0] {
	case "sync-execute":
		j.Run()

		if j.CurrentStatus() == "successful" {
			if p.Outputs != nil {
				outputs, err = FetchResults(rh.S3Svc, j.JobID())
				if err != nil {
					return c.JSON(http.StatusInternalServerError, err.Error())
				}
			}
			resp := map[string]interface{}{"jobID": j.JobID(), "outputs": outputs}
			return c.JSON(http.StatusOK, resp)
		} else {
			resp := map[string]interface{}{"processID": j.ProcessID(), "type": "process", "jobID": jobID, "status": 0, "message": "Job Failed. Call logs route for details."}
			return c.JSON(http.StatusInternalServerError, resp)
		}
	case "async-execute":
		go j.Run()
		resp := map[string]interface{}{"processID": j.ProcessID(), "type": "process", "jobID": jobID, "status": "accepted"}
		return c.JSON(http.StatusCreated, resp)
	default:
		resp := map[string]interface{}{"processID": j.ProcessID(), "type": "process", "jobID": jobID, "status": 0, "message": "incorrect controller option defined in process configuration"}
		return c.JSON(http.StatusInternalServerError, resp)
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
	if job, ok := rh.JobsCache.Jobs[jobID]; ok {
		err := (*job).Kill()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, fmt.Sprintf("job %s dismissed", jobID))
	}
	return c.JSON(http.StatusNotFound, fmt.Sprintf("job %s not in the active jobs list", jobID))
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
	if job, ok := rh.JobsCache.Jobs[jobID]; ok {
		resp := JobStatus{
			ProcessID:  (*job).ProcessID(),
			JobID:      (*job).JobID(),
			LastUpdate: (*job).LastUpdate(),
			Status:     (*job).CurrentStatus(),
		}
		return c.JSON(http.StatusOK, resp)
	}
	output := map[string]interface{}{"type": "process", "jobID": jobID, "status": 0, "message": "jobID not found"}
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
	if job, ok := rh.JobsCache.Jobs[jobID]; ok {
		switch (*job).CurrentStatus() {
		case SUCCESSFUL:
			outputs, err := FetchResults(rh.S3Svc, (*job).JobID())
			if err != nil {
				if err.Error() == "not found" {
					output := map[string]interface{}{"type": "process", "jobID": jobID, "status": (*job).CurrentStatus(), "message": err.Error()}
					return c.JSON(http.StatusNotFound, output)
				}
				return c.JSON(http.StatusInternalServerError, err.Error())
			}
			output := map[string]interface{}{
				"type":    "process",
				"jobID":   jobID,
				"status":  (*job).CurrentStatus(),
				"updated": (*job).LastUpdate(),
				"outputs": outputs,
			}
			return c.JSON(http.StatusOK, output)

		case FAILED, DISMISSED:
			output := map[string]interface{}{
				"type":    "process",
				"jobID":   jobID,
				"status":  (*job).CurrentStatus(),
				"message": "Job Failed or Dismissed. Call logs route for details.",
				"updated": (*job).LastUpdate(),
			}
			return c.JSON(http.StatusOK, output)

		default:
			output := map[string]interface{}{"type": "process", "jobID": jobID, "status": (*job).CurrentStatus(), "message": "results not ready", "updated": (*job).LastUpdate()}
			return c.JSON(http.StatusNotFound, output)
		}

	}
	output := map[string]interface{}{"type": "process", "jobID": jobID, "status": 0, "message": "jobID not found"}
	return c.JSON(http.StatusNotFound, output)
}

// @Summary Job Logs
// @Description
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /jobs/{jobID}/logs [get]
func (rh *RESTHandler) JobLogsHandler(c echo.Context) error {
	jobID := c.Param("jobID")
	outputFormat := c.QueryParam("f")

	if job, ok := rh.JobsCache.Jobs[jobID]; ok {
		logs, err := (*job).Logs()
		if err != nil {
			if err.Error() == "not found" {
				output := map[string]interface{}{"type": "process", "jobID": jobID, "status": (*job).CurrentStatus(), "message": err.Error()}
				return c.JSON(http.StatusNotFound, output)
			}
			output := map[string]interface{}{"type": "process", "jobID": jobID, "status": 0, "message": "Error while fetching logs: " + err.Error()}
			return c.JSON(http.StatusInternalServerError, output)
		}

		switch outputFormat {
		case "html":
			return c.Render(http.StatusOK, "logs", logs)
		case "json":
			return c.JSON(http.StatusOK, logs)
		case "":
			return c.JSON(http.StatusOK, logs)
		default:
			return c.JSON(http.StatusBadRequest, "valid format options are 'html' or 'json'. default (i.e. not specified) is json)")
		}
	}

	output := JobLog{ContainerLog: []string{"jobID not found, if the job was just submitted logs may not be available"}}
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

	jobsList := rh.JobsCache.ListJobs()

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
