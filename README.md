# 🚀 Stremiarr

**Stremiarr** is a production-grade, self-hosted media streaming infrastructure stack. It is designed to provide a high-performance, resilient pipeline for torrent discovery and streaming, specifically optimized for **Real-Debrid** integration with a focus on **zero-stutter playback**.

Unlike simple standalone applications, Stremiarr orchestrates a suite of specialized services to handle the entire lifecycle of a stream—from discovery to secure proxying and aggressive caching.

---

## 🏗️ Architecture

The stack is orchestrated via **Docker Compose** and utilizes a "Host Networking" model for maximum throughput and low latency.

- **Caddy (Gateway)**: A high-performance reverse proxy that handles automatic TLS (Cloudflare DNS-01), streaming-optimized buffering, and security (HSTS, rate-limiting).
- **Rclone (The Muscle)**: Acts as the bridge between Real-Debrid and your player, featuring a **250GB VFS cache** with optimized read-ahead and chunking.
- **Handoff (The Brain)**: A custom-built Go service that provides stream aliasing, a background validation worker pool, and a zero-stutter buffer pool for high-efficiency proxying. **Supports multiple Real-Debrid tokens with automatic round-robin load balancing, advanced rate-limit handling, and auto-queuing of uncached torrents (with smart seeder-count thresholds).** Also includes an optimized **Library Prefetcher** to automatically discover and cache your Stremio library on Real-Debrid.
- **Comet & Postgres**: A multi-source scraper engine backed by a tuned PostgreSQL instance for high-concurrency metadata storage.

---

## 🛠️ Key Features

*   **Production-Ready CI/CD**: Includes over 20 management scripts for validating Compose files, Caddy syntax, upstream connectivity, and security.
*   **Infrastructure as Code**: The entire stack is portable, using relative pathing and environment variables.
*   **Drift Detection**: A snapshotting system allows you to baseline "known-good" configurations and detect/rollback unintended changes.
*   **Observability**: Integrated health-check scripts designed for 5-minute cron intervals with webhook alerting capabilities.
*   **Optimized Performance**: Pre-configured Postgres tuning, Go memory limits, and Rclone VFS flags specifically tailored for 4K streaming.

---

## 🚀 Quick Start

### 1. Prerequisites
- Docker & Docker Compose installed.
- A Cloudflare account and API Token (for DNS-01 TLS).
- A Real-Debrid account and API Key.

### 2. Configuration
Clone this repository and set up your environment:

```bash
cp compose/.env.example compose/.env
# Edit compose/.env with your secrets
nano compose/.env
```

Edit `caddy/Caddyfile` to replace `yourdomain.com` with your actual domain.
Edit `rclone/rclone.conf` with your Real-Debrid path/credentials.

### 3. Deployment

```bash
# Run CI checks (optional but recommended)
./scripts/ci.sh

# Deploy the stack
./scripts/deploy.sh
```

---

## 🛠️ Management

- **Status**: `./scripts/status.sh`
- **Logs**: `./scripts/logs.sh <service>`
- **Backup**: `./scripts/backup.sh`
- **Snapshot**: `./scripts/snapshot.sh` (Saves a "known-good" config state)
- **Health Watch**: `./scripts/health_watch.sh` (Can be set as a cron job)

---

## 📦 Service Overview

| Service | Purpose | Port |
| :--- | :--- | :--- |
| **Caddy** | Edge Proxy & TLS | 80/443 |
| **Handoff** | Stream Manager | 9944 |
| **Rclone** | Debrid VFS Cache | 9933 |
| **Comet** | Torrent Scraper | 8000 |
| **Postgres** | Database | 5432 |

---

## 🚀 Why Stremiarr?

Stremiarr is built for users who want a **"set it and forget it"** production environment. By decoupling the scraper (Comet) from the delivery mechanism (Rclone/Handoff), it ensures that even if one indexer is down, your streaming pipeline remains fast and stable. It is the ultimate "Arr" equivalent for those who prefer the speed of Debrid over local storage.

---

## ⚖️ License
MIT
