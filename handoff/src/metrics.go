package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricStreamsRequested = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "handoff_streams_requested_total",
			Help: "The total number of stream requests received",
		},
		[]string{"alias"},
	)

	metricRDApiCalls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "handoff_rd_api_calls_total",
			Help: "The total number of Real-Debrid API calls",
		},
		[]string{"method", "status"},
	)

	metricStreamsPlayed = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "handoff_streams_played_total",
			Help: "The total number of streams played successfully",
		},
	)

	metricStreamFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "handoff_stream_failures_total",
			Help: "The total number of streams that failed or hit strikes",
		},
	)
)
