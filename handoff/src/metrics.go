package main

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	streamsProxied = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_streams_proxied_total",
		Help: "The total number of streams proxied to clients",
	})

	bytesTransferred = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_bandwidth_bytes_total",
		Help: "The total bytes of bandwidth transferred",
	})

	activeConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "stremiarr_active_connections",
		Help: "The number of currently active websocket connections",
	})
	metricRDApiCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stremiarr_rd_api_calls_total",
		Help: "The total number of API calls made to Real-Debrid",
	}, []string{"method", "status"})

	metricStreamsRequested = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stremiarr_streams_requested_total",
		Help: "The total number of streams requested by users",
	}, []string{"type"})

	metricStreamFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_stream_failures_total",
		Help: "The total number of stream playback failures",
	})

	metricStreamsPlayed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_streams_played_total",
		Help: "The total number of streams successfully played",
	})

	// Cache metrics
	metricCacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_cache_hits_total",
		Help: "The total number of cache hits",
	})
	metricCacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_cache_misses_total",
		Help: "The total number of cache misses",
	})

	// Request duration histogram
	metricRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stremiarr_request_duration_seconds",
		Help:    "Request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"path", "method", "status_code"})

	// Debrid provider status gauge
	metricDebridProviderStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stremiarr_debrid_provider_status",
		Help: "Status of debrid providers (1=healthy, 0=rate limited/error)",
	}, []string{"provider"})

	// Prefetch metrics
	metricPrefetchItems = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_prefetch_items_processed_total",
		Help: "Total library items processed by prefetch",
	})
	metricPrefetchCached = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_prefetch_already_cached_total",
		Help: "Total items found already cached on debrid",
	})
	metricPrefetchSubmitted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_prefetch_submitted_total",
		Help: "Total items submitted to debrid",
	})

	// Stream validation metrics
	metricValidationAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_validation_attempts_total",
		Help: "Total stream validation attempts",
	})
	metricValidationSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_validation_success_total",
		Help: "Successful stream validations",
	})
	metricValidationFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "stremiarr_validation_failed_total",
		Help: "Failed stream validations",
	})
)

func initMetrics() {
	// /metrics endpoint is handled in routeHandler via promhttp.Handler()
}

// TrackMetrics updates Prometheus counters
func TrackMetrics(bytes int64) {
	streamsProxied.Inc()
	bytesTransferred.Add(float64(bytes))
}

// ObserveRequestDuration records a request's latency
func ObserveRequestDuration(path, method, statusCode string, duration time.Duration) {
	metricRequestDuration.WithLabelValues(path, method, statusCode).Observe(duration.Seconds())
}

// RecordCacheHit records a cache hit event
func RecordCacheHit() {
	metricCacheHits.Inc()
}

// RecordCacheMiss records a cache miss event
func RecordCacheMiss() {
	metricCacheMisses.Inc()
}

// UpdateDebridProviderStatus sets the status gauge for a provider
func UpdateDebridProviderStatus(provider string, healthy bool) {
	if healthy {
		metricDebridProviderStatus.WithLabelValues(provider).Set(1)
	} else {
		metricDebridProviderStatus.WithLabelValues(provider).Set(0)
	}
}
