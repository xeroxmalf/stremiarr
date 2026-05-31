# 🛣️ Stremiarr Roadmap

This document outlines the planned feature development and architectural improvements for Stremiarr.

## 🚀 Phase 1: Reliability & Hardening (Current)
- [x] **Multi-Token Round-Robin**: Support multiple Real-Debrid API keys.
- [x] **Smart Token Health-Checking**: Automatically disable expired/invalid tokens and retry them periodically.
- [x] **Enhanced CI/CD**: Unit tests for the Go service and automated security scanning (Trivy).
- [x] **Portable Scripting**: Refactor management scripts to be project-relative and platform-agnostic.

## 🛠️ Phase 2: User Experience & Observability (Short-Term)
- [ ] **Interactive TUI Dashboard**: A command-line dashboard for real-time stream monitoring and container status.
- [ ] **Consolidated Logging**: Centralized log collection for all Docker services with better filtering.
- [ ] **Auto-Update System**: Optional background process to keep Docker images and Go service up-to-date.
- [ ] **Detailed Documentation**: Add a comprehensive troubleshooting guide and performance tuning wiki.

## 🏗️ Phase 3: Architectural Scaling (Medium-Term)
- [ ] **PostgreSQL Migration**: Move Handoff's SQLite metadata into the shared PostgreSQL instance for better concurrency.
- [ ] **Distributed Deployment**: Support for deploying components (e.g., discovery vs. streaming) on separate machines.
- [ ] **Plugin System**: Allow users to add custom scrapers or validation logic without modifying core code.
- [ ] **Stream Health Metrics**: Export Prometheus-compatible metrics for Grafana visualization.

## 🌟 Phase 4: Feature Expansion (Long-Term)
- [ ] **Web-Based Management UI**: A full-featured web dashboard for configuring aliases, sources, and monitoring health.
- [ ] **Multi-Debrid Support**: Add support for AllDebrid, Premiumize, and other providers.
- [ ] **Smart Pre-Caching**: Background worker to pre-cache popular streams based on trending data.
- [ ] **Transcoding Support**: Optional integration with FFmpeg/Jellyfin for on-the-fly transcoding of high-bitrate streams.

---

## 📈 Contribution
We welcome contributions! Please see the `CONTRIBUTING.md` (coming soon) for guidelines.
