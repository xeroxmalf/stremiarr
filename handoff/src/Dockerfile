FROM golang:1.24-alpine AS builder
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go.mod and go.sum first to leverage Docker cache for dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and embedded HTML templates
COPY *.go *.html .

# Build the application statically without CGO
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -extldflags '-static'" -o handoff .

FROM alpine:latest
WORKDIR /app

LABEL org.opencontainers.image.source="https://github.com/stremiarr/stremiarr"
LABEL org.opencontainers.image.description="Stremiarr Handoff"

# Add runtime dependencies
RUN apk add --no-cache tzdata ca-certificates wget

# Create non-root user
RUN addgroup -g 1000 handoff && adduser -u 1000 -G handoff -s /bin/sh -D handoff

# Copy binary from builder
COPY --from=builder /app/handoff .

# The application uses /data for streams.db and keys.json
VOLUME ["/data"]

# Switch to non-root user
USER handoff

EXPOSE 9944

ENTRYPOINT ["/app/handoff"]
