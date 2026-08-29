## 2025-02-27 - Backend architectural mismatch: UI polling vs write flush

**Learning:** Handoff features an aggressive data flush cycle. In this case, `bwFlush()` aggregates and writes bandwidth numbers every 30 seconds to the DB to prevent thrashing. However, the UI constantly polls `/api/bandwidth` every 5 seconds which triggers a full table aggregation query over `bandwidth_log` every single time. Since the underlying database records only change at most every 30 seconds, 5 out of 6 of these heavy database queries were completely redundant and returning identical data. This is a classic mismatch between write frequency and read frequency.

**Action:** Added a 30-second in-memory cache directly mirroring the background batch worker (`bwFlushLoop`) interval to eliminate unnecessary DB group-by reads. Whenever optimizing UI polling endpoints, always cross-reference how often the underlying data source actually updates to spot redundant reads.
