package handlers

import (
	"app/jobs"
	"app/utils"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

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

// LandingPage godoc
// @Summary Landing Page
// @Description [LandingPage Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_landing_page)
// @Tags info
// @Accept */*
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router / [get]
func LandingPage(c echo.Context) error {
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
// @Success 200 {object} map[string]interface{} "conformsTo:["http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/...."]"
// @Router /conformance [get]
func Conformance(c echo.Context) error {
	err := validateFormat(c)
	if err != nil {
		return err
	}

	output := map[string][]string{
		"conformsTo": {
			"http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/ogc-process-description",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/core",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/json",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/html",
		},
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
func ProcessListHandler(pl *jobs.ProcessList) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := validateFormat(c)
		if err != nil {
			return err
		}

		processList, err := pl.ListAll()
		if err != nil {
			return prepareResponse(c, http.StatusInternalServerError, "error", errResponse{Message: err.Error(), HTTPStatus: http.StatusInternalServerError})
		}

		return prepareResponse(c, http.StatusOK, "processes", processList)
	}
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
func ProcessDescribeHandler(pl *jobs.ProcessList) echo.HandlerFunc {
	return func(c echo.Context) error {
		processID := c.Param("processID")

		err := validateFormat(c)
		if err != nil {
			return err
		}

		p, err := pl.Get(processID)
		if err != nil {
			return prepareResponse(c, http.StatusBadRequest, "error", errResponse{Message: err.Error(), HTTPStatus: http.StatusBadRequest})
		}

		description, err := p.Describe()
		if err != nil {
			return prepareResponse(c, http.StatusInternalServerError, "error", errResponse{Message: err.Error(), HTTPStatus: http.StatusInternalServerError})
		}
		return prepareResponse(c, http.StatusOK, "process", description)
	}
}

// @Summary Execute Process
// @Description [Execute Process Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_create_job)
// @Tags processes
// @Accept */*
// @Produce json
// @Success 200 {object} jobs.JobResponse
// @Router /processes/{processID}/execution [post]
// Does not produce HTML
func Execution(pl *jobs.ProcessList, jc *jobs.JobsCache, s3sv *s3.S3) echo.HandlerFunc {
	return func(c echo.Context) error {

		processID := c.Param("processID")

		log.Debug("processID", processID)
		if processID == "" {
			return c.JSON(http.StatusBadRequest, errResponse{Message: "'processID' parameter is required"})
		}

		p, err := pl.Get(processID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errResponse{Message: "'processID' incorrect"})
		}

		var params jobs.RunRequestBody
		err = c.Bind(&params)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errResponse{Message: err.Error()})
		}

		// Review this section
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
		if p.Runtime.Command == nil {
			cmd = []string{string(jsonParams)}
		} else {
			cmd = append(p.Runtime.Command, string(jsonParams))
		}

		if jobType == "sync-execute" {
			j = &jobs.DockerJob{
				Ctx:         context.TODO(),
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
				j = &jobs.AWSBatchJob{
					Ctx:         context.TODO(),
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
		jc.Add(&j)

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
					outputs, err = jobs.FetchResults(s3sv, j.JobID())
					if err != nil {
						return c.JSON(http.StatusInternalServerError, errResponse{Message: err.Error()})
					}
				}
				resp := map[string]interface{}{"jobID": j.JobID(), "outputs": outputs}
				return c.JSON(http.StatusOK, resp)
			} else {
				resp := jobs.JobResponse{ProcessID: j.ProcessID(), Type: "process", JobID: jobID, Status: "0", Message: "Job Failed. Call logs route for details."}
				return c.JSON(http.StatusInternalServerError, resp)
			}
		case "async-execute":
			go j.Run()
			resp := jobs.JobResponse{ProcessID: j.ProcessID(), Type: "process", JobID: jobID, Status: "accepted"}
			return c.JSON(http.StatusCreated, resp)
		default:
			resp := jobs.JobResponse{ProcessID: j.ProcessID(), Type: "process", JobID: jobID, Status: "0", Message: "incorrect controller option defined in process configuration"}
			return c.JSON(http.StatusInternalServerError, resp)
		}
	}
}

// @Summary Dismiss Job
// @Description [Dismss Job Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#ats_dismiss)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} jobs.JobResponse
// @Router /jobs/{jobID} [delete]
// Does not produce HTML
func JobDismissHandler(jc *jobs.JobsCache) echo.HandlerFunc {
	return func(c echo.Context) error {

		jobID := c.Param("jobID")
		if j, ok := jc.Jobs[jobID]; ok {
			err := (*j).Kill()
			if err != nil {
				return c.JSON(http.StatusBadRequest, errResponse{Message: err.Error()})
			}
			return c.JSON(http.StatusOK, jobs.JobResponse{ProcessID: (*j).ProcessID(), Type: "process", JobID: jobID, Status: (*j).CurrentStatus(), Message: fmt.Sprintf("job %s dismissed", jobID)})
		}
		return c.JSON(http.StatusNotFound, errResponse{Message: fmt.Sprintf("job %s not in the active jobs list", jobID)})
	}
}

