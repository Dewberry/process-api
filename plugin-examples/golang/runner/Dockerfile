# Build go container
FROM golang:alpine3.18 as gobuilder

COPY ./ /app
WORKDIR /app

#  Hot reloader for convenience
RUN go install github.com/githubnemo/CompileDaemon@v1.4.0
RUN go build main.go
ENTRYPOINT CompileDaemon --build="go build main.go" --command="./main"

# Build prod container
FROM alpine:3.18
WORKDIR /app
COPY --from=gobuilder /app/main /app