package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenExpired  = errors.New("token expired")
	ErrTokenRevoked  = errors.New("token revoked")
	ErrInvalidToken  = errors.New("invalid token")
	ErrInvalidScope  = errors.New("invalid scope")
)

var validScopes = map[string]bool{
	"create": true,
	"list":   true,
	"revoke": true,
	"admin":  true,
}

// Token represents an authentication token.
type Token struct {
	ID        string
	UserID    string
	Scopes    []string
	CreatedAt time.Time
	ExpiresAt time.Time
	Revoked   bool
}

// TokenStore defines the interface for token management.
type TokenStore interface {
	CreateToken(userID string, scopes []string, ttlMinutes int) (*Token, error)
	ValidateToken(tokenStr string) (*Token, error)
	RevokeToken(id string) error
	ListTokens(userID string) []*Token
	ListAllTokens() []*Token
}

// MemoryTokenStore is a thread-safe in-memory implementation of TokenStore.
type MemoryTokenStore struct {
	mu         sync.RWMutex
	tokens     map[string]*Token // keyed by token ID (which is the full token string)
	signingKey []byte
}

// NewMemoryTokenStore creates a new in-memory token store with the given HMAC signing key.
func NewMemoryTokenStore(signingKey string) *MemoryTokenStore {
	return &MemoryTokenStore{
		tokens:     make(map[string]*Token),
		signingKey: []byte(signingKey),
	}
}

// CreateToken generates a new signed token for the given user.
func (s *MemoryTokenStore) CreateToken(userID string, scopes []string, ttlMinutes int) (*Token, error) {
	for _, scope := range scopes {
		if !validScopes[scope] {
			return nil, fmt.Errorf("%w: %s", ErrInvalidScope, scope)
		}
	}

	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("generating random token: %w", err)
	}
	hexRandom := hex.EncodeToString(randomBytes)

	sig := computeHMAC([]byte(hexRandom), s.signingKey)
	tokenStr := hexRandom + "." + hex.EncodeToString(sig)

	now := time.Now()
	token := &Token{
		ID:        tokenStr,
		UserID:    userID,
		Scopes:    scopes,
		CreatedAt: now,
		ExpiresAt: now.Add(time.Duration(ttlMinutes) * time.Minute),
		Revoked:   false,
	}

	s.mu.Lock()
	s.tokens[token.ID] = token
	s.mu.Unlock()

	return token, nil
}

// ValidateToken verifies the token string signature and checks expiry/revocation.
func (s *MemoryTokenStore) ValidateToken(tokenStr string) (*Token, error) {
	parts := strings.SplitN(tokenStr, ".", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidToken
	}

	hexRandom := parts[0]
	providedSig, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	expectedSig := computeHMAC([]byte(hexRandom), s.signingKey)
	if !hmac.Equal(providedSig, expectedSig) {
		return nil, ErrInvalidToken
	}

	s.mu.RLock()
	token, exists := s.tokens[tokenStr]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrTokenNotFound
	}
	if token.Revoked {
		return nil, ErrTokenRevoked
	}
	if time.Now().After(token.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return token, nil
}

// RevokeToken marks a token as revoked by its ID.
func (s *MemoryTokenStore) RevokeToken(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, exists := s.tokens[id]
	if !exists {
		return ErrTokenNotFound
	}
	token.Revoked = true
	return nil
}

// ListTokens returns all tokens for a given user.
func (s *MemoryTokenStore) ListTokens(userID string) []*Token {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Token
	for _, token := range s.tokens {
		if token.UserID == userID {
			result = append(result, token)
		}
	}
	return result
}

// ListAllTokens returns all tokens in the store.
func (s *MemoryTokenStore) ListAllTokens() []*Token {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Token, 0, len(s.tokens))
	for _, token := range s.tokens {
		result = append(result, token)
	}
	return result
}

func computeHMAC(message, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}
