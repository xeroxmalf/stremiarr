# 🛣️ Stremiarr Roadmap

This document outlines the planned feature development and architectural improvements for Stremiarr.

## 🚀 Phase 1: Reliability & Hardening (Current)
- [x] **Multi-Token Round-Robin**: Support multiple Real-Debrid API keys.
- [x] **Smart Token Health-Checking**: Automatically disable expired/invalid tokens and retry them periodically.
- [x] **Enhanced CI/CD**: Unit tests for the Go service and automated security scanning (Trivy).
- [x] **Portable Scripting**: Refactor management scripts to be project-relative and platform-agnostic.
- [x] **Advanced Rate Limiting**: Exponential backoff wrapper (`rdDo`) to gracefully handle Real-Debrid API 429 limits without crashing.

## 🛠️ Phase 2: User Experience & Observability (Short-Term)
- [x] **Interactive TUI Dashboard**: A command-line dashboard for real-time stream monitoring and container status.
- [x] **Consolidated Logging**: Centralized log collection for all Docker services with better filtering.
- [ ] **Auto-Update System**: Optional background process to keep Docker images and Go service up-to-date.
- [ ] **Detailed Documentation**: Add a comprehensive troubleshooting guide and performance tuning wiki.

## 🏗️ Phase 3: Architectural Scaling (Medium-Term)
- [ ] **PostgreSQL Migration**: Move Handoff's SQLite metadata into the shared PostgreSQL instance for better concurrency.
- [ ] **Distributed Deployment**: Support for deploying components (e.g., discovery vs. streaming) on separate machines.
- [ ] **Plugin System**: Allow users to add custom scrapers or validation logic without modifying core code.
- [ ] **Stream Health Metrics**: Export Prometheus-compatible metrics for Grafana visualization.

## 🌟 Phase 4: Feature Expansion (Long-Term)
- [x] **Web-Based Management UI**: A full-featured web dashboard for configuring aliases, sources, and monitoring health.
- [ ] **Multi-Debrid Support**: Add support for AllDebrid, Premiumize, and other providers.
- [x] **Smart Pre-Caching**: Background library prefetch worker that intelligently queues uncached streams to Real-Debrid with seeder thresholds and persistent history tracking.
- [x] **Plex Media Server Integration**: Support adding Real-Debrid as a native local library via Zurg and Rclone FUSE mounts.
- [ ] **Transcoding Support**: Optional integration with FFmpeg/Jellyfin for on-the-fly transcoding of high-bitrate streams.

---

## 📈 Contribution
We welcome contributions! Please see the `CONTRIBUTING.md` (coming soon) for guidelines.
