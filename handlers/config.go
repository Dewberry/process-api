package handlers

import (
	"app/jobs"
	pr "app/processes"
	"encoding/json"
	"io"
	"log"
	"os"
	"strconv"
	"text/template"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
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
	DB          *jobs.DB
	ActiveJobs  *jobs.ActiveJobs
	ProcessList *pr.ProcessList
}

// Pretty print a JSON
func prettyPrint(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// Initializes resources and return a new handler
// errors are fatal
func NewRESTHander(pluginsDir string) *RESTHandler {
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
	funcMap := template.FuncMap{
		"prettyPrint": prettyPrint, // to pretty pring JSONs for results and metadata
	}

	config.T = Template{
		templates: template.Must(template.New("").Funcs(funcMap).ParseGlob("views/*.html")),
	}

	s3Mock, err := strconv.ParseBool(os.Getenv("S3_MOCK"))
	if err != nil {
		log.Fatal("error parsing `S3_MOCK`, must be bool not %s" + os.Getenv("S3_MOCK") + err.Error())
	}
	if s3Mock {
		s3Config := &aws.Config{
			Credentials:      credentials.NewStaticCredentials("user", "password", ""),
			Endpoint:         aws.String("http://localhost:9000"),
			Region:           aws.String("us-east-1"),
			DisableSSL:       aws.Bool(true),
			S3ForcePathStyle: aws.Bool(true),
		}
		sess := session.Must(session.NewSession(s3Config))
		config.S3Svc = s3.New(sess)

	} else {
		// Set up a session with AWS credentials and region
		sess := session.Must(session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
		}))
		config.S3Svc = s3.New(sess)
	}

	// Setup Active Jobs that will store all jobs currently in process
	ac := jobs.ActiveJobs{}
	ac.Jobs = make(map[string]*jobs.Job)
	config.ActiveJobs = &ac

	adb := jobs.InitDB(".data/db.sqlite")
	config.DB = adb

	processList, err := pr.LoadProcesses(pluginsDir)
	if err != nil {
		log.Fatal(err)
	}

	config.ProcessList = &processList

	return &config
}
