#!/bin/bash
docker build . -t lawlerseth/runner:v0.0.1
# docker push lawlerseth/runner:v0.0.1

# Test locally with minio running
docker run -it --rm --env-file .env lawlerseth/runner:v0.0.1 ./main '{"jobID":"asdfw-qewfsdaerg-3q", "prefix":"demo/payload-example.json"}'