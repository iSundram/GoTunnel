package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/iSundram/GoTunnel/internal/broker"
	"github.com/iSundram/GoTunnel/internal/logging"
	"github.com/iSundram/GoTunnel/internal/metrics"
	"github.com/iSundram/GoTunnel/internal/protocol"
	"github.com/iSundram/GoTunnel/internal/registry"
)

const defaultTimeout = 30 * time.Second

// httpRequest is the JSON envelope sent to the tunnel client.
type httpRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body,omitempty"`
}

// httpResponse is the JSON envelope received from the tunnel client.
type httpResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body,omitempty"`
}

// Proxy is an HTTP handler that routes incoming requests through tunnel
// client WebSocket connections based on the Host header subdomain.
type Proxy struct {
	broker         *broker.Broker
	registry       registry.Registry
	metrics        *metrics.Metrics
	log            *logging.Logger
	publicHostname string
}

// NewProxy creates a new reverse proxy handler.
func NewProxy(b *broker.Broker, r registry.Registry, m *metrics.Metrics, l *logging.Logger) *Proxy {
	return &Proxy{
		broker:   b,
		registry: r,
		metrics:  m,
		log:      l.WithComponent("proxy"),
	}
}

// SetPublicHostname configures the base domain used to extract subdomains
// from incoming Host headers.
func (p *Proxy) SetPublicHostname(hostname string) {
	p.publicHostname = hostname
}

// ServeHTTP implements http.Handler. It extracts the subdomain from the
// request Host header, looks up the corresponding tunnel, and forwards the
// request through the client's WebSocket connection.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	subdomain := extractSubdomain(r.Host, p.publicHostname)
	if subdomain == "" {
		http.Error(w, "502 Bad Gateway: unable to determine tunnel", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}

	// Look up tunnel in the registry.
	tunnel, err := p.registry.GetBySubdomain(subdomain)
	if err != nil {
		p.log.Warn("tunnel not found", "subdomain", subdomain, "error", err)
		http.Error(w, "502 Bad Gateway: tunnel not found", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}

	// Get the client connection from the broker.
	conn, ok := p.broker.GetConnectionBySubdomain(subdomain)
	if !ok {
		p.log.Warn("client connection not found", "subdomain", subdomain)
		http.Error(w, "502 Bad Gateway: tunnel client not connected", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}

	// Open a stream on the client connection.
	stream := p.broker.OpenStream(conn, tunnel.ID)
	defer p.broker.CloseStream(conn, stream.ID)

	// Send an open_stream frame to the client.
	openMsg := protocol.OpenStream{
		Type:         "open_stream",
		StreamID:     strconv.FormatUint(uint64(stream.ID), 10),
		TargetTunnel: tunnel.ID,
		Metadata: protocol.StreamMetadata{
			Method: r.Method,
			Path:   r.URL.RequestURI(),
		},
	}
	openFrame, err := protocol.EncodeControlMessage(protocol.FrameOpenStream, openMsg)
	if err != nil {
		p.log.Error("failed to encode open_stream", "error", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}
	if err := p.broker.SendMessage(conn, openFrame); err != nil {
		p.log.Error("failed to send open_stream", "error", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}

	// Serialize the HTTP request into a JSON envelope.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		p.log.Error("failed to read request body", "error", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}

	headers := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		headers[k] = strings.Join(v, ", ")
	}

	reqEnvelope := httpRequest{
		Method:  r.Method,
		Path:    r.URL.RequestURI(),
		Headers: headers,
		Body:    body,
	}
	reqData, err := json.Marshal(reqEnvelope)
	if err != nil {
		p.log.Error("failed to marshal request envelope", "error", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}

	// Send the serialized request as a stream_data frame.
	streamData := protocol.StreamData{
		StreamID: strconv.FormatUint(uint64(stream.ID), 10),
		Data:     reqData,
	}
	dataFrame, err := protocol.EncodeControlMessage(protocol.FrameStreamData, streamData)
	if err != nil {
		p.log.Error("failed to encode stream_data", "error", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}
	if err := p.broker.SendMessage(conn, dataFrame); err != nil {
		p.log.Error("failed to send stream_data", "error", err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}
	p.metrics.AddBytesSent(float64(len(reqData)))

	// Wait for the response from the tunnel client with a timeout.
	var respData []byte
	select {
	case respData = <-stream.Response:
	case <-stream.Done:
		p.log.Warn("stream closed before response", "stream_id", stream.ID)
		http.Error(w, "502 Bad Gateway: stream closed", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	case <-time.After(defaultTimeout):
		p.log.Warn("stream response timeout", "stream_id", stream.ID)
		http.Error(w, "504 Gateway Timeout", http.StatusGatewayTimeout)
		p.metrics.IncRequests("504", r.Method)
		return
	}
	p.metrics.AddBytesRecv(float64(len(respData)))

	// Decode the response envelope.
	var resp httpResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		p.log.Error("failed to unmarshal response envelope", "error", err)
		http.Error(w, "502 Bad Gateway: invalid response", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}

	// Write response headers.
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}

	statusCode := resp.StatusCode
	if statusCode == 0 {
		p.log.Error("tunnel client returned no status code", "stream_id", stream.ID)
		http.Error(w, "502 Bad Gateway: invalid response status", http.StatusBadGateway)
		p.metrics.IncRequests("502", r.Method)
		return
	}
	w.WriteHeader(statusCode)

	// Write response body.
	if len(resp.Body) > 0 {
		if _, err := w.Write(resp.Body); err != nil {
			p.log.Error("failed to write response body", "error", err)
		}
	}

	p.metrics.IncRequests(fmt.Sprintf("%d", statusCode), r.Method)
}

// extractSubdomain extracts the subdomain from a Host header given the
// public hostname. For example, given host "myapp.gotunnel.example.com"
// and publicHostname "gotunnel.example.com", it returns "myapp".
// If the host does not end with the public hostname or has no subdomain
// prefix, an empty string is returned.
func extractSubdomain(host, publicHostname string) string {
	// Strip port if present.
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		h := host[:idx]
		// Only strip if what follows the colon looks like a port.
		if _, err := strconv.Atoi(host[idx+1:]); err == nil {
			host = h
		}
	}

	host = strings.ToLower(strings.TrimSpace(host))
	publicHostname = strings.ToLower(strings.TrimSpace(publicHostname))

	if publicHostname == "" {
		return ""
	}

	suffix := "." + publicHostname
	if !strings.HasSuffix(host, suffix) {
		return ""
	}

	subdomain := strings.TrimSuffix(host, suffix)
	if subdomain == "" {
		return ""
	}

	// Subdomain must be a single label (no additional dots).
	if strings.Contains(subdomain, ".") {
		return ""
	}

	return subdomain
}
