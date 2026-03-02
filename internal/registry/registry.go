package registry

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrSubdomainReserved = errors.New("subdomain is reserved")
	ErrSubdomainTaken    = errors.New("subdomain is already taken")
	ErrSubdomainTooLong  = errors.New("subdomain exceeds maximum length")
	ErrTunnelNotFound    = errors.New("tunnel not found")
)

// Tunnel represents a registered tunnel connection.
type Tunnel struct {
	ID              string    `json:"id"`
	OwnerID         string    `json:"owner_id"`
	Subdomain       string    `json:"subdomain"`
	Protocol        string    `json:"protocol"` // "http" | "tcp"
	TargetPort      int       `json:"target_port"`
	CreatedAt       time.Time `json:"created_at"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	ConnID          string    `json:"conn_id"`
	TTL             int       `json:"ttl_seconds"`
	BandwidthLimit  int       `json:"bandwidth_limit_mbps"`
}

// Registry defines operations for managing tunnels.
type Registry interface {
	Register(tunnel *Tunnel) error
	Unregister(id string) error
	GetByID(id string) (*Tunnel, error)
	GetBySubdomain(subdomain string) (*Tunnel, error)
	GetByOwner(ownerID string) ([]*Tunnel, error)
	GetByConnID(connID string) ([]*Tunnel, error)
	List() []*Tunnel
	UpdateHeartbeat(id string) error
	IsSubdomainReserved(subdomain string) bool
	CleanExpired() int
}

// MemoryRegistry is a thread-safe in-memory implementation of Registry.
type MemoryRegistry struct {
	mu                 sync.RWMutex
	tunnels            map[string]*Tunnel // keyed by ID
	subdomainIndex     map[string]string  // subdomain -> tunnel ID
	reservedSubdomains map[string]struct{}
	maxSubdomainLength int
	defaultTTL         int
}

// NewMemoryRegistry creates a new MemoryRegistry.
func NewMemoryRegistry(reservedSubdomains []string, maxSubdomainLength int, defaultTTL int) *MemoryRegistry {
	reserved := make(map[string]struct{}, len(reservedSubdomains))
	for _, s := range reservedSubdomains {
		reserved[s] = struct{}{}
	}
	return &MemoryRegistry{
		tunnels:            make(map[string]*Tunnel),
		subdomainIndex:     make(map[string]string),
		reservedSubdomains: reserved,
		maxSubdomainLength: maxSubdomainLength,
		defaultTTL:         defaultTTL,
	}
}

func (r *MemoryRegistry) Register(tunnel *Tunnel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.reservedSubdomains[tunnel.Subdomain]; ok {
		return ErrSubdomainReserved
	}
	if r.maxSubdomainLength > 0 && len(tunnel.Subdomain) > r.maxSubdomainLength {
		return ErrSubdomainTooLong
	}
	if _, ok := r.subdomainIndex[tunnel.Subdomain]; ok {
		return ErrSubdomainTaken
	}

	if tunnel.TTL == 0 {
		tunnel.TTL = r.defaultTTL
	}

	r.tunnels[tunnel.ID] = tunnel
	r.subdomainIndex[tunnel.Subdomain] = tunnel.ID
	return nil
}

func (r *MemoryRegistry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tunnels[id]
	if !ok {
		return ErrTunnelNotFound
	}
	delete(r.subdomainIndex, t.Subdomain)
	delete(r.tunnels, id)
	return nil
}

func (r *MemoryRegistry) GetByID(id string) (*Tunnel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tunnels[id]
	if !ok {
		return nil, ErrTunnelNotFound
	}
	return t, nil
}

func (r *MemoryRegistry) GetBySubdomain(subdomain string) (*Tunnel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.subdomainIndex[subdomain]
	if !ok {
		return nil, ErrTunnelNotFound
	}
	return r.tunnels[id], nil
}

func (r *MemoryRegistry) GetByOwner(ownerID string) ([]*Tunnel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Tunnel
	for _, t := range r.tunnels {
		if t.OwnerID == ownerID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (r *MemoryRegistry) GetByConnID(connID string) ([]*Tunnel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Tunnel
	for _, t := range r.tunnels {
		if t.ConnID == connID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (r *MemoryRegistry) List() []*Tunnel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Tunnel, 0, len(r.tunnels))
	for _, t := range r.tunnels {
		result = append(result, t)
	}
	return result
}

func (r *MemoryRegistry) UpdateHeartbeat(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tunnels[id]
	if !ok {
		return ErrTunnelNotFound
	}
	t.LastHeartbeatAt = time.Now()
	return nil
}

func (r *MemoryRegistry) IsSubdomainReserved(subdomain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.reservedSubdomains[subdomain]
	return ok
}

func (r *MemoryRegistry) CleanExpired() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	count := 0
	for id, t := range r.tunnels {
		if t.TTL > 0 && now.Sub(t.LastHeartbeatAt) > time.Duration(t.TTL)*time.Second {
			delete(r.subdomainIndex, t.Subdomain)
			delete(r.tunnels, id)
			count++
		}
	}
	return count
}
