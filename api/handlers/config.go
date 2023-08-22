package handlers

import (
	"app/jobs"
	pr "app/processes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
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

// Store configuration for the handler
type RESTHandler struct {
	Title       string
	Description string
	ConformsTo  []string
	T           Template
	StorageSvc  *s3.S3
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
func NewRESTHander(pluginsDir string, dbPath string) *RESTHandler {
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

	stType, exist := os.LookupEnv("STORAGE_SERVICE")
	if !exist {
		log.Fatal("env variable STORAGE_SERVICE not set")
	}

	stSvc, err := NewStorageService(stType)
	if err != nil {
		log.Fatal(err)
	}
	config.StorageSvc = stSvc

	// Setup Active Jobs that will store all jobs currently in process
	ac := jobs.ActiveJobs{}
	ac.Jobs = make(map[string]*jobs.Job)
	config.ActiveJobs = &ac

	adb := jobs.InitDB(dbPath)
	config.DB = adb

	processList, err := pr.LoadProcesses(pluginsDir)
	if err != nil {
		log.Fatal(err)
	}
	config.ProcessList = &processList

	return &config
}

// Constructor to create storage service based on the type provided
func NewStorageService(provideType string) (*s3.S3, error) {

	switch provideType {
	case "minio":
		region := os.Getenv("MINIO_S3_REGION")
		accessKeyID := os.Getenv("MINIO_ACCESS_KEY_ID")
		secretAccessKey := os.Getenv("MINIO_SECRET_ACCESS_KEY")
		endpoint := os.Getenv("MINIO_S3_ENDPOINT")
		if endpoint == "" {
			return nil, errors.New("`MINIO_S3_ENDPOINT` env var required if STORAGE_SERVICE='minio'")
		}

		sess, err := session.NewSession(&aws.Config{
			Endpoint:         aws.String(endpoint),
			Region:           aws.String(region),
			Credentials:      credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
			S3ForcePathStyle: aws.Bool(true),
		})
		if err != nil {
			return nil, fmt.Errorf("error connecting to minio session: %s", err.Error())
		}
		return s3.New(sess), nil

	case "aws-s3":
		region := os.Getenv("AWS_REGION")
		accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
		secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
		})
		if err != nil {
			return nil, fmt.Errorf("error creating s3 session: %s", err.Error())
		}
		return s3.New(sess), nil

	default:
		return nil, fmt.Errorf("unsupported storage provider type")
	}
}
