.PHONY: build test test-coverage lint vet fmt tidy docker clean all

BINARY_NAME=handoff
GO=go

all: fmt vet lint test build

build:
	CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BINARY_NAME) .

test:
	$(GO) test -v ./...

test-coverage:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

lint:
	golangci-lint run

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy

docker:
	docker build -t handoff:dev .

clean:
	rm -f $(BINARY_NAME) coverage.out coverage.html
