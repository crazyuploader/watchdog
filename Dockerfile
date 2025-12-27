# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o watchdog ./cmd

# ---

FROM alpine:3.23

WORKDIR /app

COPY --from=builder /app/watchdog /app/watchdog

# Add a non-root user for security
RUN adduser -D -u 10001 appuser
USER appuser

ENTRYPOINT ["/app/watchdog"]
