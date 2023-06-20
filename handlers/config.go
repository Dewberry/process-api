package handlers

import (
	"app/jobs"
	pr "app/processes"
	"io"
	"text/template"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/labstack/echo/v4"
)

// Store for templates and a receiver function to render them
type Template struct {
	templates *template.Template
}

// Render the named template with the data
func (t Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// Store configuration for the handler
type RESTHandler struct {
	Title       string
	Description string
	ConformsTo  []string
	T           Template
	S3Svc       *s3.S3
	ActiveJobs  *jobs.ActiveJobs
	ProcessList *pr.ProcessList
}

// Initializes resources and return a new handler
func NewRESTHander(pluginsDir string) (*RESTHandler, error) {
	// working with pointers here so as not to copy large templates, yamls, and ActiveJobs
	config := RESTHandler{
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

	// Setup Active Jobs that will store all jobs currently in process
	ac := jobs.ActiveJobs{}
	ac.Jobs = make(map[string]*jobs.Job)
	config.ActiveJobs = &ac

	processList, err := pr.LoadProcesses(pluginsDir)
	if err != nil {
		return nil, err
	}
	config.ProcessList = &processList

	return &config, nil
}
