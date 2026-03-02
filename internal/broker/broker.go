package broker

import (
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// Stream represents an active request/response stream within a client connection.
type Stream struct {
	ID       uint32
	TunnelID string
	Request  chan []byte
	Response chan []byte
	Done     chan struct{}
}

// ClientConn represents an active client connection.
type ClientConn struct {
	ConnID  string
	UserID  string
	WS      *websocket.Conn
	Tunnels []string
	Streams map[uint32]*Stream
	mu      sync.Mutex
	sendMu  sync.Mutex
	Done    chan struct{}
}

// Broker manages active client connections and routes incoming requests by subdomain.
type Broker struct {
	connections    map[string]*ClientConn
	subdomainToConn map[string]string
	nextStreamID   atomic.Uint32
	mu             sync.RWMutex
}

// NewBroker creates and returns a new Broker.
func NewBroker() *Broker {
	return &Broker{
		connections:    make(map[string]*ClientConn),
		subdomainToConn: make(map[string]string),
	}
}

// AddConnection registers a new client connection with the broker.
func (b *Broker) AddConnection(connID, userID string, ws *websocket.Conn) *ClientConn {
	conn := &ClientConn{
		ConnID:  connID,
		UserID:  userID,
		WS:      ws,
		Tunnels: []string{},
		Streams: make(map[uint32]*Stream),
		Done:    make(chan struct{}),
	}
	b.mu.Lock()
	b.connections[connID] = conn
	b.mu.Unlock()
	return conn
}

// RemoveConnection unregisters a client connection and cleans up its subdomain mappings.
func (b *Broker) RemoveConnection(connID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	conn, ok := b.connections[connID]
	if !ok {
		return
	}

	for sub, cid := range b.subdomainToConn {
		if cid == connID {
			delete(b.subdomainToConn, sub)
		}
	}

	conn.mu.Lock()
	for _, s := range conn.Streams {
		select {
		case <-s.Done:
		default:
			close(s.Done)
		}
	}
	conn.mu.Unlock()

	delete(b.connections, connID)
}

// MapSubdomain associates a subdomain with a connection ID for routing.
func (b *Broker) MapSubdomain(subdomain, connID string) {
	b.mu.Lock()
	b.subdomainToConn[subdomain] = connID
	b.mu.Unlock()
}

// UnmapSubdomain removes a subdomain routing entry.
func (b *Broker) UnmapSubdomain(subdomain string) {
	b.mu.Lock()
	delete(b.subdomainToConn, subdomain)
	b.mu.Unlock()
}

// GetConnectionBySubdomain looks up the client connection for a given subdomain.
func (b *Broker) GetConnectionBySubdomain(subdomain string) (*ClientConn, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	connID, ok := b.subdomainToConn[subdomain]
	if !ok {
		return nil, false
	}
	conn, ok := b.connections[connID]
	return conn, ok
}

// GetConnection retrieves a client connection by its connection ID.
func (b *Broker) GetConnection(connID string) (*ClientConn, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	conn, ok := b.connections[connID]
	return conn, ok
}

// OpenStream creates a new stream on the given client connection.
func (b *Broker) OpenStream(conn *ClientConn, tunnelID string) *Stream {
	id := b.nextStreamID.Add(1)
	s := &Stream{
		ID:       id,
		TunnelID: tunnelID,
		Request:  make(chan []byte, 1),
		Response: make(chan []byte, 1),
		Done:     make(chan struct{}),
	}
	conn.mu.Lock()
	conn.Streams[id] = s
	conn.mu.Unlock()
	return s
}

// CloseStream removes and closes a stream on the given client connection.
func (b *Broker) CloseStream(conn *ClientConn, streamID uint32) {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	s, ok := conn.Streams[streamID]
	if !ok {
		return
	}
	select {
	case <-s.Done:
	default:
		close(s.Done)
	}
	delete(conn.Streams, streamID)
}

// SendMessage performs a thread-safe WebSocket write on the client connection.
func (b *Broker) SendMessage(conn *ClientConn, data []byte) error {
	conn.sendMu.Lock()
	defer conn.sendMu.Unlock()
	return conn.WS.WriteMessage(websocket.BinaryMessage, data)
}
