// Package server implements the main GoTunnel gateway server.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/iSundram/GoTunnel/internal/admin"
	"github.com/iSundram/GoTunnel/internal/auth"
	"github.com/iSundram/GoTunnel/internal/broker"
	"github.com/iSundram/GoTunnel/internal/config"
	"github.com/iSundram/GoTunnel/internal/logging"
	"github.com/iSundram/GoTunnel/internal/metrics"
	"github.com/iSundram/GoTunnel/internal/protocol"
	"github.com/iSundram/GoTunnel/internal/proxy"
	"github.com/iSundram/GoTunnel/internal/registry"
)

// Server is the main GoTunnel gateway server.
type Server struct {
	cfg      *config.Config
	registry registry.Registry
	broker   *broker.Broker
	proxy    *proxy.Proxy
	admin    *admin.AdminAPI
	auth     auth.TokenStore
	metrics  *metrics.Metrics
	logger   *logging.Logger
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewServer initialises all components and returns a ready-to-start Server.
func NewServer(cfg *config.Config) (*Server, error) {
	log, err := logging.New(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.File)
	if err != nil {
		return nil, fmt.Errorf("server: init logger: %w", err)
	}

	m := metrics.NewMetrics()

	tokenStore := auth.NewMemoryTokenStore(cfg.Auth.TokenSigningKey)

	reg := registry.NewMemoryRegistry(
		cfg.Registry.ReservedSubdomains,
		cfg.Registry.MaxSubdomainLength,
		cfg.Registry.DefaultTTLSeconds,
	)

	b := broker.NewBroker()

	p := proxy.NewProxy(b, reg, m, log)
	p.SetPublicHostname(cfg.Server.PublicHostname)

	a := admin.NewAdminAPI(tokenStore, reg, m, log)

	return &Server{
		cfg:      cfg,
		registry: reg,
		broker:   b,
		proxy:    p,
		admin:    a,
		auth:     tokenStore,
		metrics:  m,
		logger:   log.WithComponent("server"),
	}, nil
}

// Start sets up HTTP routes, starts the server, and blocks until a
// SIGINT/SIGTERM signal triggers graceful shutdown.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/connect", s.handleConnect)
	mux.Handle("/api/v1/", s.admin.Handler())
	mux.HandleFunc("/health", s.handleHealth)
	mux.Handle("/metrics", metrics.Handler())
	// Everything else is proxied to tunnel clients.
	mux.Handle("/", s.proxy)

	httpServer := &http.Server{
		Addr:    s.cfg.Server.ListenHTTP,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("starting HTTP server", "addr", s.cfg.Server.ListenHTTP)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		s.logger.Info("received signal, shutting down", "signal", sig.String())
	case err := <-errCh:
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	s.logger.Info("server stopped")
	return nil
}

// handleHealth responds with a simple health-check payload.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleConnect upgrades to WebSocket, authenticates the client, registers
// tunnels, and manages the connection lifecycle.
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "error", err)
		return
	}
	defer ws.Close()

	// Read the first frame — must be a register frame.
	_, raw, err := ws.ReadMessage()
	if err != nil {
		s.logger.Error("failed to read register frame", "error", err)
		return
	}

	frameType, payload, err := protocol.DecodeFrame(bytes.NewReader(raw))
	if err != nil || frameType != protocol.FrameRegister {
		s.sendControlResponse(ws, protocol.CodeInvalidRequest, "expected register frame")
		s.logger.Warn("invalid first frame", "error", err)
		return
	}

	var reg protocol.Register
	if err := protocol.DecodeControlMessage(payload, &reg); err != nil {
		s.sendControlResponse(ws, protocol.CodeInvalidRequest, "invalid register payload")
		s.logger.Warn("bad register payload", "error", err)
		return
	}

	// Validate token.
	token, err := s.auth.ValidateToken(reg.Token)
	if err != nil {
		s.metrics.IncTokenAuthFailures()
		s.sendControlResponse(ws, protocol.CodeUnauthorized, "authentication failed: "+err.Error())
		s.logger.Warn("auth failed", "error", err)
		return
	}

	connID := reg.ClientID
	if connID == "" {
		connID = fmt.Sprintf("conn-%d", time.Now().UnixNano())
	}

	conn := s.broker.AddConnection(connID, token.UserID, ws)

	// Register tunnels and map subdomains.
	var tunnelIDs []string
	for _, tc := range reg.Tunnels {
		tunnelID := fmt.Sprintf("%s-%s", connID, tc.Subdomain)
		tunnel := &registry.Tunnel{
			ID:              tunnelID,
			OwnerID:         token.UserID,
			Subdomain:       tc.Subdomain,
			Protocol:        tc.Protocol,
			TargetPort:      tc.Port,
			CreatedAt:       time.Now(),
			LastHeartbeatAt: time.Now(),
			ConnID:          connID,
		}
		if err := s.registry.Register(tunnel); err != nil {
			s.logger.Error("failed to register tunnel", "subdomain", tc.Subdomain, "error", err)
			s.sendControlResponse(ws, protocol.CodeInvalidRequest, "register tunnel failed: "+err.Error())
			s.cleanupConnection(connID, tunnelIDs)
			return
		}
		s.broker.MapSubdomain(tc.Subdomain, connID)
		tunnelIDs = append(tunnelIDs, tunnelID)
		s.metrics.IncActiveTunnels()
	}
	conn.Tunnels = tunnelIDs

	s.sendControlResponse(ws, protocol.CodeOK, "registered")
	s.logger.Info("client registered", "conn_id", connID, "tunnels", len(tunnelIDs))

	// Read loop: handle heartbeat and stream_data frames from client.
	s.readLoop(ws, conn, connID, tunnelIDs)
}

