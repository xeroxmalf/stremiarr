# Handoff Roadmap

Roadmap for the Handoff service — the core Go component of the Stremiarr stack.

## Legend

- ✅ Done — implemented in current codebase
- 🚧 In progress — partially implemented or needs refinement
- 🔲 Planned — agreed next steps
- 💡 Future — ideas for later exploration

---

## Core Stability & Reliability

- ✅ Multi-token RD load balancing with round-robin
- ✅ Per-token rate-limit lockout with auto-recovery
- ✅ Exponential backoff with jitter (`rdDo`)
- ✅ AllDebrid, Premiumize, Offcloud provider support
- ✅ Smart stream validation with size probing
- ✅ Background revalidation sweep (stale streams)
- ✅ Strike system (3 failures → dead stream)
- ✅ Predictive next-episode caching
- ✅ DB migrations (SQLite ↔ PostgreSQL)
- ✅ Graceful FFmpeg fallback (if not installed)

- 🔲 Health-aggregated token scoring (promote healthy keys)
- 🔲 Circuit-breaker per upstream addon source
- 🔲 Structured JSON logging for production ops
- 💡 Distributed health consensus across multiple Handoff instances

---

## Performance & Optimization

- ✅ Bandwidth batching (30s aggregation to DB)
- ✅ HW accel detection cached with sync.Once
- ✅ SWR (stale-while-revalidate) for stream responses
- ✅ Aggressive SQLite tuning (WAL, mmap, cache_size)
- ✅ Redis cache backend support
- ✅ Context-aware source fetching with fast-fail timeouts
- ✅ gzip middleware (skips video/range/websocket)
- ✅ Bit-shift backoff instead of math.Pow

- 🔲 Connection pool tuning based on empirical load
- 🔲 Stream dedup using hash instead of raw URL
- 🔲 Concurrent DB maintenance without full table locks
- 💡 Native HTTP/3 support via quic-go

---

## Web UI (Completed)

- ✅ Dashboard with live WebSocket stats
- ✅ Source management (toggle/remove/discover)
- ✅ RD key management with lockout status
- ✅ Alias/mapping management
- ✅ Prefetch job controls + live log
- ✅ Stremio authentication
- ✅ Maintenance tools (clean streams, clear cache)
- ✅ Responsive dark theme design

- 🔲 User profiles + per-user preferences
- 🔲 Stream history browser
- 🔲 Real-time bandwidth chart
- 💡 Dark/light theme toggle, custom themes

---

## Prefetch & Caching

- ✅ Stremio library fetch via datastoreGet API
- ✅ DMM PoW direct scraping
- ✅ RD instantAvailability batch checks (40 hashes)
- ✅ Smart seeder thresholds (≥2 for submit, ≥5 for best)
- ✅ Prefetch history persistence (skip re-checked)
- ✅ Force mode (wipe history, full rescan)
- ✅ Job cancellation support
- ✅ Stream cache bust on RD cache hit

- 🔲 Per-episode/season granularity (not whole show)
- 🔲 Priority queue (watched/active shows first)
- 🔲 Bandwidth-aware submission rate limiting
- 💡 Integration with Trakt.tv watchlist for smart prefetch

---

## Integrations

- ✅ Home Assistant webhook integration
- ✅ Radarr sync (DownloadedMoviesScan)
- ✅ Sonarr sync (DownloadedEpisodesScan)
- ✅ Webhook events (stream_started, stream_failed, anomaly)
- ✅ Prometheus metrics exporter
- ✅ OpenSubtitles fetching (with caching)

- 🔲 OAuth2/OIDC login (currently stub)
- 🔲 Discord/Telegram webhook targets
- 🔲 Jellyfin/Emby native plugin (beyond stub)
- 🔲 Kodi addon polish
- 💡 Plex metadata injection from prefetch

---

## API & Developer Experience

- ✅ REST API for all management operations
- ✅ WebSocket live stats
- ✅ Addon discovery by manifest URL
- ✅ RBAC middleware (role-based access)
- ✅ Alias system for short URLs

- 🔲 GraphQL API (currently stub)
- 🔲 OpenAPI/Swagger documentation
- 🔲 Versioned API with deprecation policy
- 💡 Plugin system for custom stream processors

---

## Security & Hardening

- ✅ Rate limiting per IP
- ✅ Non-root container user
- ✅ Static binary builds (CGO_ENABLED=0)
- ✅ Secure cookie flags for OAuth

- 🔲 Admin password-based auth (currently only env RBAC)
- 🔲 TLS termination for local APIs (behind Caddy but useful standalone)
- 🔲 Input validation/mutation on all API endpoints
- 🔲 Audit logging for sensitive operations
- 💡 API key rotation support

---

## Code Quality

- ✅ golangci-lint integration
- ✅ Structured error handling
- ✅ Shared httpClient with connection pooling
- ✅ Consistent cache miss error handling

- 🔲 Increase test coverage (currently basic tests)
- 🔲 Load tests (simulate concurrent stream requests)
- 🔲 Integration tests against mock RD API
- 💡 Fuzzing for config parsing and URL handling

---

## Transcoding & Media Pipeline

- ✅ Audio transcoding to AAC via FFmpeg
- ✅ Range request support for seeking
- ✅ Hardware acceleration (CUDA/QSV/AMF detection)
- ✅ Fallback to direct proxy if FFmpeg missing

- 🔲 Video codec transcoding (H.265→H.264 for compatibility)
- 🔲 HLS manifest generation for adaptive bitrate
- 🔲 Subtitle muxing into transcode stream
- 💡 GPU-accelerated video transcoding pipeline

---

## Multi-Instance & Scaling

- ✅ Redis-backed cache for shared state
- ✅ PostgreSQL support for shared DB
- ✅ Stateless design (config/data externalized)

- 🔲 Leader election for single-writer operations
- 🔲 Load-balanced Handoff behind Caddy
- 🔲 Horizontal scaling playbook
- 💡 Cluster-wide stream validation coordination

---

## Notes

This roadmap reflects the codebase as of Aug 2026, after the major optimization pass. Priority is driven by:

1. Stability under production load
2. Streaming reliability (zero-stutter goal)
3. Operational visibility (metrics, logs, UI)
4. Extensibility (plugins, integrations)
