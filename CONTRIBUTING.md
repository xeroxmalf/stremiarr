# Contributing to Stremiarr

First off, thank you for considering contributing to Stremiarr!

## Prerequisites
- Go 1.24+
- Docker
- Docker Compose
- ShellCheck

## Repository Structure Overview
- `caddy/`: Reverse proxy configurations.
- `comet/`: Metadata and scraper components.
- `compose/`: Docker compose configuration files.
- `docs/`: Project documentation.
- `handoff/`: Core Go application for handling streams.
- `helm/`: Helm charts for Kubernetes deployments.
- `scripts/`: Utility and setup scripts.

## Development Setup
1. Clone the repository: `git clone <repo-url> && cd Stremiarr`
2. Setup environment variables by copying the example file: `cp .env.example .env`
3. Build Handoff: `cd handoff && go build -o handoff ./src`

## Running the Stack Locally
Use Docker Compose to run the stack:
```bash
docker compose up -d
```

## Running Tests
Run the Go test suite inside the `handoff/src` directory:
```bash
cd handoff/src && go test -v ./...
```

## Running Linters
We enforce code quality using linters:
- Go code: `golangci-lint run`
- Shell scripts: `shellcheck scripts/*.sh`

## Running CI Locally
You can simulate the CI process locally by running:
```bash
./scripts/ci.sh
```

## Code Style Guidelines
- **Go formatting**: Use the standard `gofmt` to format all Go code.
- **Error handling**: Use `log.Printf` or structured logging patterns as established in the project.

## Commit Message Conventions
We follow conventional commits to automatically generate changelogs. Please use the following prefixes:
- `feat`: A new feature
- `fix`: A bug fix
- `docs`: Documentation only changes
- `chore`: Routine tasks, maintenance, or dependency updates
- `refactor`: Code changes that neither fix a bug nor add a feature

## Pull Request Process
1. Fork the repository and create your branch from `main`.
2. Ensure your code passes all tests and linters.
3. Open a Pull Request detailing the changes, reasoning, and any testing performed.

## Script Conventions
All shell scripts must use strict error handling and define the `PROJECT` path pattern:
```bash
#!/bin/bash
set -euo pipefail
```
