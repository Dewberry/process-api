// Package handlers implements echo handler functions.
// It communicates with jobs and processes package to get required resources
package handlers

// Some design decisions:
// Errors should use errResponse, no need to give job status and other details in errors
// Results, Logs, Metadata will reply back with data only no status and other details
// These rules are in compliance with Specs

import (
	"app/jobs"
	"app/utils"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"github.com/sirupsen/logrus"
)

// base error
type errResponse struct {
	HTTPStatus int    `json:"-"`
	Message    string `json:"message"`
}

// jobResponse store response of different job endpoints
type jobResponse struct {
	Type       string      `default:"process" json:"type,omitempty"`
	JobID      string      `json:"jobID"`
	LastUpdate time.Time   `json:"updated,omitempty"`
	Status     string      `json:"status,omitempty"`
	ProcessID  string      `json:"processID,omitempty"`
	Message    string      `json:"message,omitempty"`
	Outputs    interface{} `json:"outputs,omitempty"`
}

type link struct {
	Href  string `json:"href"`
	Rel   string `json:"rel,omitempty"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
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
// If query parameter not defined then fall back to Accept header as suggested in OGC Specs
// Both are not defined then return JSON
func prepareResponse(c echo.Context, httpStatus int, renderName string, output interface{}) error {
	// this is to conform to OGC Process API classes: /req/html/definition and /req/json/definition
	outputFormat := c.QueryParam("f")
	switch outputFormat {
	case "html":
		return c.Render(httpStatus, renderName, output)
	case "json":
		return c.JSON(httpStatus, output)
	default:
		accept := c.Request().Header.Get("Accept")
		if strings.Contains(accept, "application/json") {
			// Browsers generally send text/html as an accept header
			// return c.Render(httpStatus, renderName, output)
			return c.JSON(httpStatus, output)
		} else if strings.Contains(accept, "text/html") {
			// Browsers generally send text/html as an accept header
			return c.Render(httpStatus, renderName, output)
		} else {
			// Default to JSON for any other cases, including 'Accept: */*'
			return c.JSON(httpStatus, output)
		}
	}
}

// runRequestBody provides the required inputs for containerized processes
// specs: https://developer.ogc.org/api/processes/index.html#tag/Execute
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
// @Success 200 {object} map[string]interface{}
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
		limit = 20
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// instantiate result variable without importing processes pkg
	result := rh.ProcessList.InfoList[0:0]

	if offset < len(rh.ProcessList.InfoList) {
		upperBound := offset + limit
		if upperBound > len(rh.ProcessList.InfoList) {
			upperBound = len(rh.ProcessList.InfoList)
		}
		result = rh.ProcessList.InfoList[offset:upperBound]
	}

	// required by /req/core/process-list-success
	links := make([]link, 0)

	// if offset is not 0
	if offset != 0 {
		lnk := link{
			Href:  fmt.Sprintf("/processes?offset=%v&limit=%v", offset-limit, limit),
			Title: "prev",
		}
		links = append(links, lnk)
	}

	// if limit is not exhausted
	if limit == len(result) {
		lnk := link{
			Href:  fmt.Sprintf("/processes?offset=%v&limit=%v", offset+limit, limit),
			Title: "next",
		}
		links = append(links, lnk)
	}

	output := make(map[string]interface{}, 0)
	output["processes"] = result
	output["links"] = links

	return prepareResponse(c, http.StatusOK, "processes", output)
}

// ProcessDescribeHandler godoc
// @Summary Describe Process Information
// @Description [Process Description Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_process_description)
// @Tags processes
// @Param processID path string true "example: pyecho"
// @Accept */*
// @Produce json
// @Success 200 {object} processes.processDescription
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
// @Accept json
// @Produce json
// @Param processID path string true "pyecho"
// @Param inputs body string true "example: {inputs: {text:Hello World!}} (add double quotes for all strings in the payload)"
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

	mode := p.Info.JobControlOptions[0]
	host := p.Host.Type

	// ----------- Process related setup is complete at this point ---------

	jobID := uuid.New().String()

	// switch host {
	// case "local":
	// 	params.Inputs["resultsCallbackUri"] = fmt.Sprintf("%s/jobs/%s/results_update", os.Getenv("API_URL_LOCAL"), jobID)
	// default:
	// 	params.Inputs["resultsCallbackUri"] = fmt.Sprintf("%s/jobs/%s/results_update", os.Getenv("API_URL_PUBLIC"), jobID)
	// }

	var j jobs.Job
	switch host {
	case "local":
		j = &jobs.DockerJob{
			UUID:           jobID,
			ProcessName:    processID,
			ProcessVersion: p.Info.Version,
			Image:          p.Container.Image,
			EnvVars:        p.Container.EnvVars,
			Resources:      jobs.Resources(p.Container.Resources),
			Cmd:            cmd,
			StorageSvc:     rh.StorageSvc,
			DB:             rh.DB,
			DoneChan:       rh.MessageQueue.JobDone,
		}

	case "aws-batch":
		j = &jobs.AWSBatchJob{
			UUID:           jobID,
			ProcessName:    processID,
			Image:          p.Container.Image,
			Cmd:            cmd,
			JobDef:         p.Host.JobDefinition,
			JobQueue:       p.Host.JobQueue,
			JobName:        fmt.Sprintf("%s_%s", rh.Name, jobID),
			ProcessVersion: p.Info.Version,
			StorageSvc:     rh.StorageSvc,
			DB:             rh.DB,
			DoneChan:       rh.MessageQueue.JobDone,
		}
	}

	// Create job
	err = j.Create()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errResponse{Message: fmt.Sprintf("submission error %s", err.Error())})
	}

	// Add to active jobs
	rh.ActiveJobs.Add(&j)

	resp := jobResponse{ProcessID: j.ProcessID(), Type: "process", JobID: jobID, Status: j.CurrentStatus()}
	switch mode {
	case "sync-execute":
		j.WaitForRunCompletion()
		resp.Status = j.CurrentStatus()

		if resp.Status == "successful" {
			var outputs interface{}

			if p.Outputs != nil {
				outputs, err = jobs.FetchResults(rh.StorageSvc, j.JobID())
				if err != nil {
					resp.Message = "error fetching results. Error: " + err.Error()
					return c.JSON(http.StatusInternalServerError, resp)
				}
			}
			resp.Outputs = outputs
			return c.JSON(http.StatusOK, resp)
		} else {
			resp.Message = "job unsuccessful. Call logs route for details"
			return c.JSON(http.StatusInternalServerError, resp)
		}
	case "async-execute":
		resp.Status = j.CurrentStatus()
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
// @Description [Job Status Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_status_info)
// @Tags jobs
// @Info [Format YAML](http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/statusInfo.yaml)
// @Accept */*
// @Param jobID path string true "example: 44d9ca0e-2ca7-4013-907f-a8ccc60da3b4"
// @Produce json
// @Success 200 {object} jobResponse
// @Router /jobs/{jobID} [get]
func (rh *RESTHandler) JobStatusHandler(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	jobID := c.Param("jobID")
	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok {
		resp := jobResponse{
			ProcessID:  (*job).ProcessID(),
			JobID:      (*job).JobID(),
			LastUpdate: (*job).LastUpdate(),
			Status:     (*job).CurrentStatus(),
		}
		return prepareResponse(c, http.StatusOK, "jobStatus", resp)
	} else if jRcrd, ok := rh.DB.GetJob(jobID); ok {
		resp := jobResponse{
			ProcessID:  jRcrd.ProcessID,
			JobID:      jRcrd.JobID,
			LastUpdate: jRcrd.LastUpdate,
			Status:     jRcrd.Status,
		}
		return prepareResponse(c, http.StatusOK, "jobStatus", resp)
	}
	output := errResponse{HTTPStatus: http.StatusNotFound, Message: fmt.Sprintf("%s job id not found", jobID)}
	return prepareResponse(c, http.StatusNotFound, "error", output)
}

// @Summary Job Results
// @Description [Job Results Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)
// @Tags jobs
// @Accept */*
// @Produce json
// @Param jobID path string true "ex: 44d9ca0e-2ca7-4013-907f-a8ccc60da3b4"
// @Success 200 {object} map[string]interface{}
// @Router /jobs/{jobID}/results [get]
// Does not produce HTML
func (rh *RESTHandler) JobResultsHandler(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	jobID := c.Param("jobID")
	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok { // ActiveJobs hit
		output := errResponse{HTTPStatus: http.StatusNotFound, Message: fmt.Sprintf("results not ready, job %s", (*job).CurrentStatus())}
		return prepareResponse(c, http.StatusNotFound, "error", output)

	} else if jRcrd, ok := rh.DB.GetJob(jobID); ok { // db hit

		switch jRcrd.Status {
		case jobs.SUCCESSFUL:
			outputs, err := jobs.FetchResults(rh.StorageSvc, jRcrd.JobID)
			if err != nil {
				if err.Error() == "not found" {
					output := errResponse{HTTPStatus: http.StatusNotFound, Message: "results not available"}
					return prepareResponse(c, http.StatusNotFound, "error", output)
				}
				output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: err.Error()}
				return prepareResponse(c, http.StatusInternalServerError, "error", output)
			}
			output := jobResponse{JobID: jobID, Outputs: outputs}
			return prepareResponse(c, http.StatusOK, "jobResults", output)

		case jobs.FAILED, jobs.DISMISSED:
			output := errResponse{HTTPStatus: http.StatusNotFound, Message: "job Failed or Dismissed. Call logs route for details"}
			return prepareResponse(c, http.StatusNotFound, "error", output)

		default:
			output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: "job status out of sync in database"}
			return prepareResponse(c, http.StatusInternalServerError, "error", output)
		}

	}
	// miss
	output := errResponse{HTTPStatus: http.StatusNotFound, Message: fmt.Sprintf("%s job id not found", jobID)}
	return prepareResponse(c, http.StatusNotFound, "error", output)
}

// @Summary Job Metadata
// @Description Provides metadata associated with a job
// @Tags jobs
// @Accept */*
// @Produce json
// @Param jobID path string true "example: 44d9ca0e-2ca7-4013-907f-a8ccc60da3b4"
// @Success 200 {object} map[string]interface{}
// @Router /jobs/{jobID}/results [get]
// Does not produce HTML
func (rh *RESTHandler) JobMetaDataHandler(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	jobID := c.Param("jobID")
	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok { // ActiveJobs hit
		output := errResponse{HTTPStatus: http.StatusNotFound, Message: fmt.Sprintf("metadata not ready, job %s", (*job).CurrentStatus())}
		return prepareResponse(c, http.StatusNotFound, "error", output)

	} else if jRcrd, ok := rh.DB.GetJob(jobID); ok { // db hit
		switch jRcrd.Status {
		case jobs.SUCCESSFUL:
			md, err := jobs.FetchMeta(rh.StorageSvc, jobID)
			if err != nil {
				if err.Error() == "not found" {
					output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: "metadata not found"}
					return prepareResponse(c, http.StatusInternalServerError, "error", output)
				}
				output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: err.Error()}
				return prepareResponse(c, http.StatusInternalServerError, "error", output)
			}
			return prepareResponse(c, http.StatusOK, "jobMetadata", md)

		case jobs.FAILED, jobs.DISMISSED:
			output := errResponse{HTTPStatus: http.StatusNotFound, Message: "job Failed or Dismissed. Metadata only available for successful jobs"}
			return prepareResponse(c, http.StatusNotFound, "error", output)

		default:
			output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: "job status out of sync in database"}
			return prepareResponse(c, http.StatusInternalServerError, "error", output)
		}

	}
	// miss
	output := errResponse{HTTPStatus: http.StatusNotFound, Message: fmt.Sprintf("%s job id not found", jobID)}
	return prepareResponse(c, http.StatusNotFound, "error", output)
}

// @Summary Job Logs
// @Description
// @Tags jobs
// @Accept */*
// @Produce json
// @Param jobID path string true "example: 44d9ca0e-2ca7-4013-907f-a8ccc60da3b4"
// @Success 200 {object} jobs.JobLogs
// @Router /jobs/{jobID}/logs [get]
func (rh *RESTHandler) JobLogsHandler(c echo.Context) error {
	jobID := c.Param("jobID")

	err := validateFormat(c)
	if err != nil {
		return err
	}

	var pid string

	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok { // ActiveJobs hit
		err = (*job).UpdateContainerLogs()
		if err != nil {
			output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: "error while updating container logs: " + err.Error()}
			return prepareResponse(c, http.StatusInternalServerError, "error", output)
		}
		pid = (*job).ProcessID()
	} else if jRcrd, ok := rh.DB.GetJob(jobID); ok { // db hit
		pid = jRcrd.ProcessID
	} else { // miss
		output := errResponse{HTTPStatus: http.StatusNotFound, Message: "jobID not found"}
		return prepareResponse(c, http.StatusNotFound, "error", output)
	}

	logs, err := jobs.FetchLogs(rh.StorageSvc, jobID, pid, false)
	if err != nil {
		output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: "error while fetching logs: " + err.Error()}
		return prepareResponse(c, http.StatusInternalServerError, "error", output)
	}

	return prepareResponse(c, http.StatusOK, "jobLogs", logs)

}

// @Summary Summary of all (active) Jobs
// @Description [Job List Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} []jobs.JobRecord
// @Router /jobs [get]
func (rh *RESTHandler) ListJobsHandler(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")
	processIDs := c.QueryParam("processID") // assuming comma-separated list: "process1,process2"
	statuses := c.QueryParam("status")

	// // to do: list jobs for that user only if not admin
	// fmt.Println(c.Request().Header.Get("X-ProcessAPI-User-Email"))
	// fmt.Println(c.Request().Header.Get("X-ProcessAPI-User-Roles"))

	var processIDList []string
	if processIDs != "" {
		processIDList = strings.Split(processIDs, ",")
	}
	for _, pid := range processIDList {
		// to do: revise along with #
		_, err = rh.ProcessList.Get(pid)
		if err != nil {
			output := errResponse{HTTPStatus: http.StatusBadRequest, Message: fmt.Sprintf("processID %s incorrect, no such process exist", pid)}
			return prepareResponse(c, http.StatusBadRequest, "error", output)
		}
	}

	var statusList []string
	if statuses != "" {
		statusList = strings.Split(statuses, ",")
	}
	for _, st := range statusList {
		switch st {
		case jobs.ACCEPTED, jobs.RUNNING, jobs.DISMISSED, jobs.FAILED, jobs.SUCCESSFUL:
			// valid status
		default:
			output := errResponse{HTTPStatus: http.StatusBadRequest, Message: "One or more status values not valid"}
			return prepareResponse(c, http.StatusBadRequest, "error", output)
		}
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit > 100 || limit < 1 {
		limit = 20
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	result, err := rh.DB.GetJobs(limit, offset, processIDList, statusList)
	if err != nil {
		output := errResponse{HTTPStatus: http.StatusInternalServerError, Message: err.Error()}
		return prepareResponse(c, http.StatusNotFound, "error", output)
	}

	// required by /req/job-list/job-list-success
	links := make([]link, 0)

	// if offset is not 0
	if offset != 0 {
		lnk := link{
			Href:  fmt.Sprintf("/jobs?offset=%v&limit=%v&processID=%v&status=%v", offset-limit, limit, processIDs, statuses),
			Title: "prev",
		}
		links = append(links, lnk)
	}

	// if limit is not exhausted
	if limit == len(result) {
		lnk := link{
			Href:  fmt.Sprintf("/jobs?offset=%v&limit=%v&processID=%v&status=%v", offset+limit, limit, processIDs, statuses),
			Title: "next",
		}
		links = append(links, lnk)
	}

	output := make(map[string]interface{}, 0)
	output["jobs"] = result
	output["links"] = links
	return prepareResponse(c, http.StatusOK, "jobs", output)
}

// Sample message body:
//
//	{
//		"status": "successful",
//		"updated": "2023-08-28T18:25:44.731Z"
//	}
//
// Time must be in RFC3339(ISO) format
func (rh *RESTHandler) JobStatusUpdateHandler(c echo.Context) error {

	jobID := c.Param("jobID")

	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok { // ActiveJobs hit
		var sm jobs.StatusMessage
		sm.Job = job
		// setup some kind of token/auth to allow only the allowed agents to post to this route
		defer c.Request().Body.Close()
		dataBytes, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errResponse{http.StatusBadRequest, "could not read message body"})
		}
		if err = json.Unmarshal(dataBytes, &sm); err != nil {
			return c.JSON(http.StatusBadRequest, errResponse{http.StatusBadRequest, "incorrect message body"})
		}
		// check status valid
		switch sm.Status {
		case jobs.ACCEPTED, jobs.RUNNING, jobs.DISMISSED, jobs.FAILED, jobs.SUCCESSFUL:
			// do nothing
		default:
			return c.JSON(http.StatusBadRequest, fmt.Sprintf("status not valid, valid options are: %s, %s, %s, %s, %s", jobs.ACCEPTED, jobs.RUNNING, jobs.DISMISSED, jobs.FAILED, jobs.SUCCESSFUL))
		}
		(*sm.Job).LogMessage(fmt.Sprintf("Status update received: %s.", sm.Status), logrus.InfoLevel)
		rh.MessageQueue.StatusChan <- sm
		return c.JSON(http.StatusAccepted, "status update received")
	} else if ok := rh.DB.CheckJobExist(jobID); ok { // db hit
		log.Infof("Status update received for inactive job: %s", jobID)
		// returning Accepted here so that callers do not retry
		return c.JSON(http.StatusAccepted, "job not an active job")
	} else {
		return c.JSON(http.StatusBadRequest, "job id not found")
	}
}

// func (rh *RESTHandler) JobResultsUpdateHandler(c echo.Context) error {

// 	// setup some kind of token/auth to allow only the container to post to this route

// 	defer c.Request().Body.Close()
// 	dataBytes, err := io.ReadAll(c.Request().Body)
// 	if err != nil {
// 		return c.JSON(http.StatusBadRequest, errResponse{http.StatusBadRequest, "incorrect message body"})
// 	}

// 	jobID := c.Param("jobID")

// 	if job, ok := rh.ActiveJobs.Jobs[jobID]; ok { // ActiveJobs hit
// 		err = (*job).WriteResults(dataBytes)
// 		if err != nil {
// 			return c.JSON(http.StatusInternalServerError, errResponse{http.StatusInternalServerError, "error writing results"})
// 		}
// 		return c.JSON(http.StatusCreated, "results processed")
// 	} else {
// 		return c.JSON(http.StatusBadRequest, "job not an active job")
// 	}

// }
