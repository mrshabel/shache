package cache

import "time"

// K is any type which supports compare operations
// type K comparable

type Cacher[T comparable] interface {
	// Put adds the value to the cache, and returns a boolean to indicate whether the value
	// already existed or not. Put refreshes the underlying tracking of the cache policy
	Put(key T, value CacheEntry) bool

	// Get returns the value associated with a given key and a boolean to indicate whether
	// the value was found or not
	Get(key T) (CacheEntry, bool)

	// Clear resets the underlying cache to its original state
	Clear()

	// Size returns the current size of the cache. It gives details on the number of elements currently
	// present
	Size() int

	// Capacity returns the maximum allowed elements for the underlying cache
	Capacity() int
}

// CacheEntry is a representation of a single cache entry value. It contains the TTL and the value
// associated with a given key
type CacheEntry struct {
	Value       any           `json:"value"`
	TTL         time.Duration `json:"ttl"`
	AccessCount int           `json:"accessCount"`
	CreatedAt   time.Time     `json:"createdAt"`
	ExpiresAt   time.Time     `json:"updatedAt"`
}

// IsExpired checks if the entry has expired
func (c *CacheEntry) IsExpired() bool {
	if c.TTL <= 0 {
		return false
	}
	return c.ExpiresAt.Before(time.Now())
}

// Visited update the visited statistics of the entry
func (c *CacheEntry) Visited() {
	c.AccessCount++
}
