# syntax=docker/dockerfile:1

# Alpine is chosen for its small footprint
# compared to Ubuntu
FROM golang:1.17-alpine

# Install libvips and necessary dependencies
RUN set -x -o pipefail \
    && apk update \
    && apk upgrade \
    && apk add \
    build-base \
    vips-dev \
    && rm -rf /var/cache/apk/*

WORKDIR /app

COPY go.mod .
COPY go.sum .

# Download necessary Go modules
RUN go mod download

# COPY the source code as the last step
COPY . .

# Build
RUN go build main.go

EXPOSE 8080

CMD [ "./main" ]

