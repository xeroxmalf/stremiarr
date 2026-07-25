# Performance Tuning & Optimizations

Stremiarr is designed to be highly optimized out of the box, but here are some details and tweaks to push your setup to the maximum capability.

## Handoff Profiling & Memory
Handoff is written in Go and utilizes memory-efficient parsing for large Stremio manifests.
- **Regex Caching**: Core regex patterns (like parsing seeders) are globally compiled. This drops CPU usage significantly when processing thousands of stream torrent hashes.
- **Garbage Collection**: If you want to keep Handoff's memory strictly under control on low-RAM devices (e.g., Raspberry Pi), you can set `GOMEMLIMIT=250MiB` in the `docker-compose.yml` environment.

## Rclone VFS Cache Tuning
The Rclone instance (which caches chunks from Real-Debrid) is the bottleneck for stutter-free 4K playback.
- **`--vfs-read-chunk-size`**: Default is `32M`. If you are watching extreme bitrate Remuxes (80GB+ files) and have a fast SSD, increase this to `128M` and `--vfs-read-chunk-size-limit` to `2G`.
- **`--vfs-cache-max-size`**: We recommend at least `50G` for 4K streaming. If you have plenty of storage, `250G` ensures you rarely redownload the same file chunks when scrubbing/rewinding.

## PostgreSQL Tuning for Comet Scraper
Comet relies heavily on Postgres for tracking metadata. 
In your `docker-compose.yml`, the Postgres container has optimized `command` arguments:
- `-c shared_buffers=4GB` (Increase if you have >4GB RAM)
- `-c effective_cache_size=11GB`
- `-c work_mem=64MB`
- `-c maintenance_work_mem=1GB`
- `-c max_connections=200`

## Handoff Connection Pooling
Handoff optimizes database and backend interactions through connection pooling:
- `MaxOpenConns=25`
- `MaxIdleConns=5`

## HTTP Client Tuning
To ensure robust upstream fetching from addons and Stremio services:
- `MaxIdleConnsPerHost=20` (or `32` for heavier setups)

## Prometheus Metrics
Handoff exposes native Prometheus metrics at `http://<host>:9944/metrics`.
We highly recommend setting up a Prometheus + Grafana stack to scrape this endpoint. Tracking `handoff_streams_requested_total` vs `handoff_streams_played_total` will give you precise insight into your cache-hit ratios and help you tune your prefetch thresholds.
