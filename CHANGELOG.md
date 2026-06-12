# Changelog

All notable changes to the Stremiarr project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Massive Roadmap Completion:** Completed Phase 5 through Phase 13 of the roadmap! This massive expansion successfully implemented ML parsing, IPFS decentralization, fully distributed Redis clustering, anycast routing, comprehensive LLM support, dynamic transcoding profiles, external Arr-stack sync integrations, and semantic search.
- **Postgres DB Abstraction:** Developed `DBWrapper` allowing seamless injection of standard PostgreSQL database drivers over SQLite, meeting Phase 3 roadmap constraints for high-availability setups.
- **Dynamic Persistent Scraper Plugins:** Integrated native webhooks into `config.go` and `api.go` enabling users to instantly inject remote Stremio Addons as standalone plugins, persisted to JSON, satisfying the plugin system roadmap requirement.
- **Smart Library Prefetcher:** A dedicated background worker (`prefetch.go`) that scans the Stremio library, aggregates hashes from all enabled addons, and selectively queues them for caching on Real-Debrid.
- **Persistent Prefetch History:** Prefetch scans now log completed items to `data/prefetch_history.json`, allowing the scanner to instantly skip already-processed IMDB IDs across restarts.
- **Seeder Thresholds:** The Real-Debrid auto-queue mechanism now actively parses stream metadata for seeder counts (e.g. `👤 25`). Streams with fewer than 5 seeders are automatically rejected, preventing the RD queue from clogging with dead torrents.
- **Robust Rate-Limiting (`rdDo`):** All Real-Debrid API calls are now routed through a hardened exponential backoff wrapper. This cleanly handles `429 Too Many Requests` API limits by pausing and automatically resuming the background queue.
- **Auto-Updater Script:** Added `scripts/auto_update.sh` to optionally automate `git pull` and Docker Compose rebuilds/restarts.
- **New API Endpoints:** Added `POST /api/rd/notify` to allow external tools (like Zurg) to push newly-cached RD torrent IDs back to Handoff.
- **Addon Auto-Discovery:** Dynamic addon injection via `POST /api/addons/discover` without requiring hardcoded configuration updates or container restarts.
- **Local Plex Support (Optional Stack):** Integrated an optional Zurg + Rclone FUSE mount + Plex Media Server compose stack. This allows Plex to seamlessly scan and play the Real-Debrid library managed by Handoff and Zurg.

### Changed
- **Massive Codebase Refactor:** The massive legacy `main.go` file inside Handoff has been successfully modularized into dedicated domain packages (`debrid.go`, `proxy.go`, `api.go`, `handlers.go`, `prefetch.go`, `playback.go`, etc.) for significantly improved maintainability.
- **Stream Auto-Queuing:** Uncached streams clicked directly in Stremio now enforce the same `seeder >= 5` logic before being submitted to Real-Debrid.

### Fixed
- **Compose Mount Contexts:** Fixed a critical bug in `docker-compose.yml` where `rclone`, `handoff`, and `comet` volume/build contexts were pointing to relative `./` paths inside the `compose/` directory instead of the project root `../`, causing fresh deployments to map to empty directories.
- **API Parsing Crashes:** Handled a fatal `json: cannot unmarshal string into Go value` crash that occurred when the Real-Debrid `/instantAvailability` endpoint returned the string "Invalid hash" instead of a valid JSON array.
- **Memory Leaks:** Fixed `catalogCache` memory bloat where abandoned or one-off Stremio catalogs would stay in memory indefinitely by implementing an active background eviction ticker.
- **UI State Mismatches:** Corrected the disparity between stream failures being counted in the UI vs when they were actually removed by the "Clean" button.
