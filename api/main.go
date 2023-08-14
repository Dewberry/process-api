package main

import (
	_ "app/docs"
	"app/handlers"

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
	"github.com/labstack/gommon/log"

	echoSwagger "github.com/swaggo/echo-swagger"
)

var (
	pluginsDir string
	dbPath     string
	port       string
	envFP      string
)

func init() {

	flag.StringVar(&pluginsDir, "d", "plugins", "specify the relative path of the processes dir")
	flag.StringVar(&port, "p", "5050", "specify the port to run the api on")
	flag.StringVar(&envFP, "e", "../.env", "specify the path of the dot env file to load")
	flag.StringVar(&dbPath, "db", "../.data/db.sqlite", "specify the path of the sqlite database")

	flag.Parse()

	err := godotenv.Load(envFP)
	if err != nil {
		log.Warnf("no .env file is being used: %s", err.Error())
	}
}

// @title Process-API Server
// @version dev-4.19.23
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

	// Initialize resources
	rh := handlers.NewRESTHander(pluginsDir, dbPath)

	// todo: handle this error: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running

	// Set server configuration
	e := echo.New()
	e.Static("/public", "public")

	// e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Output: e.Logger.Output(),
	}))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowCredentials: true,
		AllowOrigins:     []string{"*"},
	}))
	e.Logger.SetLevel(log.DEBUG)
	log.SetLevel(log.INFO)
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

	// Start server
	go func() {
		e.Logger.Info("server starting on port: ", port)
		if err := e.Start(":" + port); err != nil && err != http.ErrServerClosed {
			log.Error("server error : ", err.Error())
			e.Logger.Fatal("shutting down the server")
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 10 seconds.
	// Use a buffered channel to avoid missing signals as recommended for signal.Notify
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	e.Logger.Info("gracefully shutting down the server")

	// Kill any running docker containers (clean up resources)

	if err := rh.ActiveJobs.KillAll(); err != nil {
		e.Logger.Error(err)
	} else {
		e.Logger.Info("killed and removed active containers")
	}

	time.Sleep(15 * time.Second) // sleep so that routines spawned by KillAll() can finish, using 15 seconds because AWS batch monitors jobs every 10 seconds
	if err := rh.DB.Handle.Close(); err != nil {
		e.Logger.Fatal(err)
	} else {
		e.Logger.Info("closed connection to database")
	}

	// Shutdown the server
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}
	e.Logger.Info("server gracefully shutdown")
}
