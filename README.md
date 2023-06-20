# process-api

[![E2E Tests](https://github.com/dewberry/process-api/actions/workflows/e2e-tests.yml/badge.svg?event=push)](https://github.com/Dewberry/process-api/actions/workflows/e2e-tests.yml)

A lightweight, extensible, OGC compliant Process API for local and cloud based containerized processing.

![](/docs/swagger-screenshot.png)
*created using [swaggo](https://github.com/swaggo/swag)*

For more information on the specification visit the [OGC API - Processes - Part 1: Core](https://docs.ogc.org/is/18-062r2/18-062r2.html#toc0).

The API responses follow the examples provided here:
https://developer.ogc.org/api/processes/index.html

---

## Build and run

1. Create a `.env` file (example below)
2. Add process configuration files (yaml) to the [plugins](plugins/) directory or use -d flag to specify path of the directory with process configuration files
3. Update swagger documents and compile the server: `swag init && go build main.go`.
4. Run the server: `./main`, with the following available flags:
   ```
      `-d [type string] specify the path of the processes directory to load (default "plugins" assuming program called from inside repo)`
      `-e [type string] specify the path of the dot env file to load (default ".env")`
      `-p [type string] specify the port to run the api on (default "5050")`
   ```


Once the server is up and running, go to http://localhost:5050/swagger/ for documentation details.

---

## System Components

The system design consists of four major system components:

### API
The API is the main orchestrator for all the downstream functionality and a single point of communication with the system.

### Processes
Processes are computational tasks described through a configuration file that can be executed in a container. Each configuration file contains information about the process such as the title of this process, its description, execution mode, execution resources, secrets required, inputs, and outputs. Each config file is to be unmarshalled to register a process in the API. These processes then can be called several times by the client application to perform jobs.

### Execution Platforms
Execution platforms are hosts that can provide resources to run a job using configuration defined in the process and with arguments supplied by the client request. The execution platforms can be on the cloud or a local machine.

### Jobs
Each execution of a process is called a job. A job can be synchronous or asynchronous depending on which platform it is being executed upon. Synchronous jobs return responses after the job has reached a finished state, meaning either successful or failed. The asynchronous jobs return a response immediately with a job id for the client so that the client can monitor the jobs.

*Note on Procesess: The developers must make sure they choose the right platform to execute a process. The processes that are short-lived and fast and do not create a file resource as an output, for example getting the water surface elevation values for a coordinate from cloud raster, must be registered to run on the local machine so that they are synchronous. These kinds of processes should output data in JSON format.*

*On the other hand, processes that take a long time to execute and their results are files, for example clipping a raster, must be registered to run on the cloud so that they are asynchronous. These processes should contain links to file resources in their results.*


## Behaviour

![](/design.svg)

At the start of the app, all the `.yaml` `.yml` (configuration) files are read and processes are registered. Each file describes what resources the process requires and where it wants to be executed. There are two execution platforms available; local processes run in a docker container, hence they must specify a docker image and the tag. The API will download these images from the repository and then run them on the host machine. Commands specified will be appended to the entrypoint of the container. The API responds to the request of local processes synchronously.

Cloud processes are executed on the cloud using a workload management service. AWS Batch was chosen as the provider for its wide user base. Cloud processes must specify the provider type, job definition, job queue, and job name. The API will submit a request to run the job to the AWS Batch API directly.

The containerized processes must expect a JSON load as the last argument of the entrypoint command and write results to S3 using the supplied variables in the JSON. It is the responsibility of the process to write these results correctly if the process succeeds. The API will try to fetch these results from the expected location when the client requests results for jobs.

When a job is submitted, a local container is fired up immediately for sync jobs, and a job request is submitted to the AWS batch for async jobs. When a local job reaches a finished state (successful or failed), the local container is removed. Similarly, if an active job is explicitly dismissed using DEL route or the HTTP connection drops, the job is terminated, and resources are freed up. If the server is gracefully shut down, all currently active jobs are terminated, and resources are freed up.

The API responds to all GET requests (except `/jobs/<jobID>/results`) as HTML or JSON depending upon if the request is being originated from Browser or not or if it specifies the format using query parameter ‘f’.

## Example .env file

For AWS services, an env file should be located at the root of this repository (`./.env`) and be formatted like so:

```properties
# AWS
AWS_ACCESS_KEY_ID='************'
AWS_SECRET_ACCESS_KEY='**************************'
AWS_DEFAULT_REGION='us-east-1'

# S3
S3_BUCKET='********'
S3_RESULTS_DIR='results'
S3_LOGS_DIR='logs'
S3_META_DIR='metadata'

# BATCH
BATCH_LOG_STREAM_GROUP='/aws/batch/job'

# GDAL
CPL_VSIL_USE_TEMP_FILE_FOR_RANDOM_WRITE='YES'

# Policies
EXPIRY_DAYS='7'
```

## Notes
*NOTE: This server was adapted for ogc-compliance from an existing api developed by @albrazeau*