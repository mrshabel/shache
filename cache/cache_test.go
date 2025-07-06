package cache

import (
	"testing"
	"time"

	"github.com/mrshabel/shache/utils"
)

// test cache flow end-to-end
func TestCache(t *testing.T) {
	sampleEntry := CacheEntry{Value: "Shabel", TTL: 500 * time.Millisecond}

	// cache contains string key-pair
	cache := NewCache[string](10)

	// set an arbitrary key
	exists := cache.Put("name", sampleEntry)
	utils.AssertFalse(t, exists)

	// retrieve value
	val, exists := cache.Get("name")
	t.Log(val)
	utils.AssertTrue(t, exists)

	// replace value. should return True
	exists = cache.Put("name", sampleEntry)
	utils.AssertTrue(t, exists)

	// get non-existent key
	_, exists = cache.Get("age")
	utils.AssertFalse(t, exists)

	// sleep until ttl entry is evicted
	time.Sleep(2 * sampleEntry.TTL)
	val, exists = cache.Get("name")
	utils.AssertEqual(t, val, CacheEntry{})
	utils.AssertFalse(t, exists)

	// clear cache
	cache.Clear()
	_, exists = cache.Get("name")
	utils.AssertFalse(t, exists)
}
