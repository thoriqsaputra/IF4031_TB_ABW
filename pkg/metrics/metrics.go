package metrics

import (
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"strconv"
	"time"
)

// HTTPMetrics contains HTTP-related Prometheus metrics
type HTTPMetrics struct {
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	RequestsInFlight prometheus.Gauge
}

// NewHTTPMetrics creates new HTTP metrics
func NewHTTPMetrics(serviceName string) *HTTPMetrics {
	metrics := &HTTPMetrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: serviceName + "_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    serviceName + "_http_request_duration_seconds",
				Help:    "HTTP request latencies in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		RequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: serviceName + "_http_requests_in_flight",
				Help: "Number of HTTP requests currently being processed",
			},
		),
	}

	// Register metrics
	prometheus.MustRegister(metrics.RequestsTotal)
	prometheus.MustRegister(metrics.RequestDuration)
	prometheus.MustRegister(metrics.RequestsInFlight)

	return metrics
}

// Middleware returns a Fiber middleware that records HTTP metrics
func (m *HTTPMetrics) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		m.RequestsInFlight.Inc()
		defer m.RequestsInFlight.Dec()

		// Process request
		err := c.Next()

		// Record metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Response().StatusCode())
		method := c.Method()
		path := c.Path()

		m.RequestDuration.WithLabelValues(method, path).Observe(duration)
		m.RequestsTotal.WithLabelValues(method, path, status).Inc()

		return err
	}
}

// Handler returns the Prometheus metrics HTTP handler
func Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		handler := promhttp.Handler()
		handler.ServeHTTP(c.Response().BodyWriter(), c.Request())
		return nil
	}
}
