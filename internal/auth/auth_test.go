package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const testKey = "test-signing-key-secret"

func TestCreateToken_Success(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	tok, err := store.CreateToken("user1", []string{"create", "list"}, 60)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok.UserID != "user1" {
		t.Fatalf("UserID: got %s, want user1", tok.UserID)
	}
	if len(tok.Scopes) != 2 {
		t.Fatalf("Scopes length: got %d, want 2", len(tok.Scopes))
	}
	if tok.Revoked {
		t.Fatal("new token should not be revoked")
	}
	if tok.ID == "" {
		t.Fatal("token ID should not be empty")
	}
	if !strings.Contains(tok.ID, ".") {
		t.Fatal("token ID should contain a dot separator")
	}
	if tok.ExpiresAt.Before(tok.CreatedAt) {
		t.Fatal("ExpiresAt should be after CreatedAt")
	}
}

func TestCreateToken_InvalidScope(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	_, err := store.CreateToken("user1", []string{"create", "invalid_scope"}, 60)
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
	if !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("expected ErrInvalidScope, got: %v", err)
	}
}

func TestValidateToken_Valid(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	tok, _ := store.CreateToken("user1", []string{"create"}, 60)

	validated, err := store.ValidateToken(tok.ID)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if validated.UserID != "user1" {
		t.Fatalf("UserID mismatch: got %s", validated.UserID)
	}
}

func TestValidateToken_Expired(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	tok, _ := store.CreateToken("user1", []string{"create"}, 1)

	// Manually expire the token
	store.mu.Lock()
	store.tokens[tok.ID].ExpiresAt = time.Now().Add(-1 * time.Minute)
	store.mu.Unlock()

	_, err := store.ValidateToken(tok.ID)
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestValidateToken_Revoked(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	tok, _ := store.CreateToken("user1", []string{"create"}, 60)

	store.RevokeToken(tok.ID)

	_, err := store.ValidateToken(tok.ID)
	if !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("expected ErrTokenRevoked, got: %v", err)
	}
}

func TestValidateToken_InvalidFormat(t *testing.T) {
	store := NewMemoryTokenStore(testKey)

	_, err := store.ValidateToken("no-dot-separator")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestValidateToken_TamperedSignature(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	tok, _ := store.CreateToken("user1", []string{"create"}, 60)

	parts := strings.SplitN(tok.ID, ".", 2)
	tampered := parts[0] + ".0000000000000000000000000000000000000000000000000000000000000000"

	_, err := store.ValidateToken(tampered)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for tampered signature, got: %v", err)
	}
}

func TestValidateToken_TamperedPayload(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	tok, _ := store.CreateToken("user1", []string{"create"}, 60)

	parts := strings.SplitN(tok.ID, ".", 2)
	tampered := "aa" + parts[0][2:] + "." + parts[1]

	_, err := store.ValidateToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered payload")
	}
}

func TestValidateToken_InvalidHexSignature(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	_, err := store.ValidateToken("abcdef.not-valid-hex!")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestValidateToken_NotFound(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	// Create a token with one store, then validate with a store that has the same key
	// but the token was never stored
	store2 := NewMemoryTokenStore(testKey)
	tok, _ := store.CreateToken("user1", []string{"create"}, 60)

	_, err := store2.ValidateToken(tok.ID)
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound, got: %v", err)
	}
}

func TestRevokeToken(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	tok, _ := store.CreateToken("user1", []string{"create"}, 60)

	if err := store.RevokeToken(tok.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// Revoke again should still succeed (already revoked)
	store.mu.RLock()
	if !store.tokens[tok.ID].Revoked {
		t.Fatal("token should be revoked")
	}
	store.mu.RUnlock()
}

func TestRevokeToken_NotFound(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	err := store.RevokeToken("nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound, got: %v", err)
	}
}

func TestListTokens(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	store.CreateToken("user1", []string{"create"}, 60)
	store.CreateToken("user1", []string{"list"}, 60)
	store.CreateToken("user2", []string{"admin"}, 60)

	tokens := store.ListTokens("user1")
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens for user1, got %d", len(tokens))
	}

	tokens = store.ListTokens("user2")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token for user2, got %d", len(tokens))
	}

	tokens = store.ListTokens("nobody")
	if len(tokens) != 0 {
		t.Fatalf("expected 0 tokens for nobody, got %d", len(tokens))
	}
}

func TestListAllTokens(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	store.CreateToken("user1", []string{"create"}, 60)
	store.CreateToken("user2", []string{"list"}, 60)

	all := store.ListAllTokens()
	if len(all) != 2 {
		t.Fatalf("expected 2 tokens total, got %d", len(all))
	}
}

func TestHasScope(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	tok, _ := store.CreateToken("user1", []string{"create", "list"}, 60)

	hasCreate := false
	hasAdmin := false
	for _, s := range tok.Scopes {
		if s == "create" {
			hasCreate = true
		}
		if s == "admin" {
			hasAdmin = true
		}
	}
	if !hasCreate {
		t.Fatal("token should have 'create' scope")
	}
	if hasAdmin {
		t.Fatal("token should not have 'admin' scope")
	}
}

func TestAllValidScopes(t *testing.T) {
	store := NewMemoryTokenStore(testKey)
	scopes := []string{"create", "list", "revoke", "admin"}
	tok, err := store.CreateToken("user1", scopes, 60)
	if err != nil {
		t.Fatalf("CreateToken with all valid scopes: %v", err)
	}
	if len(tok.Scopes) != 4 {
		t.Fatalf("expected 4 scopes, got %d", len(tok.Scopes))
	}
}

func TestDifferentSigningKeys(t *testing.T) {
	store1 := NewMemoryTokenStore("key-one")
	store2 := NewMemoryTokenStore("key-two")

	tok, _ := store1.CreateToken("user1", []string{"create"}, 60)
	// Token signed with key-one should fail validation against key-two
	_, err := store2.ValidateToken(tok.ID)
	if err == nil {
		t.Fatal("expected error when validating with different signing key")
	}
}
