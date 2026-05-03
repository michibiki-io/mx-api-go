package httpapi

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

const dashboardTokenHeader = "X-MX-API-Dashboard-Token"

type dashboardTokenStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	tokens map[string]time.Time
}

func newDashboardTokenStore(ttl time.Duration) *dashboardTokenStore {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &dashboardTokenStore{
		ttl:    ttl,
		tokens: make(map[string]time.Time),
	}
}

func (s *dashboardTokenStore) Issue() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ""
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	token := fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	s.tokens[token] = now.Add(s.ttl)
	return token
}

func (s *dashboardTokenStore) Valid(token string) bool {
	if token == "" {
		return false
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	expiresAt, ok := s.tokens[token]
	return ok && now.Before(expiresAt)
}

func (s *dashboardTokenStore) pruneLocked(now time.Time) {
	for token, expiresAt := range s.tokens {
		if !now.Before(expiresAt) {
			delete(s.tokens, token)
		}
	}
}
