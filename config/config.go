package config

import (
	"app/jobs"
	pr "app/processes"
	"io"
	"text/template"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

// Store for templates and a receiver function to render them
type Template struct {
	templates *template.Template
}

// Render the named template with the data
func (t Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// Store configuration for the API
type APIConfig struct {
	Title       string
	Description string
	ConformsTo  []string
	T           Template
	S3Svc       *s3.S3
	JobsCache   *jobs.JobsCache
	ProcessList *pr.ProcessList
}

// Init initializes the API's configuration
func Init(processesDir string, cacheSize uint64) (*APIConfig, error) {
	// working with pointers here so as not to copy large templates, yamls, and job cache
	config := APIConfig{
		Title:       "process-api",
		Description: "ogc process api written in Golang for use with cloud service controllers to manage asynchronous requests",
		ConformsTo: []string{
			"http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/ogc-process-description",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/core",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/json",
			"http://www.opengis.net/spec/ogcapi-processes-1/1.0/conf/html",
		},
	}

	// Read all the html templates
	config.T = Template{
		templates: template.Must(template.ParseGlob("public/views/*.html")),
	}

	// Set up a session with AWS credentials and region
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	config.S3Svc = s3.New(sess)

	// Setup Job Cache that will store all jobs
	jc := jobs.JobsCache{MaxSizeBytes: uint64(cacheSize),
		CurrentSizeBytes: 0, TrimThreshold: 0.80}
	// load from previous snapshot if it exist
	err := jc.LoadCacheFromFile()
	if err != nil {
		log.Errorf("Error loading snapshot: %s \n", err.Error())
		log.Info("Starting with clean database.")
		jc.Jobs = make(map[string]*jobs.Job)
	}
	config.JobsCache = &jc

	processList, err := pr.LoadProcesses(processesDir)
	if err != nil {
		return nil, err
	}
	config.ProcessList = &processList

	return &config, nil
}
