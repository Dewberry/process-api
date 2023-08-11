# -------------------------------
FROM golang:1.19.12 AS dev

# RUN apt-get update && apt-get install -y docker.io && rm -rf /var/lib/apt/lists/*
# Hot-Reloader
RUN go install github.com/githubnemo/CompileDaemon@v1.4.0

COPY ./ /app
WORKDIR /app

RUN go build main.go

ENTRYPOINT CompileDaemon --build="go build main.go" --command="./main"
# -------------------------------

# -------------------------------
FROM debian:12.1-slim as prod

# RUN apk add --no-cache docker-cli

COPY --from=dev /app/main /usr/local/bin/main

ENTRYPOINT ["main"]
# -------------------------------
