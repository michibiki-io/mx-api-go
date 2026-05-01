package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const maxIdempotencyKeyLength = 200

type idempotencyStore struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]idempotencyEntry
}

type idempotencyEntry struct {
	requestHash string
	expiresAt   time.Time
	statusCode  int
	response    gin.H
	completed   bool
}

type idempotencyState int

const (
	idempotencyStarted idempotencyState = iota
	idempotencyReplay
	idempotencyConflict
	idempotencyInProgress
)

func newIdempotencyStore(ttl time.Duration) *idempotencyStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &idempotencyStore{
		ttl:     ttl,
		now:     time.Now,
		entries: map[string]idempotencyEntry{},
	}
}

func (s *idempotencyStore) Start(key, requestHash string) (idempotencyState, idempotencyEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if entry, ok := s.entries[key]; ok {
		if now.Before(entry.expiresAt) {
			if entry.requestHash != requestHash {
				return idempotencyConflict, entry
			}
			if entry.completed {
				return idempotencyReplay, entry
			}
			return idempotencyInProgress, entry
		}
		delete(s.entries, key)
	}

	entry := idempotencyEntry{
		requestHash: requestHash,
		expiresAt:   now.Add(s.ttl),
	}
	s.entries[key] = entry
	return idempotencyStarted, entry
}

func (s *idempotencyStore) Complete(key string, statusCode int, response gin.H) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.entries[key]
	entry.statusCode = statusCode
	entry.response = response
	entry.completed = true
	entry.expiresAt = s.now().Add(s.ttl)
	s.entries[key] = entry
}

func (s *idempotencyStore) Forget(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

func (h *Handler) startIdempotency(c *gin.Context, values map[string]string) (string, bool) {
	if !h.cfg.Security.Idempotency.Enabled {
		return "", true
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		return "", true
	}
	if !validIdempotencyKey(key) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"status": "BadRequest", "errors": "Invalid Idempotency-Key"})
		return "", false
	}

	cacheKey := c.Request.Method + "|" + c.FullPath() + "|" + key
	state, entry := h.idempotency.Start(cacheKey, requestHash(values))
	switch state {
	case idempotencyStarted:
		return cacheKey, true
	case idempotencyReplay:
		c.AbortWithStatusJSON(entry.statusCode, entry.response)
		return "", false
	case idempotencyConflict:
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"status": "Conflict", "errors": "Idempotency-Key was reused with a different request"})
		return "", false
	case idempotencyInProgress:
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"status": "Conflict", "errors": "Idempotency-Key is already processing"})
		return "", false
	default:
		return "", true
	}
}

func validIdempotencyKey(key string) bool {
	if len(key) > maxIdempotencyKeyLength || strings.ContainsAny(key, "\r\n") {
		return false
	}
	return true
}

func requestHash(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	hasher := sha256.New()
	for _, key := range keys {
		hasher.Write([]byte(key))
		hasher.Write([]byte{0})
		hasher.Write([]byte(values[key]))
		hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
