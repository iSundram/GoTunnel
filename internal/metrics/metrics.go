package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric references for GoTunnel.
type Metrics struct {
	ActiveTunnels     prometheus.Gauge
	StreamsActive     prometheus.Gauge
	BytesSent         prometheus.Counter
	BytesRecv         prometheus.Counter
	Requests          *prometheus.CounterVec
	TokenAuthFailures prometheus.Counter
	Reconnects        prometheus.Counter
}

// NewMetrics creates and registers all GoTunnel Prometheus metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
		ActiveTunnels: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gotunnel_active_tunnels_total",
			Help: "Number of currently active tunnels.",
		}),
		StreamsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gotunnel_streams_active",
			Help: "Number of currently active streams.",
		}),
		BytesSent: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gotunnel_bytes_sent_total",
			Help: "Total number of bytes sent.",
		}),
		BytesRecv: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gotunnel_bytes_recv_total",
			Help: "Total number of bytes received.",
		}),
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gotunnel_requests_total",
			Help: "Total number of requests by HTTP status code and method.",
		}, []string{"code", "method"}),
		TokenAuthFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gotunnel_token_auth_failures_total",
			Help: "Total number of token authentication failures.",
		}),
		Reconnects: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gotunnel_reconnects_total",
			Help: "Total number of reconnects.",
		}),
	}

	m.Register()
	return m
}

// Register registers all metrics with the default Prometheus registry.
func (m *Metrics) Register() {
	prometheus.MustRegister(
		m.ActiveTunnels,
		m.StreamsActive,
		m.BytesSent,
		m.BytesRecv,
		m.Requests,
		m.TokenAuthFailures,
		m.Reconnects,
	)
}

// IncActiveTunnels increments the active tunnels gauge by 1.
func (m *Metrics) IncActiveTunnels() { m.ActiveTunnels.Inc() }

// DecActiveTunnels decrements the active tunnels gauge by 1.
func (m *Metrics) DecActiveTunnels() { m.ActiveTunnels.Dec() }

// SetActiveTunnels sets the active tunnels gauge to the given value.
func (m *Metrics) SetActiveTunnels(v float64) { m.ActiveTunnels.Set(v) }

// IncStreamsActive increments the active streams gauge by 1.
func (m *Metrics) IncStreamsActive() { m.StreamsActive.Inc() }

// DecStreamsActive decrements the active streams gauge by 1.
func (m *Metrics) DecStreamsActive() { m.StreamsActive.Dec() }

// SetStreamsActive sets the active streams gauge to the given value.
func (m *Metrics) SetStreamsActive(v float64) { m.StreamsActive.Set(v) }

// AddBytesSent adds the given number of bytes to the bytes-sent counter.
func (m *Metrics) AddBytesSent(n float64) { m.BytesSent.Add(n) }

// AddBytesRecv adds the given number of bytes to the bytes-received counter.
func (m *Metrics) AddBytesRecv(n float64) { m.BytesRecv.Add(n) }

// IncRequests increments the requests counter for the given code and method.
func (m *Metrics) IncRequests(code, method string) {
	m.Requests.WithLabelValues(code, method).Inc()
}

// IncTokenAuthFailures increments the token auth failures counter by 1.
func (m *Metrics) IncTokenAuthFailures() { m.TokenAuthFailures.Inc() }

// IncReconnects increments the reconnects counter by 1.
func (m *Metrics) IncReconnects() { m.Reconnects.Inc() }

// Handler returns an http.Handler that serves the Prometheus /metrics endpoint.
func Handler() http.Handler {
	return promhttp.Handler()
}
