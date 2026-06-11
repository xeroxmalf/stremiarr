# Troubleshooting Guide

Welcome to the Stremiarr Troubleshooting Guide. If you are experiencing issues with your deployment, check the common scenarios below.

## Real-Debrid 429 Too Many Requests
**Symptom**: Streams fail to play, and Handoff logs show `429 Too Many Requests`.
**Cause**: You have hit the API limits for Real-Debrid (usually 200 requests / minute).
**Solution**:
1. **Ensure `rdDo` backoff is active**: Handoff natively uses exponential backoff to handle these. If you are experiencing this constantly, you may need to reduce your background library prefetch concurrency or check if an external script is hammering the API.
2. **Add Multiple Tokens**: Stremiarr supports multi-token round-robin load balancing. Add multiple comma-separated keys to `RD_API_KEY` in your `.env` file to double or triple your API limits.

## Stremio Catalog Not Loading
**Symptom**: Stremio shows "Failed to fetch" or infinite loading wheels on catalogs.
**Cause**: Handoff proxying to your external Addons might be timing out.
**Solution**: 
1. Check `docker logs handoff` for `context deadline exceeded` errors.
2. Disable slow or unresponsive Addons via the Web UI (accessible at port `9944`).

## Plex / Local Library Scan Issues (FUSE Mount)
**Symptom**: Plex cannot see your movies, or scanning is infinitely stuck.
**Cause**: The Rclone FUSE mount isn't working correctly due to missing privileges.
**Solution**:
1. Ensure your `docker-compose.yml` has `cap_add: - SYS_ADMIN` and `devices: - /dev/fuse` for the `rclone-mount` service.
2. Verify that Docker supports shared bind mounts (`:shared`) on your host OS. On some NAS systems, you might need to run the mount natively outside Docker.

## High CPU Usage in Handoff
**Symptom**: The `handoff` container is maxing out a CPU core.
**Cause**: The background Library Prefetcher is aggressively scanning hundreds of IMDB IDs.
**Solution**:
The prefetcher is designed to run once and then cache the history. Let it finish. If you must stop it, restart the container: `docker restart handoff`.

## Checking Metrics
You can always view live operational health by connecting Grafana to the newly exposed `/metrics` endpoint on port `9944`.
