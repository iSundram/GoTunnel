package registry

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestRegistry() *MemoryRegistry {
	return NewMemoryRegistry([]string{"api", "admin", "www"}, 20, 60)
}

func newTunnel(id, owner, subdomain string) *Tunnel {
	return &Tunnel{
		ID:              id,
		OwnerID:         owner,
		Subdomain:       subdomain,
		Protocol:        "http",
		TargetPort:      8080,
		CreatedAt:       time.Now(),
		LastHeartbeatAt: time.Now(),
		ConnID:          "conn-" + id,
	}
}

func TestRegister_Success(t *testing.T) {
	r := newTestRegistry()
	tun := newTunnel("t1", "user1", "myapp")
	if err := r.Register(tun); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if tun.TTL != 60 {
		t.Fatalf("TTL not set to default: got %d", tun.TTL)
	}
}

func TestRegister_CustomTTL(t *testing.T) {
	r := newTestRegistry()
	tun := newTunnel("t1", "user1", "myapp")
	tun.TTL = 120
	if err := r.Register(tun); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if tun.TTL != 120 {
		t.Fatalf("custom TTL overwritten: got %d", tun.TTL)
	}
}

func TestRegister_ReservedSubdomain(t *testing.T) {
	r := newTestRegistry()
	tun := newTunnel("t1", "user1", "api")
	err := r.Register(tun)
	if !errors.Is(err, ErrSubdomainReserved) {
		t.Fatalf("expected ErrSubdomainReserved, got %v", err)
	}
}

func TestRegister_SubdomainTaken(t *testing.T) {
	r := newTestRegistry()
	r.Register(newTunnel("t1", "user1", "myapp"))
	err := r.Register(newTunnel("t2", "user2", "myapp"))
	if !errors.Is(err, ErrSubdomainTaken) {
		t.Fatalf("expected ErrSubdomainTaken, got %v", err)
	}
}

func TestRegister_SubdomainTooLong(t *testing.T) {
	r := newTestRegistry()
	tun := newTunnel("t1", "user1", strings.Repeat("a", 21))
	err := r.Register(tun)
	if !errors.Is(err, ErrSubdomainTooLong) {
		t.Fatalf("expected ErrSubdomainTooLong, got %v", err)
	}
}

func TestRegister_SubdomainExactMaxLength(t *testing.T) {
	r := newTestRegistry()
	tun := newTunnel("t1", "user1", strings.Repeat("a", 20))
	if err := r.Register(tun); err != nil {
		t.Fatalf("Register with exact max length: %v", err)
	}
}

func TestGetByID(t *testing.T) {
	r := newTestRegistry()
	r.Register(newTunnel("t1", "user1", "myapp"))

	tun, err := r.GetByID("t1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if tun.ID != "t1" {
		t.Fatalf("wrong tunnel: %s", tun.ID)
	}

	_, err = r.GetByID("nonexistent")
	if !errors.Is(err, ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound, got %v", err)
	}
}

func TestGetBySubdomain(t *testing.T) {
	r := newTestRegistry()
	r.Register(newTunnel("t1", "user1", "myapp"))

	tun, err := r.GetBySubdomain("myapp")
	if err != nil {
		t.Fatalf("GetBySubdomain: %v", err)
	}
	if tun.Subdomain != "myapp" {
		t.Fatalf("wrong subdomain: %s", tun.Subdomain)
	}

	_, err = r.GetBySubdomain("nope")
	if !errors.Is(err, ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound, got %v", err)
	}
}

func TestGetByOwner(t *testing.T) {
	r := newTestRegistry()
	r.Register(newTunnel("t1", "user1", "app1"))
	r.Register(newTunnel("t2", "user1", "app2"))
	r.Register(newTunnel("t3", "user2", "app3"))

	tunnels, err := r.GetByOwner("user1")
	if err != nil {
		t.Fatalf("GetByOwner: %v", err)
	}
	if len(tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(tunnels))
	}

	tunnels, err = r.GetByOwner("nobody")
	if err != nil {
		t.Fatalf("GetByOwner: %v", err)
	}
	if len(tunnels) != 0 {
		t.Fatalf("expected 0 tunnels, got %d", len(tunnels))
	}
}

