# process-api
A lightweight, extensible, OGC compliant Process API for cloud based processing

![](/docs/swagger-screenshot.png)
*created using [swaggo](https://github.com/swaggo/swag)*

## Processes

Example processes are provided in [plugins/](plugins/) directory.

__synchronous__ jobs are run using the Docker CLI locally on the host.

__asynchronous__ jobs are run remotely using AWS Batch. To test/utilize this feature an AWS account is required with batch job definitions, queues, resources, etc. set up in AWS prior to being called by this api.


---

## Build and run

1. Create a `.env` file (example below)
2. Add process configuration files (yaml) to the [plugins](plugins/) directory
3. Update swagger documents and compile the server: `swag init && go build main.go`.
4. Run the server: `./main`, with the following available flags:
   ```
      `-c [type string] specify the path of the max cache size for storing jobs (default "11073741824" (1GB))`
      `-d [type string] specify the path of the processes directory to load (default "plugins" assuming program called from inside repo)`
      `-e [type string] specify the path of the dot env file to load (default ".env")`
      `-p [type string] specify the port to run the api on (default "5050")`
   ```


Once the server is up and running, go to http://localhost:5050/swagger/ for documentation details.

For more information on the specification visit the [OGC API - Processes - Part 1: Core](https://docs.ogc.org/is/18-062r2/18-062r2.html#toc0).

The API responses follow the examples provided here:
https://developer.ogc.org/api/processes/index.html

---

## Behavior

If the client cancels the processing request for any reason (timeout, etc.), the syncrhonous jobs (local) will be abandoned, killed and removed. If the API server is shut down, all currently active jobs will be abandoned, killed and removed.

- For synchronous (local docker jobs), that means a "KILL" signal will be sent to the running container, and it will be removed.
- For asynchronous (AWS Batch jobs), no termination behavior is in place and jobs must be cancelled using the BATCH API directly.
- For the server, a `dump.json` file is created to record the final state of all jobs in the JobsCache.


## Example .env file

For AWS services, an env file should be located at the root of this repository (`./.env`) and be formatted like so:

```bash
AWS_ACCESS_KEY_ID='ASDFASDFEXAMPLE'
AWS_SECRET_ACCESS_KEY='ASDFASDFASDFASDFASDFEXAMPLE'
AWS_DEFAULT_REGION='us-east-1'

AWS_ACCESS_KEY_ID='************'
AWS_SECRET_ACCESS_KEY='**************************'
AWS_DEFAULT_REGION='us-east-1'
S3_BUCKET='********'
S3_RESULTS_DIR='results'
S3_LOGS_DIR='logs'

# BATCH
BATCH_LOG_STREAM_GROUP='/aws/batch/job'

# GDAL
CPL_VSIL_USE_TEMP_FILE_FOR_RANDOM_WRITE='YES'
```

*NOTE: This server was adapted for ogc-compliance from an existing api developed by @albrazeau*