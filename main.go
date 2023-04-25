package main

import (
	"app/config"
	_ "app/docs"
	"app/handlers"

	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"

	echoSwagger "github.com/swaggo/echo-swagger"
)

var (
	processesDir string
	cacheSize    int // default 1028*1028*1028 = 11073741824 (1GB) ~500K jobs
	port         string
	envFP        string
)

func init() {

	var cacheSizeString string
	flag.StringVar(&processesDir, "d", "plugins", "specify the relative path of the processes dir")
	flag.StringVar(&port, "p", "5050", "specify the port to run the api on")
	flag.StringVar(&cacheSizeString, "c", "11073741824", "specify the max cache size in bytes (default= 1GB)")
	flag.StringVar(&envFP, "e", ".env", "specify the path of the dot env file to load")

	flag.Parse()

	err := godotenv.Load(envFP)
	if err != nil {
		log.Warnf("no .env file is being used: %s", err.Error())
	}

	cacheSize, err = strconv.Atoi(cacheSizeString)
	if err != nil {
		log.Fatal()
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
	ac, err := config.Init(processesDir, uint64(cacheSize))
	if err != nil {
		log.Fatal(err)
	}

	// todo: handle this error: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running

	// Set server configuration
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowCredentials: true,
		AllowOrigins:     []string{"*"},
	}))
	e.Logger.SetLevel(log.DEBUG)
	log.SetLevel(log.INFO)
	e.Renderer = &ac.T

	// Server
	e.GET("/", handlers.LandingPage(ac.Title, ac.Description))
	e.GET("/swagger/*", echoSwagger.WrapHandler)
	e.GET("/conformance", handlers.Conformance(ac.ConformsTo))

	// Processes
	e.GET("/processes", handlers.ProcessListHandler(ac.ProcessList))
	e.GET("/processes/:processID", handlers.ProcessDescribeHandler(ac.ProcessList))
	e.POST("/processes/:processID/execution", handlers.Execution(ac.ProcessList, ac.JobsCache, ac.S3Svc))

	// TODO
	// e.Post("processes/:processID/new, handlers.RegisterNewProcess)
	// e.Delete("processes/:processID", handlers.RegisterNewProcess)

	// Jobs
	e.GET("/jobs", handlers.JobsCacheHandler(ac.JobsCache))
	e.GET("/jobs/:jobID", handlers.JobStatusHandler(ac.JobsCache))
	e.GET("/jobs/:jobID/results", handlers.JobResultsHandler(ac.JobsCache, ac.S3Svc)) //requires cache
	e.GET("/jobs/:jobID/logs", handlers.JobLogsHandler(ac.JobsCache))                 //requires cache
	e.DELETE("/jobs/:jobID", handlers.JobDismissHandler(ac.JobsCache))

	// JobCache Monitor
	go func() {
		for {
			time.Sleep(60 * 60 * time.Second) // check cache once an hour
			_ = ac.JobsCache.CheckCache()
		}
	}()

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
	err = ac.JobsCache.KillAll()
	if err != nil {
		e.Logger.Error(err)
	}
	e.Logger.Info("killed and removed active containers")

	// Dump cache to file
	err = ac.JobsCache.DumpCacheToFile()
	if err != nil {
		e.Logger.Error(err)
	}
	e.Logger.Info("snapshot created at .data/snapshot.gob")

	// Shutdown the server
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}
	e.Logger.Info("server gracefully shutdown")
}
