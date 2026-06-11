FROM golang:1.24-alpine AS builder
WORKDIR /app


# Copy go.mod and go.sum first to leverage Docker cache for dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ui.html .

# Build the application statically without CGO
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o handoff .

FROM alpine:latest
WORKDIR /app

# Add tzdata
RUN apk add --no-cache tzdata

COPY --from=builder /app/handoff .

# The application uses /data for the streams.db and keys.json
VOLUME ["/data"]

ENTRYPOINT ["/app/handoff"]