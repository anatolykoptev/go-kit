package wowa

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics follow the jeff_requests_total convention: one counter per request
// outcome, labelled by endpoint. Outcomes: ok | remote_error | http_error |
// transport_error | timeout | decode_error | truncated.
var wowaRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "wowa",
		Name:      "requests_total",
		Help:      "go-wowa REST requests partitioned by endpoint and outcome.",
	},
	[]string{"endpoint", "outcome"},
)
