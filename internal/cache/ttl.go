package cache

import (
	"sync"
	"time"
)

type Entry[V any] struct {
	Value     V
	ExpiresAt time.Time
}

type TTL[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]Entry[V]
	order   []K // insertion order for eviction (oldest first)
	maxSize int // 0 = unlimited
}

func NewTTL[K comparable, V any]() *TTL[K, V] {
	return &TTL[K, V]{entries: make(map[K]Entry[V])}
}

func NewTTLWithMax[K comparable, V any](maxSize int) *TTL[K, V] {
	return &TTL[K, V]{
		entries: make(map[K]Entry[V]),
		maxSize: maxSize,
	}
}

func (c *TTL[K, V]) Get(key K) (V, time.Time, bool) {
	var zero V
	if c == nil {
		return zero, time.Time{}, false
	}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return zero, time.Time{}, false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.removeFromOrder(key)
		c.mu.Unlock()
		return zero, time.Time{}, false
	}
	return entry.Value, entry.ExpiresAt, true
}

func (c *TTL[K, V]) Set(key K, value V, ttl time.Duration) time.Time {
	if c == nil {
		return time.Time{}
	}

	expiresAt := time.Now().Add(ttl)
	c.mu.Lock()
	_, isExisting := c.entries[key]
	if !isExisting {
		// Evict oldest entry when at capacity.
		if c.maxSize > 0 && len(c.entries) >= c.maxSize && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, key)
	}
	c.entries[key] = Entry[V]{
		Value:     value,
		ExpiresAt: expiresAt,
	}
	c.mu.Unlock()
	return expiresAt
}

func (c *TTL[K, V]) Delete(key K) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.removeFromOrder(key)
	c.mu.Unlock()
}

func (c *TTL[K, V]) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[K]Entry[V])
	c.order = nil
	c.mu.Unlock()
}

// Size returns the number of entries currently in the cache (including expired ones not yet evicted).
func (c *TTL[K, V]) Size() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	n := len(c.entries)
	c.mu.RUnlock()
	return n
}

// removeFromOrder removes a key from the order slice. Must be called with mu held.
func (c *TTL[K, V]) removeFromOrder(key K) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}