// @Summary Job Status
// @Description [xxx Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_status_info)
// @Tags jobs
// @Info [Format YAML](http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/statusInfo.yaml)
// @Accept */*
// @Produce json
// @Success 200 {object} JobStatus
// @Router /jobs/{jobID} [get]
func JobStatusHandler(jc *jobs.JobsCache) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := validateFormat(c)
		if err != nil {
			return err
		}

		jobID := c.Param("jobID")
		if job, ok := jc.Jobs[jobID]; ok {
			resp := jobs.JobStatus{
				ProcessID:  (*job).ProcessID(),
				JobID:      (*job).JobID(),
				LastUpdate: (*job).LastUpdate(),
				Status:     (*job).CurrentStatus(),
			}
			return prepareResponse(c, http.StatusOK, "js", resp)
		}
		output := errResponse{HTTPStatus: http.StatusNotFound, Message: fmt.Sprintf("%s job id not found", jobID)}
		return prepareResponse(c, http.StatusNotFound, "error", output)
	}
}

// @Summary Job Results
// @Description [Job Results Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} jobs.JobResponse
// @Router /jobs/{jobID} [get]
// Does not produce HTML
func JobResultsHandler(jc *jobs.JobsCache, s3sv *s3.S3) echo.HandlerFunc {
	return func(c echo.Context) error {
		jobID := c.Param("jobID")
		if job, ok := jc.Jobs[jobID]; ok {
			switch (*job).CurrentStatus() {
			case jobs.SUCCESSFUL:
				outputs, err := jobs.FetchResults(s3sv, (*job).JobID())
				if err != nil {
					if err.Error() == "not found" {
						output := jobs.JobResponse{Type: "process", JobID: jobID, Status: (*job).CurrentStatus(), Message: "results not available, resource might have expired."}
						return c.JSON(http.StatusNotFound, output)
					}
					return c.JSON(http.StatusInternalServerError, err.Error())
				}
				output := jobs.JobResponse{
					Type:       "process",
					JobID:      jobID,
					Status:     (*job).CurrentStatus(),
					LastUpdate: (*job).LastUpdate(),
					Outputs:    outputs,
				}
				return c.JSON(http.StatusOK, output)

			case jobs.FAILED, jobs.DISMISSED:
				output := jobs.JobResponse{Type: "process", JobID: jobID, Status: (*job).CurrentStatus(), Message: "job Failed or Dismissed. Call logs route for details", LastUpdate: (*job).LastUpdate()}
				return c.JSON(http.StatusOK, output)

			default:
				output := jobs.JobResponse{Type: "process", JobID: jobID, Status: (*job).CurrentStatus(), Message: "results not ready", LastUpdate: (*job).LastUpdate()}
				return c.JSON(http.StatusNotFound, output)
			}

		}
		output := errResponse{Message: "jobID not found"}
		return c.JSON(http.StatusNotFound, output)
	}
}

// @Summary Job Logs
// @Description
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} JobLogs
// @Router /jobs/{jobID}/logs [get]
func JobLogsHandler(jc *jobs.JobsCache) echo.HandlerFunc {
	return func(c echo.Context) error {
		jobID := c.Param("jobID")

		err := validateFormat(c)
		if err != nil {
			return err
		}

		if job, ok := jc.Jobs[jobID]; ok {
			logs, err := (*job).Logs()
			if err != nil {
				if err.Error() == "not found" {
					output := errResponse{HTTPStatus: http.StatusNotFound, Message: err.Error()}
					return prepareResponse(c, http.StatusNotFound, "error", output)
				}
				output := errResponse{HTTPStatus: http.StatusNotFound, Message: "Error while fetching logs: " + err.Error()}
				return prepareResponse(c, http.StatusInternalServerError, "error", output)
			}

			return prepareResponse(c, http.StatusOK, "logs", logs)
		}

		output := errResponse{HTTPStatus: http.StatusNotFound, Message: "jobID not found"}
		return prepareResponse(c, http.StatusNotFound, "error", output)
	}
}

// @Summary Summary of all (cached) Jobs
// @Description [Job List Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)
// @Tags jobs
// @Accept */*
// @Produce json
// @Success 200 {object} []JobStatus
// @Router /jobs [get]
func JobsCacheHandler(jc *jobs.JobsCache) echo.HandlerFunc {
	return func(c echo.Context) error {
		err := validateFormat(c)
		if err != nil {
			return err
		}

		jobsList := jc.ListJobs()
		return prepareResponse(c, http.StatusOK, "jobs", jobsList)
	}
}
