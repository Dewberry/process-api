// Package handlers implements echo handler functions.
// It communicates with jobs and processes package to get required resources
package handlers

import (
	"app/jobs"
	"app/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// base error
type errResponse struct {
	HTTPStatus int    `json:"-"`
	Message    string `json:"message"`
}

// jobResponse store response of different job endpoints
type jobResponse struct {
	Type       string      `default:"process" json:"type"`
	JobID      string      `json:"jobID"`
	LastUpdate time.Time   `json:"updated,omitempty"`
	Status     string      `json:"status"`
	ProcessID  string      `json:"processID,omitempty"`
	Message    string      `json:"message,omitempty"`
	Outputs    interface{} `json:"outputs,omitempty"`
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
		return c.JSON(http.StatusBadRequest, errResponse{Message: "Invalid option for query parameter 'f'. Valid options are 'html' or 'json'. Default (i.e. not specified) is json for non browser requests and html for browser requests."})
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
	case "json":
		return c.JSON(httpStatus, output)
	default:
		userAgent := c.Request().UserAgent()
		// this method is not foolproof
		if strings.Contains(userAgent, "Mozilla") && strings.Contains(userAgent, "AppleWebKit") {
			// Request is coming from a browser
			return c.Render(httpStatus, renderName, output)
		} else {
			// Request is not coming from a browser
			return c.JSON(httpStatus, output)
		}
	}
}

// runRequestBody provides the required inputs for containerized processes
type runRequestBody struct {
	Inputs  map[string]interface{} `json:"inputs"`
	EnvVars map[string]string      `json:"environmentVariables"`
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
		"title":       rh.Title,
		"description": rh.Description,
	}
	return prepareResponse(c, http.StatusOK, "landing", output)
}

// Conformance godoc
// @Summary API Conformance List
// @Description [Conformance Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_conformance_classes)
// @Tags info
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{} "conformsTo:["http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/...."]"
// @Router /conformance [get]
func (rh *RESTHandler) Conformance(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	output := map[string][]string{
		"conformsTo": rh.ConformsTo,
	}

	return prepareResponse(c, http.StatusOK, "conformance", output)
}

// ProcessListHandler godoc
// @Summary List Available Processes
// @Description [Process List Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_process_list)
// @Tags processes
// @Accept */*
// @Produce json
// @Success 200 {object} []Info
// @Router /processes [get]
func (rh *RESTHandler) ProcessListHandler(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	// to meet ogc api core requirement /req/core/pl-limit-definition
	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit > 100 || limit < 1 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	if offset >= len(rh.ProcessList.InfoList) {
		return prepareResponse(c, http.StatusOK, "processes", rh.ProcessList.InfoList[0:0])
	}

	upperBound := offset + limit
	if upperBound > len(rh.ProcessList.InfoList) {
		upperBound = len(rh.ProcessList.InfoList)
	}

	return prepareResponse(c, http.StatusOK, "processes", rh.ProcessList.InfoList[offset:upperBound])
}

