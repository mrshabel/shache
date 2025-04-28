package cache

import (
	"log"
	"sync"
	"time"
)

type CacheEntry struct {
	Value any
	TTL   time.Duration
}

// Cache is a concurrent and thread-safe cache which implements the Cacher interface
type Cache[K comparable] struct {
	mu       sync.RWMutex
	data     map[K]CacheEntry
	capacity int
}

func NewCache[K comparable](entryLimit int) *Cache[K] {
	// TODO: monitor capacity and remove stale entries in the background
	return &Cache[K]{
		capacity: entryLimit,
		data:     make(map[K]CacheEntry),
	}
}

func (c *Cache[K]) Put(key K, value CacheEntry) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// check if value exist in cache
	_, ok := c.data[key]

	// update cache
	c.data[key] = value

	// setup background goroutine to expire value
	if value.TTL > 0 {
		go func() {
			time.Sleep(value.TTL)
			c.mu.Lock()
			log.Printf("%v evicted...", key)
			delete(c.data, key)
			c.mu.Unlock()
		}()
	}
	return ok
}

func (c *Cache[K]) Get(key K) (CacheEntry, bool) {
	// retrieve value with a shared-lock
	c.mu.RLock()
	defer c.mu.RUnlock()

	val, ok := c.data[key]

	return val, ok
}

func (c *Cache[K]) Clear() {
	// block all read/write access
	c.mu.Lock()
	defer c.mu.Unlock()

	// reset map
	c.data = map[K]CacheEntry{}
}

func (c *Cache[K]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

func (c *Cache[K]) Capacity() int {
	return c.capacity
}
