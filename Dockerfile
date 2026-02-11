# syntax=docker/dockerfile:1

FROM golang:1.25.3-alpine AS builder

WORKDIR /src

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/dujiao-api ./cmd/server

FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata \
    && adduser -D -g '' appuser \
    && mkdir -p /app/db /app/uploads /app/logs \
    && chown -R appuser:appuser /app

COPY --from=builder /out/dujiao-api /app/dujiao-api
COPY config.yml.example /app/config.yml.example

EXPOSE 8080

USER appuser

CMD ["./dujiao-api"]
