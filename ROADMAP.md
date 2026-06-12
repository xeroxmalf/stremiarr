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
- [x] **Auto-Update System**: Optional background process to keep Docker images and Go service up-to-date.
- [x] **Detailed Documentation**: Add a comprehensive troubleshooting guide and performance tuning wiki.

## 🏗️ Phase 3: Architectural Scaling (Medium-Term)
- [x] **PostgreSQL Migration**: Move Handoff's SQLite metadata into the shared PostgreSQL instance for better concurrency.
- [x] **Distributed Deployment**: Support for deploying components (e.g., discovery vs. streaming) on separate machines.
- [x] **Plugin System**: Allow users to add custom scrapers or validation logic without modifying core code.
- [x] **Stream Health Metrics**: Export Prometheus-compatible metrics for Grafana visualization.

## 🌟 Phase 4: Feature Expansion (Long-Term)
- [x] **Web-Based Management UI**: A full-featured web dashboard for configuring aliases, sources, and monitoring health.
- [x] **Multi-Debrid Support**: Add support for AllDebrid, Premiumize, and other providers.
- [x] **Smart Pre-Caching**: Background library prefetch worker that intelligently queues uncached streams to Real-Debrid with seeder thresholds and persistent history tracking.
- [x] **Plex Media Server Integration**: Support adding Real-Debrid as a native local library via Zurg and Rclone FUSE mounts.
- [x] **Transcoding Support**: Optional integration via Plex Media Server compose stack for on-the-fly transcoding of high-bitrate streams.

