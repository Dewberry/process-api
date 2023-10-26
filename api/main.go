package main

import (
	_ "app/docs"
	"app/handlers"
	"fmt"

	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/natefinch/lumberjack"
	log "github.com/sirupsen/logrus"

	echoSwagger "github.com/swaggo/echo-swagger"
)

var (
	envFP      string
	pluginsDir string
	dbPath     string
	port       string
	logLevel   string
	logFile    string
)

func init() {
	// The order of precedence as Flag > Environment variable > Default value
	flag.StringVar(&envFP, "e", "", "specify the path of the dot env file to load")
	flag.Parse()

	if envFP != "" {
		err := godotenv.Load(envFP)
		if err != nil {
			log.Fatalf("could not read environment file: %s", err.Error())
		}
	}

	flag.StringVar(&pluginsDir, "d", getDefault("PLUGINS_DIR", "plugins"), "specify the relative path of the processes dir")
	flag.StringVar(&port, "p", getDefault("API_PORT", "5050"), "specify the port to run the api on")
	flag.StringVar(&dbPath, "db", getDefault("DB_PATH", "/.data/db.sqlite"), "specify the path of the sqlite database")
	flag.StringVar(&logLevel, "ll", getDefault("LOG_LEVEL", "Info"), "specify the logging level")
	flag.StringVar(&logFile, "lf", getDefault("LOG_FILE", "/.data/logs/api.log"), "specify the log file")

	flag.Parse()
}

// Checks if there's an environment variable for this configuration,
// if yes, return the env value, if not, return the default value.
func getDefault(envVar string, defaultValue string) string {
	if value, exists := os.LookupEnv(envVar); exists {
		return value
	}
	return defaultValue
}

// Init logrus logging, returning, lvl and rotating log writer
// that can be used for middleware logging
func initLogger() (log.Level, *lumberjack.Logger) {

	// Set up lumberjack as a logger
	logWriter := &lumberjack.Logger{
		Filename: logFile, // File output location
		MaxSize:  10,      // Maximum file size before rotation (in megabytes)
		MaxAge:   60,      // Maximum number of days to retain old log files
		Compress: true,    // Whether to compress the rotated files
	}

	log.SetOutput(logWriter)
	lvl, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Warnf("Invalid LOG_LEVEL set: %s; defaulting to INFO", logLevel)
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
	// Set formatter to JSON
	log.SetFormatter(&log.JSONFormatter{})

	// Enable logging the calling method
	log.SetReportCaller(true)

	return lvl, logWriter
}

// @title Process-API Server
// @version dev-8.16.23
// @description An OGC compliant process server.

// @contact.name Seth Lawler
// @contact.email slawler@dewberry.com

// @host localhost:5050
// @BasePath /
// @schemes http

// @externalDocs.description  OGC Process API
// @externalDocs.url          https://docs.ogc.org/is/18-062r2/18-062r2.html#toc0

// @externalDocs.description   Schemas
// @externalDocs.url    http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/
func main() {
	_, logWriter := initLogger()
	fmt.Println("Logging to", logFile)

	// Initialize resources
	rh := handlers.NewRESTHander(pluginsDir, dbPath)
	// todo: handle this error: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running
	// todo: all non terminated job statuses should be updated to unknown
	// todo: all logs in the logs directory should be moved to storage

	// Goroutines
	go rh.StatusUpdateRoutine()
	go rh.JobCompletionRoutine()

	// Set server configuration
	e := echo.New()
	e.Static("/public", "public")

	// e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Output: logWriter,
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowCredentials: true,
		AllowOrigins:     []string{"*"},
	}))
	e.Renderer = &rh.T

	// Server
	e.GET("/", rh.LandingPage)
	e.GET("/swagger/*", echoSwagger.WrapHandler)
	e.GET("/conformance", rh.Conformance)

	// Processes
	e.GET("/processes", rh.ProcessListHandler)
	e.GET("/processes/:processID", rh.ProcessDescribeHandler)
	e.POST("/processes/:processID/execution", rh.Execution)

	// TODO
	// e.Post("processes/:processID/new, rh.RegisterNewProcess)
	// e.Delete("processes/:processID", rh.RegisterNewProcess)

	// Jobs
	e.GET("/jobs", rh.ListJobsHandler)
	e.GET("/jobs/:jobID", rh.JobStatusHandler)
	e.GET("/jobs/:jobID/results", rh.JobResultsHandler)
	e.GET("/jobs/:jobID/logs", rh.JobLogsHandler)
	e.GET("/jobs/:jobID/metadata", rh.JobMetaDataHandler)
	e.DELETE("/jobs/:jobID", rh.JobDismissHandler)

	// Callbacks
	e.PUT("/jobs/:jobID/status", rh.JobStatusUpdateHandler)
	// e.POST("/jobs/:jobID/results", rh.JobResultsUpdateHandler)

	// Start server
	go func() {
		log.Info("server starting on port: ", port)
		if err := e.Start(":" + port); err != nil && err != http.ErrServerClosed {
			log.Error("server error : ", err.Error())
			log.Fatal("shutting down the server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 10 seconds.
	// Use a buffered channel to avoid missing signals as recommended for signal.Notify
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	<-quit
	log.Info("gracefully shutting down the server")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			log.Error(err)
		}
	}()

	// Shutdown the server
	// By default, Docker provides a grace period of 10 seconds with the docker stop command.

	// Kill any running docker containers (clean up resources)
	rh.ActiveJobs.KillAll()
	log.Info("kill command sent to all active jobs")

	// sleep so that Close() routines spawned by KillAll() can finish writing logs, and updating statuses
	// aws batch jobs close() methods take minimum of 5 seconds
	time.Sleep(5 * time.Second)

	if err := rh.DB.Handle.Close(); err != nil {
		log.Error(err)
	} else {
		log.Info("closed connection to database")
	}

	time.Sleep(4 * time.Second)

	log.Info("server gracefully shutdown")

}