func TestUnregister(t *testing.T) {
	r := newTestRegistry()
	r.Register(newTunnel("t1", "user1", "myapp"))

	if err := r.Unregister("t1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	_, err := r.GetByID("t1")
	if !errors.Is(err, ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound after unregister, got %v", err)
	}
	_, err = r.GetBySubdomain("myapp")
	if !errors.Is(err, ErrTunnelNotFound) {
		t.Fatalf("subdomain index not cleaned after unregister")
	}

	err = r.Unregister("t1")
	if !errors.Is(err, ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound for double unregister, got %v", err)
	}
}

func TestUnregister_FreesSubdomain(t *testing.T) {
	r := newTestRegistry()
	r.Register(newTunnel("t1", "user1", "myapp"))
	r.Unregister("t1")

	// Subdomain should be available again
	if err := r.Register(newTunnel("t2", "user2", "myapp")); err != nil {
		t.Fatalf("re-register freed subdomain: %v", err)
	}
}

func TestUpdateHeartbeat(t *testing.T) {
	r := newTestRegistry()
	tun := newTunnel("t1", "user1", "myapp")
	tun.LastHeartbeatAt = time.Now().Add(-1 * time.Hour)
	r.Register(tun)

	before := tun.LastHeartbeatAt
	if err := r.UpdateHeartbeat("t1"); err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}
	if !tun.LastHeartbeatAt.After(before) {
		t.Fatal("heartbeat not updated")
	}

	err := r.UpdateHeartbeat("nonexistent")
	if !errors.Is(err, ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound, got %v", err)
	}
}

func TestCleanExpired(t *testing.T) {
	r := newTestRegistry()

	alive := newTunnel("alive", "user1", "alive")
	alive.TTL = 3600
	r.Register(alive)

	expired := newTunnel("expired", "user1", "expired")
	expired.TTL = 1
	expired.LastHeartbeatAt = time.Now().Add(-10 * time.Second)
	r.Register(expired)

	noTTL := newTunnel("nottl", "user1", "nottl")
	noTTL.TTL = 0
	r.Register(noTTL)

	count := r.CleanExpired()
	if count != 1 {
		t.Fatalf("expected 1 expired, cleaned %d", count)
	}

	if _, err := r.GetByID("alive"); err != nil {
		t.Fatal("alive tunnel should still exist")
	}
	if _, err := r.GetByID("expired"); !errors.Is(err, ErrTunnelNotFound) {
		t.Fatal("expired tunnel should be removed")
	}
	if _, err := r.GetByID("nottl"); err != nil {
		t.Fatal("zero-TTL tunnel should still exist")
	}
}

func TestList(t *testing.T) {
	r := newTestRegistry()
	if l := r.List(); len(l) != 0 {
		t.Fatalf("expected empty list, got %d", len(l))
	}

	r.Register(newTunnel("t1", "u1", "a"))
	r.Register(newTunnel("t2", "u1", "b"))
	if l := r.List(); len(l) != 2 {
		t.Fatalf("expected 2, got %d", len(l))
	}
}

func TestIsSubdomainReserved(t *testing.T) {
	r := newTestRegistry()
	if !r.IsSubdomainReserved("api") {
		t.Fatal("api should be reserved")
	}
	if r.IsSubdomainReserved("myapp") {
		t.Fatal("myapp should not be reserved")
	}
}

func TestGetByConnID(t *testing.T) {
	r := newTestRegistry()
	r.Register(newTunnel("t1", "u1", "a"))
	r.Register(newTunnel("t2", "u1", "b"))

	tunnels, err := r.GetByConnID("conn-t1")
	if err != nil {
		t.Fatalf("GetByConnID: %v", err)
	}
	if len(tunnels) != 1 || tunnels[0].ID != "t1" {
		t.Fatalf("unexpected result: %v", tunnels)
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := newTestRegistry()
	var wg sync.WaitGroup
	n := 100

	// Concurrent registrations
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id := "t" + strings.Repeat("x", 0) + string(rune('A'+i%26)) + strings.Repeat("0", 0)
			tun := newTunnel(
				"id-"+string(rune(i)),
				"user1",
				"sub-"+string(rune(i)),
			)
			tun.ID = id
			tun.Subdomain = "sd" + id
			r.Register(tun)
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			r.List()
			r.CleanExpired()
		}()
	}
	wg.Wait()
}
