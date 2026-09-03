package api

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricHTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "invetor_http_requests_total",
		Help: "HTTP requests by code and path class",
	}, []string{"code", "route"})
	metricBuildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "invetor_build_info",
		Help: "Build info (1 if matches labels)",
	}, []string{"version", "commit"})
	metricUptimeSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "invetor_uptime_seconds",
		Help: "Process uptime in seconds",
	})
	startedAt = time.Now()
)

func observeHTTP(route string, code int) {
	metricHTTPRequests.WithLabelValues(strconv.Itoa(code), route).Inc()
}
