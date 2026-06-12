package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
)

func initMetrics() {
	// Register the /metrics endpoint for Prometheus scrapers
	http.Handle("/metrics", promhttp.Handler())
}

// TrackMetrics updates Prometheus counters
func TrackMetrics(bytes int64) {
	streamsProxied.Inc()
	bytesTransferred.Add(float64(bytes))
}
