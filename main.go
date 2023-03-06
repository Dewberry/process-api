package main

import (
	_ "app/docs"
	"app/jobs"

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
	cacheSize    string // default 1028*1028*1028 = 11073741824 (1GB) ~500K jobs
	port         string
	envFP        string
)

func init() {

	flag.StringVar(&processesDir, "d", "plugins", "specify the relative path of the processes dir")
	flag.StringVar(&port, "p", "5050", "specify the port to run the api on")
	flag.StringVar(&cacheSize, "c", "11073741824", "specify the max cache size (default= 1GB)")
	flag.StringVar(&envFP, "e", ".env", "specify the path of the dot env file to load")

	flag.Parse()

	err := godotenv.Load(envFP)
	if err != nil {
		log.Warnf("no .env file is being used: %s", err.Error())
	}
}

// @title Process-API Server
// @version dev-3.5.23
// @description An OGC compliant(ish) process server.

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

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowCredentials: true,
		AllowOrigins:     []string{"*"},
	}))

	e.Logger.SetLevel(log.INFO)

	maxCacheSize, err := strconv.Atoi(cacheSize)
	if err != nil {
		log.Fatal()
	}
	rh, err := jobs.NewRESTHander(processesDir, uint64(maxCacheSize))
	if err != nil {
		e.Logger.Fatal(err)
	}

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
	e.GET("/jobs", rh.JobsCacheHandler)
	e.GET("/jobs/:jobID", rh.JobStatusHandler)
	e.GET("/jobs/:jobID/results", rh.JobResultsHandler) //requires cache
	e.DELETE("/jobs/:jobID", rh.JobDismissHandler)

	// JobCache Monitor
	go func() {
		for {
			_ = rh.JobsCache.CheckCache()
			time.Sleep(60 * 60 * time.Second) // check cache once an hour
		}
	}()

	// Start server
	go func() {
		e.Logger.Info("server starting on port: ", port)
		if err := e.Start(":" + port); err != nil && err != http.ErrServerClosed {
			log.Error("server error : ", err)
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
	// Dump cache to file
	rh.JobsCache.DumpCacheToFile("dump.json")
	err = rh.JobsCache.KillAll()
	if err != nil {
		e.Logger.Error(err)
	}
	e.Logger.Info("killed and removed active containers")

	// shutdown the server
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}

}
