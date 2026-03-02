// Package client implements the GoTunnel client that connects to the gateway,
// registers tunnels, and forwards incoming requests to local services.
package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/iSundram/GoTunnel/internal/config"
	"github.com/iSundram/GoTunnel/internal/logging"
	"github.com/iSundram/GoTunnel/internal/protocol"
)

// httpRequest mirrors the proxy package's request envelope.
type httpRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body,omitempty"`
}

// httpResponse mirrors the proxy package's response envelope.
type httpResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body,omitempty"`
}

// Client connects to the GoTunnel gateway via WebSocket, registers tunnels,
// handles heartbeat, and forwards incoming stream requests to local services.
type Client struct {
	cfg    *config.ClientConfig
	ws     *websocket.Conn
	logger *logging.Logger
	done   chan struct{} // closed when the current connection drops
	closed chan struct{} // closed when Close() is called

	wsMu             sync.Mutex
	pendingStreams    map[string]chan []byte
	pendingStreamsMu sync.Mutex
}

// NewClient creates a new Client from the given configuration.
func NewClient(cfg *config.ClientConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("client: config must not be nil")
	}
	logger := logging.Default().WithComponent("client")
	return &Client{
		cfg:           cfg,
		logger:        logger,
		done:          make(chan struct{}),
		closed:        make(chan struct{}),
		pendingStreams: make(map[string]chan []byte),
	}, nil
}

// Connect dials the WebSocket gateway, sends the register frame, reads the
// control_response, and starts the heartbeat and read-loop goroutines.
func (c *Client) Connect() error {
	dialer := websocket.Dialer{}

	tlsCfg := c.cfg.Client.TLS
	if tlsCfg.SkipVerify || tlsCfg.CAFile != "" {
		tc := &tls.Config{}
		if tlsCfg.SkipVerify {
			tc.InsecureSkipVerify = true //nolint:gosec // user-configured
		}
		if tlsCfg.CAFile != "" {
			caCert, err := os.ReadFile(tlsCfg.CAFile)
			if err != nil {
				return fmt.Errorf("client: read CA file: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caCert) {
				return fmt.Errorf("client: failed to parse CA certificate")
			}
			tc.RootCAs = pool
		}
		dialer.TLSClientConfig = tc
	}

	ws, _, err := dialer.Dial(c.cfg.Client.GatewayURL, nil)
	if err != nil {
		return fmt.Errorf("client: websocket dial: %w", err)
	}
	c.ws = ws

	// Build and send the register frame.
	tunnels := make([]protocol.TunnelConfig, len(c.cfg.Client.Tunnels))
	for i, t := range c.cfg.Client.Tunnels {
		tunnels[i] = protocol.TunnelConfig{
			Name:      t.Name,
			Protocol:  t.Protocol,
			Port:      t.Port,
			Subdomain: t.Subdomain,
		}
	}
	reg := protocol.Register{
		Type:    "register",
		Token:   c.cfg.Client.Token,
		Tunnels: tunnels,
		Metadata: protocol.ClientMetadata{
			OS:      runtime.GOOS,
			Version: "1.0.0",
		},
	}

	frame, err := protocol.EncodeControlMessage(protocol.FrameRegister, reg)
	if err != nil {
		ws.Close()
		return fmt.Errorf("client: encode register: %w", err)
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		ws.Close()
		return fmt.Errorf("client: send register: %w", err)
	}

	// Read the control_response to confirm registration.
	_, raw, err := ws.ReadMessage()
	if err != nil {
		ws.Close()
		return fmt.Errorf("client: read register response: %w", err)
	}
	frameType, payload, err := protocol.DecodeFrame(bytes.NewReader(raw))
	if err != nil {
		ws.Close()
		return fmt.Errorf("client: decode register response: %w", err)
	}
	if frameType != protocol.FrameControlResponse {
		ws.Close()
		return fmt.Errorf("client: expected control_response, got frame type %d", frameType)
	}
	var resp protocol.ControlResponse
	if err := protocol.DecodeControlMessage(payload, &resp); err != nil {
		ws.Close()
		return fmt.Errorf("client: decode control response: %w", err)
	}
	if resp.Code != protocol.CodeOK {
		ws.Close()
		return fmt.Errorf("client: registration failed: %s (code %d)", resp.Message, resp.Code)
	}

	c.logger.Info("registered with gateway", "tunnels", len(tunnels))

	go c.heartbeatLoop()
	go c.readLoop()

	return nil
}

// heartbeatLoop sends heartbeat frames at the configured interval.
func (c *Client) heartbeatLoop() {
	interval := time.Duration(c.cfg.Client.HeartbeatSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-c.closed:
			return
		case <-ticker.C:
			hb := protocol.Heartbeat{
				Type:      "heartbeat",
				Timestamp: time.Now().Unix(),
			}
			frame, err := protocol.EncodeControlMessage(protocol.FrameHeartbeat, hb)
			if err != nil {
				c.logger.Error("failed to encode heartbeat", "error", err)
				continue
			}
			c.wsMu.Lock()
			err = c.ws.WriteMessage(websocket.BinaryMessage, frame)
			c.wsMu.Unlock()
			if err != nil {
				c.logger.Error("failed to send heartbeat", "error", err)
				return
			}
			c.logger.Debug("heartbeat sent")
		}
	}
}