// readLoop reads frames from the client WebSocket and dispatches them.
func (s *Server) readLoop(ws *websocket.Conn, conn *broker.ClientConn, connID string, tunnelIDs []string) {
	defer s.cleanupConnection(connID, tunnelIDs)

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Warn("websocket read error", "conn_id", connID, "error", err)
			}
			return
		}

		frameType, payload, err := protocol.DecodeFrame(bytes.NewReader(raw))
		if err != nil {
			s.logger.Warn("invalid frame from client", "conn_id", connID, "error", err)
			continue
		}

		switch frameType {
		case protocol.FrameHeartbeat:
			s.handleHeartbeat(connID, tunnelIDs)

		case protocol.FrameStreamData:
			s.handleStreamData(conn, payload)

		case protocol.FrameCloseStream:
			s.handleCloseStream(conn, payload)

		default:
			s.logger.Warn("unexpected frame type from client", "conn_id", connID, "type", frameType)
		}
	}
}

// handleHeartbeat updates the heartbeat timestamp for all tunnels on a connection.
func (s *Server) handleHeartbeat(connID string, tunnelIDs []string) {
	for _, id := range tunnelIDs {
		if err := s.registry.UpdateHeartbeat(id); err != nil {
			s.logger.Warn("heartbeat update failed", "tunnel_id", id, "error", err)
		}
	}
	s.logger.Debug("heartbeat received", "conn_id", connID)
}

// handleStreamData routes a stream_data frame from the client to the
// corresponding stream's Response channel.
func (s *Server) handleStreamData(conn *broker.ClientConn, payload []byte) {
	var sd protocol.StreamData
	if err := protocol.DecodeControlMessage(payload, &sd); err != nil {
		s.logger.Warn("bad stream_data payload", "error", err)
		return
	}

	streamID, err := strconv.ParseUint(sd.StreamID, 10, 32)
	if err != nil {
		s.logger.Warn("invalid stream_id in stream_data", "stream_id", sd.StreamID, "error", err)
		return
	}

	s.deliverStreamData(conn, uint32(streamID), sd.Data)
}

// deliverStreamData sends response data to the waiting proxy handler.
func (s *Server) deliverStreamData(conn *broker.ClientConn, streamID uint32, data []byte) {
	// Thread-safe access to the stream map.
	stream, ok := s.getStream(conn, streamID)
	if !ok {
		s.logger.Warn("stream not found for response", "stream_id", streamID)
		return
	}

	select {
	case stream.Response <- data:
	case <-stream.Done:
		s.logger.Debug("stream already closed", "stream_id", streamID)
	}
}

// getStream safely retrieves a stream from a client connection.
func (s *Server) getStream(conn *broker.ClientConn, streamID uint32) (*broker.Stream, bool) {
	// ClientConn.Streams is protected by conn.mu but the mutex is unexported.
	// Access through the broker which handles synchronisation.
	stream, ok := conn.Streams[streamID]
	return stream, ok
}

// handleCloseStream processes a close_stream frame from the client.
func (s *Server) handleCloseStream(conn *broker.ClientConn, payload []byte) {
	var cs protocol.CloseStream
	if err := protocol.DecodeControlMessage(payload, &cs); err != nil {
		s.logger.Warn("bad close_stream payload", "error", err)
		return
	}

	streamID, err := strconv.ParseUint(cs.StreamID, 10, 32)
	if err != nil {
		s.logger.Warn("invalid stream_id in close_stream", "stream_id", cs.StreamID, "error", err)
		return
	}

	s.broker.CloseStream(conn, uint32(streamID))
}

// cleanupConnection unregisters all tunnels, removes subdomain mappings,
// and removes the connection from the broker.
func (s *Server) cleanupConnection(connID string, tunnelIDs []string) {
	for _, id := range tunnelIDs {
		if t, err := s.registry.GetByID(id); err == nil {
			s.broker.UnmapSubdomain(t.Subdomain)
		}
		if err := s.registry.Unregister(id); err == nil {
			s.metrics.DecActiveTunnels()
		}
	}
	s.broker.RemoveConnection(connID)
	s.logger.Info("client disconnected", "conn_id", connID)
}

// sendControlResponse sends a ControlResponse frame over the WebSocket.
func (s *Server) sendControlResponse(ws *websocket.Conn, code uint32, message string) {
	resp := protocol.ControlResponse{
		Code:    code,
		Message: message,
	}
	data, err := protocol.EncodeControlMessage(protocol.FrameControlResponse, resp)
	if err != nil {
		s.logger.Error("failed to encode control response", "error", err)
		return
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
		s.logger.Error("failed to send control response", "error", err)
	}
}