// ProcessDescribeHandler godoc
// @Summary Describe Process Information
// @Description [Process Description Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_process_description)
// @Tags processes
// @Param processID path string true "processID"
// @Accept */*
// @Produce json
// @Success 200 {object} ProcessDescription
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
// @Success 200 {object} jobResponse
// @Router /processes/{processID}/execution [post]
// Does not produce HTML
func (rh *RESTHandler) Execution(c echo.Context) error {
	processID := c.Param("processID")

	if processID == "" {
		return c.JSON(http.StatusBadRequest, errResponse{Message: "'processID' parameter is required"})
	}

	p, err := rh.ProcessList.Get(processID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errResponse{Message: "'processID' incorrect"})
	}

	var params runRequestBody
	err = c.Bind(&params)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errResponse{Message: err.Error()})
	}

	if params.Inputs == nil {
		return c.JSON(http.StatusBadRequest, errResponse{Message: "'inputs' is required in the body of the request"})
	}

	err = p.VerifyInputs(params.Inputs)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errResponse{Message: err.Error()})
	}

	var j jobs.Job
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
	if p.Container.Command == nil {
		cmd = []string{string(jsonParams)}
	} else {
		cmd = append(p.Container.Command, string(jsonParams))
	}

	if jobType == "sync-execute" {
		j = &jobs.DockerJob{
			UUID:        jobID,
			ProcessName: processID,
			Image:       p.Container.Image,
			EnvVars:     p.Container.EnvVars,
			Resources:   jobs.Resources(p.Container.Resources),
			Cmd:         cmd,
			DB:          rh.DB,
		}

	} else {
		host := p.Host.Type
		switch host {
		case "aws-batch":
			md := fmt.Sprintf("%s/%s.json", os.Getenv("S3_META_DIR"), jobID)

			j = &jobs.AWSBatchJob{
				UUID:             jobID,
				ProcessName:      processID,
				Image:            p.Container.Image,
				Cmd:              cmd,
				JobDef:           p.Host.JobDefinition,
				JobQueue:         p.Host.JobQueue,
				JobName:          "ogc-api-id-" + jobID,
				MetaDataLocation: md,
				ProcessVersion:   p.Info.Version,
				DB:               rh.DB,
			}
		default:
			return c.JSON(http.StatusBadRequest, errResponse{Message: fmt.Sprintf("unsupported type %s", jobType)})
		}
	}

	// Create job
	err = j.Create()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errResponse{Message: fmt.Sprintf("submission errorr %s", err.Error())})
	}

	// Add to active jobs
	rh.ActiveJobs.Add(&j)

	switch p.Info.JobControlOptions[0] {
	case "sync-execute":
		defer rh.ActiveJobs.Remove(&j)

		j.Run()

		if j.CurrentStatus() == "successful" {
			var outputs interface{}

			if p.Outputs != nil {
				outputs, err = jobs.FetchResults(rh.S3Svc, j.JobID())
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
		go func() {
			defer rh.ActiveJobs.Remove(&j)
			j.Run()
		}()
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
// @Success 200 {object} jobResponse
// @Router /jobs/{jobID} [delete]
// Does not produce HTML
func (rh *RESTHandler) JobDismissHandler(c echo.Context) error {

	jobID := c.Param("jobID")
	if j, ok := rh.ActiveJobs.Jobs[jobID]; ok {
		err := (*j).Kill()
		if err != nil {
			return c.JSON(http.StatusBadRequest, errResponse{Message: err.Error()})
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
// @Success 200 {object} JobRecord
// @Router /jobs/{jobID} [get]
func (rh *RESTHandler) JobStatusHandler(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	jobID := c.Param("jobID")
	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok {
		resp := jobs.JobRecord{
			ProcessID:  (*job).ProcessID(),
			JobID:      (*job).JobID(),
			LastUpdate: (*job).LastUpdate(),
			Status:     (*job).CurrentStatus(),
		}
		return prepareResponse(c, http.StatusOK, "js", resp)
	} else if jRcrd, ok := rh.DB.GetJob(jobID); ok {
		return prepareResponse(c, http.StatusOK, "js", jRcrd)
	}
	output := errResponse{HTTPStatus: http.StatusNotFound, Message: fmt.Sprintf("%s job id not found", jobID)}
	return prepareResponse(c, http.StatusNotFound, "error", output)
}

// @Summary Job Results
// @Description [Job Results Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} jobResponse
// @Router /jobs/{jobID}/results [get]
// Does not produce HTML
func (rh *RESTHandler) JobResultsHandler(c echo.Context) error {
	jobID := c.Param("jobID")
	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok { // ActiveJobs hit
		output := jobResponse{Type: "process", JobID: jobID, Status: (*job).CurrentStatus(), Message: "results not ready", LastUpdate: (*job).LastUpdate()}
		return c.JSON(http.StatusNotFound, output)

	} else if jRcrd, ok := rh.DB.GetJob(jobID); ok { // db hit

		switch jRcrd.Status {
		case jobs.SUCCESSFUL:
			outputs, err := jobs.FetchResults(rh.S3Svc, jRcrd.JobID)
			if err != nil {
				if err.Error() == "not found" {
					output := jobResponse{Type: "process", JobID: jobID, Status: jRcrd.Status, Message: "results not available, resource might have expired."}
					return c.JSON(http.StatusNotFound, output)
				}
				return c.JSON(http.StatusInternalServerError, err.Error())
			}
			output := jobResponse{
				Type:       "process",
				JobID:      jobID,
				Status:     jRcrd.Status,
				LastUpdate: jRcrd.LastUpdate,
				Outputs:    outputs,
			}
			return c.JSON(http.StatusOK, output)

		case jobs.FAILED, jobs.DISMISSED:
			output := jobResponse{Type: "process", JobID: jobID, Status: jRcrd.Status, Message: "job Failed or Dismissed. Call logs route for details", LastUpdate: (*job).LastUpdate()}
			return c.JSON(http.StatusOK, output)

		default:
			output := jobResponse{Type: "process", JobID: jobID, Status: jRcrd.Status, Message: "results not ready", LastUpdate: jRcrd.LastUpdate}
			return c.JSON(http.StatusNotFound, output)
		}

	}
	// miss
	output := errResponse{Message: "jobID not found"}
	return c.JSON(http.StatusNotFound, output)
}

func (rh *RESTHandler) JobMetaDataHandler(c echo.Context) error {
	jobID := c.Param("jobID")
	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok { // ActiveJobs hit
		// for sync jobs jobid is not returned to user untill the job is no longer in activeJobs, so it means job is not async
		output := jobResponse{Type: "process", JobID: jobID, Status: (*job).CurrentStatus(), Message: "metadata not ready", LastUpdate: (*job).LastUpdate()}
		return c.JSON(http.StatusNotFound, output)
	} else if jRcrd, ok := rh.DB.GetJob(jobID); ok { // db hit
		// todo
		// if jRcrd.Mode == "sync"

		switch jRcrd.Status {
		case jobs.SUCCESSFUL:
			md, err := jobs.FetchMeta(rh.S3Svc, jobID)
			if err != nil {
				if err.Error() == "not found" {
					output := jobResponse{Type: "process", JobID: jobID, Status: jRcrd.Status, Message: "metadata not found."}
					return c.JSON(http.StatusInternalServerError, output)
				}
				return c.JSON(http.StatusInternalServerError, err.Error())
			}
			return c.JSON(http.StatusOK, md)

		case jobs.FAILED, jobs.DISMISSED:
			output := jobResponse{Type: "process", JobID: jobID, Status: jRcrd.Status, Message: "job Failed or Dismissed. Metadata only available for successful jobs.", LastUpdate: jRcrd.LastUpdate}
			return c.JSON(http.StatusOK, output)

		default:
			output := jobResponse{Type: "process", JobID: jobID, Status: jRcrd.Status, Message: "metadata not ready", LastUpdate: jRcrd.LastUpdate}
			return c.JSON(http.StatusNotFound, output)
		}

	}
	// miss
	output := errResponse{Message: "jobID not found"}
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

	err := validateFormat(c)
	if err != nil {
		return err
	}

	var logs jobs.JobLogs
	var errLogs error

	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok { // ActiveJobs hit
		logs, errLogs = (*job).Logs()
	} else if ok := rh.DB.CheckJobExist(jobID); ok { // db hit
		logs, errLogs = rh.DB.GetLogs(jobID)
	} else { // miss
		output := errResponse{HTTPStatus: http.StatusNotFound, Message: "jobID not found"}
		return prepareResponse(c, http.StatusNotFound, "error", output)
	}

	if errLogs != nil {
		output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: "error while fetching logs: " + errLogs.Error()}
		return prepareResponse(c, http.StatusInternalServerError, "error", output)
	}

	logs.Prettify()
	return prepareResponse(c, http.StatusOK, "logs", logs)

}

// @Summary Summary of all (active) Jobs
// @Description [Job List Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} []JobRecord
// @Router /jobs [get]
func (rh *RESTHandler) ListJobsHandler(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit > 100 || limit < 1 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	result, err := rh.DB.GetJobs(limit, offset)
	if err != nil {
		output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: err.Error()}
		return prepareResponse(c, http.StatusNotFound, "error", output)
	}
	return prepareResponse(c, http.StatusOK, "jobs", result)
}
