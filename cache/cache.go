package cache

import (
	"log"
	"sync"
	"time"
)

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
	// TODO: use time buckets to track keys that need to be expired in a particular duration
	if value.TTL > 0 {
		go func(key K) {
			time.Sleep(value.TTL)
			c.mu.Lock()
			if _, ok := c.data[key]; !ok {
				log.Printf("key to be evicted (%v) not found", key)
				return
			}

			delete(c.data, key)
			log.Printf("%v evicted...", key)
			c.mu.Unlock()
		}(key)
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

// Delete a given key from the cache. Returns False if not found
func (c *Cache[K]) Delete(key K) bool {
	// retrieve value with a shared-lock
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.data[key]; !ok {
		return false
	}
	delete(c.data, key)
	return true
}

// DeleteBulk removes the list of given keys from the cache. A list of keys not present in the cache is returned
func (c *Cache[K]) DeleteBulk(keys []K) []K {
	// retrieve value with a shared-lock
	c.mu.Lock()
	defer c.mu.Unlock()

	var voidKeys []K

	for _, key := range keys {
		// add the non-existent key
		if _, ok := c.data[key]; !ok {
			voidKeys = append(voidKeys, key)
		}
		delete(c.data, key)
	}
	return voidKeys
}

func (c *Cache[K]) Copy() map[K]CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// deep copy
	copiedData := make(map[K]CacheEntry, len(c.data))
	for key, val := range c.data {
		copiedData[key] = val
	}

	return copiedData
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
