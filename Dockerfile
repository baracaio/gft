# Multi-stage build that produces the Lambda zip.
# Usage (requires Docker BuildKit):
#   docker build --output build/ --build-arg GOARCH=amd64 .
# Or via Makefile:
#   make build-docker

FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG GOARCH=amd64
RUN GOOS=linux GOARCH=${GOARCH} CGO_ENABLED=0 \
    go build -tags lambda.norpc -ldflags="-s -w" \
    -o /bootstrap ./cmd/lambda

FROM alpine:3.21 AS packager
RUN apk add --no-cache zip
COPY --from=builder /bootstrap /bootstrap
RUN mkdir /out && zip -j /out/lambda.zip /bootstrap

# Final scratch stage — only the zip file.
# `docker build --output build/` extracts it to build/lambda.zip.
FROM scratch
COPY --from=packager /out/lambda.zip /lambda.zip