## 🧠 Phase 5: Advanced Media Management & Scraping (Future)
- [ ] **Custom Scraper DSL**: A domain-specific language for users to define their own scrapers for niche torrent sites without writing Go code.
- [ ] **AI-Powered Torrent Parsing**: Machine learning model to parse ambiguous torrent names and map them to IMDB/TMDB IDs reliably.
- [ ] **Debrid Link Failover**: If a stream fails on Real-Debrid, automatically fallback to another configured debrid provider transparently.
- [ ] **Subtitle Synchronization Pipeline**: Auto-download, sync, and bake-in or mux subtitles (OpenSubtitles, subscene) on the fly for cached streams.
- [ ] **Metadata Augmentation**: Enhance Stremio results with Rotten Tomatoes scores, Trakt.tv integration, and custom review sources.
- [ ] **Content Blacklisting & Whitelisting**: Strict filters to ignore specific release groups, codecs (e.g. ignoring AV1 if hardware doesn't support it), or hard-coded subs.
- [ ] **Anime-Specific Metadata Handling**: Better mapping of absolute anime episode numbers to standard seasonal formats using AniDB integration.

## ⚙️ Phase 6: Infrastructure & Performance Scaling (Future)
- [ ] **Redis-Backed Distributed Caching**: Replace in-memory caches with a centralized Redis cluster for multi-node deployments.
- [ ] **Kubernetes Helm Charts**: Official Helm charts for deploying the entire Stremiarr ecosystem on K8s with auto-scaling.
- [ ] **Anycast CDN Routing**: Deploy edge nodes globally to terminate Stremio connections closer to the user and route traffic over the fastest backbone to the core instance.
- [ ] **Zero-Trust Network Architecture**: Integrate Cloudflare Tunnel (cloudflared) natively so the service can be exposed without opening any ports.
- [ ] **GraphQL API Migration**: Transition the RESTful management API to GraphQL for more efficient front-end data fetching.
- [ ] **Event-Driven Webhook System**: Broadcast events (stream started, stream failed, scraper error) to Discord, Telegram, or custom webhooks.

## 🌍 Phase 7: Community & Ecosystem (Future)
- [ ] **Stremiarr Addon Store**: A centralized, community-driven repository where users can share custom scrapers, UI themes, and plugins.
- [ ] **Multi-User Role-Based Access Control (RBAC)**: Support multiple users with varying permissions (e.g. admin, viewer, scraper-only) on the web dashboard.
- [ ] **P2P Scraper Network**: A distributed network where multiple Stremiarr instances can share cached metadata and valid torrent hashes to reduce load on trackers.
- [ ] **Mobile Management App**: Native iOS and Android apps for managing the Stremiarr instance, tokens, and monitoring server health on the go.
- [ ] **OAuth2/OIDC SSO Integration**: Allow users to authenticate to the dashboard using Discord, Google, or Authelia/Authentik.

## 📊 Phase 8: Analytics & Insights (Future)
- [ ] **Comprehensive Data Lake Integration**: Export streaming history and scrape analytics to ClickHouse or BigQuery for deep analytics.
- [ ] **Predictive Pre-Caching**: Analyze user viewing habits (e.g. watching episode 1 of a season) and proactively cache upcoming episodes before the user clicks them.
- [ ] **Bandwidth Utilization Dashboards**: Granular breakdown of bandwidth usage by user, by debrid provider, and by scraper source.
- [ ] **Automated Anomaly Detection**: Alerts when scraping success rates drop significantly or if API error rates spike, indicating a potential provider outage.

## 🎞️ Phase 9: Video Playback & Transcoding Enhancements (Future)
- [ ] **On-the-fly Audio Transcoding**: Automatically transcode incompatible audio codecs (e.g. TrueHD to AC3) for specific client devices using FFmpeg in memory.
- [ ] **Dynamic Bitrate Switching**: Implement HLS/DASH manifest generation on the fly so clients can switch bitrates dynamically from Real-Debrid.
- [ ] **Hardware Acceleration Profiles**: Add fine-grained control for NVIDIA NVENC, Intel QuickSync, and Apple VideoToolbox for any server-side transcoding.

## 🛡️ Phase 10: Security & Compliance (Future)
- [ ] **Automated DMCA/Copyright Scrubbing**: Option to auto-purge specific hashes from the cache if requested, for users running public instances.
- [ ] **Advanced Abuse Protection**: Rate limit and IP ban users scraping the Stremiarr instance too aggressively using fail2ban integration.
- [ ] **End-to-End Encryption for Streams**: Wrap streams in HTTPS dynamically to prevent ISP throttling and deep packet inspection of the video data.

## 🤖 Phase 11: Machine Learning & NLP (Future)
- [ ] **Semantic Search for Content**: Use a localized vector database to find movies/shows based on plot descriptions and themes.
- [ ] **Automated Quality Scoring**: Use ML to analyze the bitrate, resolution, and audio channels of available torrents and rank them intelligently instead of just by file size.
- [ ] **LLM-Based Troubleshooting Assistant**: Integrate a local, lightweight LLM into the TUI to answer user configuration questions based on the documentation.

## 🔌 Phase 12: External Integrations (Future)
- [ ] **Home Assistant Integration**: Natively expose streaming state to Home Assistant so lights can automatically dim when Stremiarr starts streaming.
- [ ] **Radarr/Sonarr Synchronization**: Bidirectional sync between Stremiarr cache and Radarr/Sonarr databases so cached items show as "Downloaded" in the *arr stack.
- [ ] **Jellyfin / Emby Plugins**: Official plugins to mount Stremiarr content natively in Jellyfin and Emby, not just Plex.
- [ ] **Kodi Addon**: A dedicated Kodi addon that bypasses Stremio entirely and streams directly from Stremiarr's Handoff API.

## 🌐 Phase 13: Decentralization & Web3 (Future)
- [ ] **IPFS Content Hashing**: Store and distribute Stremiarr config and plugin artifacts over IPFS for uncensorable updates.
- [ ] **Tor/I2P Hidden Service Support**: Allow Stremiarr to be deployed natively as a hidden service for maximum privacy.

---

## 📈 Contribution
We welcome contributions! Please see the `CONTRIBUTING.md` (coming soon) for guidelines.
