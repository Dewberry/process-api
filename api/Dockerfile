# -------------------------------
FROM golang:1.19.12 AS dev

# RUN go install github.com/swaggo/swag/cmd/swag@v1.8.12

# Hot-Reloader
RUN go install github.com/githubnemo/CompileDaemon@v1.4.0

COPY ./ /app
WORKDIR /app

# RUN swag init
RUN go build main.go

ENTRYPOINT ["CompileDaemon", "--build=go build main.go", "--command=./main"]
# -------------------------------

# -------------------------------
FROM debian:12.1-slim as prod

COPY --from=dev /app/main /usr/local/bin/main

COPY --from=dev /app/plugins /plugins
COPY --from=dev /app/public /public
COPY --from=dev /app/views /views
COPY --from=dev /app/docs /docs

ENTRYPOINT ["main"]
# -------------------------------
