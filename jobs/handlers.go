package jobs

import (
	"app/utils"
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

// base error
type errResponse struct {
	HTTPStatus int    `json:"-"`
	Message    string `json:"message"`
}

// StatusText returns a text for the HTTP status code. It returns the empty
// string if the code is unknown.
func (er errResponse) GetHTTPStatusText() string {
	return http.StatusText(er.HTTPStatus)
}

var validFormats = []string{"", "json", "html"}

// Check if format query parameter is allowed.
func validateFormat(c echo.Context) error {
	outputFormat := c.QueryParam("f")
	if !utils.StringInSlice(outputFormat, validFormats) {
		return c.JSON(http.StatusBadRequest, errResponse{Message: "Invalid option for query parameter 'f'. Valid options are 'html' or 'json'. default (i.e. not specified) is json)"})
	}
	return nil
}

// Prepare and return response based on query parameter.
// Assumes query parameter is valid.
func prepareResponse(c echo.Context, httpStatus int, renderName string, output interface{}) error {
	outputFormat := c.QueryParam("f")
	switch outputFormat {
	case "html":
		return c.Render(httpStatus, renderName, output)
	case "json", "":
		return c.JSON(httpStatus, output)
	}
	return nil
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
		log.Errorf("Error loading snapshot: %s \n", err.Error())
		log.Info("Starting with clean database.")
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
	err := validateFormat(c)
	if err != nil {
		return err
	}

	output := map[string]string{
		"title":       "process-api",
		"description": "ogc process api written in Golang for use with cloud service controllers to manage asynchronous requests",
	}
	return prepareResponse(c, http.StatusOK, "landing", output)
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
	err := validateFormat(c)
	if err != nil {
		return err
	}

	output := map[string][]string{
		"conformsTo": {
			"http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/ogc-process-description",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/core",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/json"},
	}

	return prepareResponse(c, http.StatusOK, "conformance", output)
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
	err := validateFormat(c)
	if err != nil {
		return err
	}

	processList, err := rh.ProcessList.ListAll()
	if err != nil {
		return prepareResponse(c, http.StatusInternalServerError, "error", errResponse{Message: err.Error(), HTTPStatus: http.StatusInternalServerError})
	}

	return prepareResponse(c, http.StatusOK, "processes", processList)
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

	err := validateFormat(c)
	if err != nil {
		return err
	}

	p, err := rh.ProcessList.Get(processID)
	if err != nil {
		return prepareResponse(c, http.StatusBadRequest, "error", errResponse{Message: err.Error(), HTTPStatus: http.StatusBadRequest})
	}

	description, err := p.Describe()
	if err != nil {
		return prepareResponse(c, http.StatusInternalServerError, "error", errResponse{Message: err.Error(), HTTPStatus: http.StatusInternalServerError})
	}
	return prepareResponse(c, http.StatusOK, "process", description)
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
		return c.JSON(http.StatusBadRequest, errResponse{Message: "'processID' parameter is required"})
	}

	p, err := rh.ProcessList.Get(processID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errResponse{Message: "'processID' parameter is required"})
	}

	var params RunRequestBody
	err = c.Bind(&params)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errResponse{Message: err.Error()})
	}

	// Review this section
	if params.Inputs == nil {
		return c.JSON(http.StatusBadRequest, errResponse{Message: "'inputs' is required in the body of the request"})
	}

	err = p.verifyInputs(params.Inputs)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errResponse{Message: err.Error()})
	}

	var j Job
	jobType := p.Info.JobControlOptions[0]
	jobID := uuid.New().String()

	params.Inputs["jobID"] = jobID
	params.Inputs["resultsDir"] = os.Getenv("S3_RESULTS_DIR")
	params.Inputs["expDays"] = os.Getenv("EXPIRY_DAYS")
	jsonParams, err := json.Marshal(params.Inputs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errResponse{Message: err.Error()})
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
			return c.JSON(http.StatusBadRequest, errResponse{Message: fmt.Sprintf("unsupported type %s", jobType)})
		}
	}

	// Add to cache
	rh.JobsCache.Add(&j)

	// Create job
	err = j.Create()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errResponse{Message: fmt.Sprintf("submission errorr %s", err.Error())})
	}

	var outputs interface{}

	switch p.Info.JobControlOptions[0] {
	case "sync-execute":
		j.Run()

		if j.CurrentStatus() == "successful" {
			if p.Outputs != nil {
				outputs, err = FetchResults(rh.S3Svc, j.JobID())
				if err != nil {
					return c.JSON(http.StatusInternalServerError, errResponse{Message: err.Error()})
				}
			}
			resp := map[string]interface{}{"jobID": j.JobID(), "outputs": outputs}
			return c.JSON(http.StatusOK, resp)
		} else {
			resp := jobResponse{ProcessID: j.ProcessID(), Type: "process", JobID: jobID, Status: "0", Message: "Job Failed. Call logs route for details."}
			return c.JSON(http.StatusInternalServerError, resp)
		}
	case "async-execute":
		go j.Run()
		resp := jobResponse{ProcessID: j.ProcessID(), Type: "process", JobID: jobID, Status: "accepted"}
		return c.JSON(http.StatusCreated, resp)
	default:
		resp := jobResponse{ProcessID: j.ProcessID(), Type: "process", JobID: jobID, Status: "0", Message: "incorrect controller option defined in process configuration"}
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
	if j, ok := rh.JobsCache.Jobs[jobID]; ok {
		err := (*j).Kill()
		if err != nil {
			return c.JSON(http.StatusBadRequest, jobResponse{ProcessID: (*j).ProcessID(), Type: "process", JobID: jobID, Status: (*j).CurrentStatus(), Message: err.Error()})
		}
		return c.JSON(http.StatusOK, jobResponse{ProcessID: (*j).ProcessID(), Type: "process", JobID: jobID, Status: (*j).CurrentStatus(), Message: fmt.Sprintf("job %s dismissed", jobID)})
	}
	return c.JSON(http.StatusNotFound, errResponse{Message: fmt.Sprintf("job %s not in the active jobs list", jobID)})
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
// @Success 200 {object} JobLogs
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

	output := map[string]interface{}{"type": "process", "jobID": jobID, "status": 0, "message": "jobID not found"}
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
