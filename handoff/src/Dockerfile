FROM golang:1.23-alpine AS builder
WORKDIR /app

# Install build dependencies for CGO (required by sqlite3)
RUN apk add --no-cache gcc musl-dev

# Copy source code
COPY main.go ui.html .

# Initialize go modules and fetch dependencies
RUN go mod init handoff && \
    go get github.com/mattn/go-sqlite3 && \
    go mod tidy

# Build the application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -o handoff main.go

FROM alpine:latest
WORKDIR /app

# Add tzdata (often useful for time functions in Go)
RUN apk add --no-cache tzdata

COPY --from=builder /app/handoff .

# The application uses /data for the streams.db and mappings.json, 
# so we create a volume mount point
VOLUME ["/data"]

ENTRYPOINT ["/app/handoff"]