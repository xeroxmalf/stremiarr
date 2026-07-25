# 🚀 Stremiarr

![CI](https://github.com/xeroxmalf/stremiarr/actions/workflows/ci.yml/badge.svg)
![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)
![License](https://img.shields.io/badge/License-MIT-green)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker)

**Stremiarr** is a production-grade, self-hosted media streaming infrastructure stack. It is designed to provide a high-performance, resilient pipeline for torrent discovery and streaming, specifically optimized for **Real-Debrid** integration with a focus on **zero-stutter playback**.

Unlike simple standalone applications, Stremiarr orchestrates a suite of specialized services to handle the entire lifecycle of a stream—from discovery to secure proxying and aggressive caching.

---

## 🚀 Why Stremiarr?

Stremiarr is built for users who want a **"set it and forget it"** production environment. By decoupling the scraper (Comet) from the delivery mechanism (Rclone/Handoff), it ensures that even if one indexer is down, your streaming pipeline remains fast and stable. It is the ultimate "Arr" equivalent for those who prefer the speed of Debrid over local storage.

---

## 📋 Prerequisites

Before you begin, ensure you have the following installed:
- [Docker](https://docs.docker.com/get-docker/) (v20.10+)
- [Docker Compose](https://docs.docker.com/compose/install/) (v2.0+)
- [Go](https://go.dev/doc/install) (1.24+)
- `make` utility
- A Cloudflare account and API Token (for DNS-01 TLS).
- A Real-Debrid account and API Key.

---

## 🏗️ Architecture

The stack is orchestrated via **Docker Compose** and utilizes a "Host Networking" model for maximum throughput and low latency.

- **Caddy (Gateway)**: A high-performance reverse proxy that handles automatic TLS (Cloudflare DNS-01), streaming-optimized buffering, and security (HSTS, rate-limiting).
- **Rclone (The Muscle)**: Acts as the bridge between Real-Debrid and your player, featuring a **250GB VFS cache** with optimized read-ahead and chunking.
- **Handoff (The Brain)**: A custom-built Go service that provides stream aliasing, a background validation worker pool, and a zero-stutter buffer pool for high-efficiency proxying. **Supports multiple Real-Debrid tokens with automatic round-robin load balancing, advanced rate-limit handling, and auto-queuing of uncached torrents (with smart seeder-count thresholds).** Also includes an optimized **Library Prefetcher** to automatically discover and cache your Stremio library on Real-Debrid.
- **Comet & Postgres**: A multi-source scraper engine backed by a tuned PostgreSQL instance for high-concurrency metadata storage.
- **Zurg & Plex (Optional)**: Provides a native Plex Media Server integration by mounting the Real-Debrid cloud as a lightning-fast local filesystem via Zurg and an Rclone FUSE mount.

---

## 📁 Project Structure

```text
.
├── cmd/                # Main applications for this project
├── internal/           # Private application and library code
├── pkg/                # Library code that's ok to use by external applications
├── docker-compose.yml  # Docker Compose definition for the stack
├── Makefile            # Build and management tasks
├── helm/               # Helm charts for Kubernetes deployment
└── docs/               # Documentation
```

---

## 🛠️ Service Overview

| Service | Purpose | Port |
| :--- | :--- | :--- |
| **Caddy** | Edge Proxy & TLS | 80/443 |
| **Handoff** | Stream Manager | 9944 |
| **Rclone** | Debrid VFS Cache | 9933 |
| **Comet** | Torrent Scraper | 8000 |
| **Postgres** | Database | 5432 |
| **Zurg** | Debrid WebDAV | 9999 |
| **Plex** | Media Server | 32400 |

---

## 🚀 Make Commands

The project includes a Makefile with several helpful commands:

- `make build`: Build the Go binaries
- `make test`: Run all tests
- `make run`: Run the Handoff service locally
- `make docker-build`: Build Docker images
- `make docker-up`: Start the complete stack using Docker Compose
- `make docker-down`: Stop and remove the Docker stack
- `make lint`: Run golangci-lint

---

## 👨‍💻 Development

If you want to contribute to Stremiarr, please start by reading our [CONTRIBUTING.md](./CONTRIBUTING.md).

For local development:
1. Clone the repository
2. Run `make build` to verify the code compiles
3. Run `make test` to ensure tests pass
4. Use `make docker-up` to bring up the supporting services

---

## 📄 Further Documentation

- [CONTRIBUTING.md](./CONTRIBUTING.md) - Guidelines for contributing
- [CHANGELOG.md](./CHANGELOG.md) - Version history and updates
- [ROADMAP.md](./ROADMAP.md) - Future plans and features
- [SECURITY.md](./SECURITY.md) - Security policies and reporting

---

## ⚖️ License

MIT
