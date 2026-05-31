# 🚀 Stremiarr

Stremiarr is a pre-configured, production-grade media streaming stack. It integrates torrent discovery, Real-Debrid unrestrict services, and optimized stream proxying into a single, easy-to-deploy package.

## 🏗️ Architecture

- **Caddy**: Reverse proxy with automatic TLS (Cloudflare DNS) and streaming-optimized buffers.
- **Rclone**: High-speed WebDAV/HTTP interface for Real-Debrid with aggressive VFS caching (250GB).
- **Handoff (Go)**: A custom stream management service that provides secure aliases, stream validation, and proxying.
- **Comet**: Torrent discovery and scraping engine.
- **Postgres**: Persistent storage for Comet.

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

## 🛠️ Management

- **Status**: `./scripts/status.sh`
- **Logs**: `./scripts/logs.sh <service>`
- **Backup**: `./scripts/backup.sh`
- **Snapshot**: `./scripts/snapshot.sh` (Saves a "known-good" config state)
- **Health Watch**: `./scripts/health_watch.sh` (Can be set as a cron job)

## ⚖️ License
MIT (or your preferred license)