// readLoop reads frames from the gateway WebSocket and dispatches them.
func (c *Client) readLoop() {
	defer func() {
		select {
		case <-c.done:
		default:
			close(c.done)
		}
	}()

	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.logger.Error("websocket read error", "error", err)
			}
			return
		}

		frameType, payload, err := protocol.DecodeFrame(bytes.NewReader(raw))
		if err != nil {
			c.logger.Warn("invalid frame from gateway", "error", err)
			continue
		}

		switch frameType {
		case protocol.FrameOpenStream:
			var os protocol.OpenStream
			if err := protocol.DecodeControlMessage(payload, &os); err != nil {
				c.logger.Warn("bad open_stream payload", "error", err)
				continue
			}
			go c.handleOpenStream(os)

		case protocol.FrameStreamData:
			var sd protocol.StreamData
			if err := protocol.DecodeControlMessage(payload, &sd); err != nil {
				c.logger.Warn("bad stream_data payload", "error", err)
				continue
			}
			c.deliverStreamData(sd.StreamID, sd.Data)

		case protocol.FrameHeartbeat:
			c.logger.Debug("heartbeat received from gateway")

		case protocol.FrameControlResponse:
			var cr protocol.ControlResponse
			if err := protocol.DecodeControlMessage(payload, &cr); err != nil {
				c.logger.Warn("bad control_response payload", "error", err)
				continue
			}
			if cr.Code != protocol.CodeOK {
				c.logger.Error("control error from gateway", "code", cr.Code, "message", cr.Message)
			} else {
				c.logger.Debug("control response", "message", cr.Message)
			}

		default:
			c.logger.Warn("unknown frame type from gateway", "type", frameType)
		}
	}
}

// handleOpenStream processes an open_stream frame: waits for the accompanying
// stream_data, makes a local HTTP request, and sends the response back.
func (c *Client) handleOpenStream(openMsg protocol.OpenStream) {
	c.logger.Debug("open_stream received", "stream_id", openMsg.StreamID, "tunnel", openMsg.TargetTunnel)

	dataCh := make(chan []byte, 1)
	c.pendingStreamsMu.Lock()
	c.pendingStreams[openMsg.StreamID] = dataCh
	c.pendingStreamsMu.Unlock()

	defer func() {
		c.pendingStreamsMu.Lock()
		delete(c.pendingStreams, openMsg.StreamID)
		c.pendingStreamsMu.Unlock()
	}()

	// Wait for the stream_data frame carrying the request body.
	var reqData []byte
	select {
	case reqData = <-dataCh:
	case <-c.done:
		return
	case <-c.closed:
		return
	case <-time.After(30 * time.Second):
		c.logger.Warn("timeout waiting for stream_data", "stream_id", openMsg.StreamID)
		return
	}

	var req httpRequest
	if err := json.Unmarshal(reqData, &req); err != nil {
		c.logger.Error("failed to parse request envelope", "stream_id", openMsg.StreamID, "error", err)
		return
	}

	port := c.resolvePort(openMsg.TargetTunnel)
	if port == 0 {
		c.logger.Error("no matching tunnel for target", "target", openMsg.TargetTunnel)
		return
	}

	localBind := c.cfg.Client.LocalBind
	if localBind == "" {
		localBind = "127.0.0.1"
	}
	targetURL := fmt.Sprintf("http://%s:%d%s", localBind, port, req.Path)

	httpReq, err := http.NewRequest(req.Method, targetURL, bytes.NewReader(req.Body))
	if err != nil {
		c.logger.Error("failed to create local request", "error", err)
		return
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("local request failed", "error", err)
		c.sendErrorResponse(openMsg.StreamID, http.StatusBadGateway, "local service unavailable")
		return
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		c.logger.Error("failed to read local response body", "error", err)
		return
	}

	respHeaders := make(map[string]string, len(httpResp.Header))
	for k, v := range httpResp.Header {
		respHeaders[k] = strings.Join(v, ", ")
	}

	resp := httpResponse{
		StatusCode: httpResp.StatusCode,
		Headers:    respHeaders,
		Body:       body,
	}
	respData, err := json.Marshal(resp)
	if err != nil {
		c.logger.Error("failed to marshal response", "error", err)
		return
	}

	c.sendStreamData(openMsg.StreamID, respData)
	c.sendCloseStream(openMsg.StreamID)

	c.logger.Debug("stream completed", "stream_id", openMsg.StreamID, "status", httpResp.StatusCode)
}

// deliverStreamData routes incoming stream data to the pending stream handler.
func (c *Client) deliverStreamData(streamID string, data []byte) {
	c.pendingStreamsMu.Lock()
	ch, ok := c.pendingStreams[streamID]
	c.pendingStreamsMu.Unlock()
	if !ok {
		c.logger.Warn("no pending stream for data", "stream_id", streamID)
		return
	}
	select {
	case ch <- data:
	default:
		c.logger.Warn("stream data channel full", "stream_id", streamID)
	}
}

// resolvePort finds the local port for the given target tunnel ID by matching
// the subdomain suffix (server constructs IDs as "connID-subdomain").
func (c *Client) resolvePort(targetTunnel string) int {
	for _, t := range c.cfg.Client.Tunnels {
		if strings.HasSuffix(targetTunnel, "-"+t.Subdomain) {
			return t.Port
		}
	}
	if len(c.cfg.Client.Tunnels) > 0 {
		return c.cfg.Client.Tunnels[0].Port
	}
	return 0
}

// sendStreamData sends a stream_data frame with the given payload.
func (c *Client) sendStreamData(streamID string, data []byte) {
	sd := protocol.StreamData{StreamID: streamID, Data: data}
	frame, err := protocol.EncodeControlMessage(protocol.FrameStreamData, sd)
	if err != nil {
		c.logger.Error("failed to encode stream_data", "error", err)
		return
	}
	c.wsMu.Lock()
	err = c.ws.WriteMessage(websocket.BinaryMessage, frame)
	c.wsMu.Unlock()
	if err != nil {
		c.logger.Error("failed to send stream_data", "error", err)
	}
}

// sendCloseStream sends a close_stream frame for the given stream.
func (c *Client) sendCloseStream(streamID string) {
	cs := protocol.CloseStream{StreamID: streamID}
	frame, err := protocol.EncodeControlMessage(protocol.FrameCloseStream, cs)
	if err != nil {
		c.logger.Error("failed to encode close_stream", "error", err)
		return
	}
	c.wsMu.Lock()
	err = c.ws.WriteMessage(websocket.BinaryMessage, frame)
	c.wsMu.Unlock()
	if err != nil {
		c.logger.Error("failed to send close_stream", "error", err)
	}
}

// sendErrorResponse sends an error response envelope back to the gateway.
func (c *Client) sendErrorResponse(streamID string, statusCode int, message string) {
	resp := httpResponse{
		StatusCode: statusCode,
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       []byte(message),
	}
	respData, err := json.Marshal(resp)
	if err != nil {
		return
	}
	c.sendStreamData(streamID, respData)
	c.sendCloseStream(streamID)
}

// Run connects to the gateway and blocks until done. On disconnection it
// reconnects with exponential backoff up to the configured maximum retries.
func (c *Client) Run() error {
	maxRetries := c.cfg.Client.Reconnect.MaxRetries
	backoffMs := c.cfg.Client.Reconnect.BackoffMs
	if backoffMs <= 0 {
		backoffMs = 1000
	}

	attempt := 0
	for {
		// Reset per-connection state.
		c.done = make(chan struct{})
		c.pendingStreamsMu.Lock()
		c.pendingStreams = make(map[string]chan []byte)
		c.pendingStreamsMu.Unlock()

		err := c.Connect()
		if err != nil {
			c.logger.Error("connection failed", "error", err, "attempt", attempt+1)
		} else {
			attempt = 0
			// Block until the connection drops.
			<-c.done
			c.logger.Warn("disconnected from gateway")
		}

		select {
		case <-c.closed:
			return nil
		default:
		}

		attempt++
		if maxRetries > 0 && attempt >= maxRetries {
			return fmt.Errorf("client: max reconnection attempts (%d) reached", maxRetries)
		}

		// Exponential backoff capped at 2^10 × base.
		shift := min(attempt-1, 10)
		wait := time.Duration(backoffMs) * time.Millisecond * time.Duration(int64(1)<<shift)
		c.logger.Info("reconnecting", "wait", wait, "attempt", attempt)

		select {
		case <-time.After(wait):
		case <-c.closed:
			return nil
		}
	}
}

// Close gracefully disconnects the client from the gateway.
func (c *Client) Close() {
	select {
	case <-c.closed:
		return
	default:
		close(c.closed)
	}

	c.wsMu.Lock()
	ws := c.ws
	c.wsMu.Unlock()

	if ws != nil {
		ws.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		ws.Close()
	}
}
