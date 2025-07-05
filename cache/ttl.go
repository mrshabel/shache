// this package contains code that sets up the ttl mechanism for the cache by recording entries on a min-heap
package cache

import (
	"container/heap"
	"sync"
	"time"
)

// TTLHandler handles expiration across the distributed cache
type TTLHandler struct {
	cache *DistributedCache
	// heap containing the expirations
	heap *TTLHeap
	// mutex for the handler
	mu sync.RWMutex
	// interval for performing cache invalidation
	interval time.Duration
	ticker   *time.Ticker
	// channel to receive stop notifications
	stopCh chan struct{}
}

// TTLEntry holds information about a given key and its expiry in the cache
type TTLEntry struct {
	Key string `json:"key"`
	// the priority of item in the priority queue
	ExpiresAt time.Time `json:"expiresAt"`
	// index of item in heap
	Index int `json:"index"`
}

// TTLHeap is a min-heap of ttl entries ranked on the expiry time
type TTLHeap []*TTLEntry

// interface methods

func (h TTLHeap) Len() int { return len(h) }

func (h TTLHeap) Less(i, j int) bool {
	// get the lowest time
	return h[i].ExpiresAt.Before(h[j].ExpiresAt)
}

func (h TTLHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].Index = i
	h[j].Index = j
}

func (h *TTLHeap) Push(x any) {
	// append item to heap through its end
	n := len(*h)
	entry := x.(*TTLEntry)
	entry.Index = n
	*h = append(*h, entry)
}

func (h *TTLHeap) Pop() any {
	old := *h
	n := len(old)
	entry := old[n-1]
	// cleanup entry
	old[n-1] = nil
	entry.Index = -1
	*h = old[:n-1]
	return entry
}

// Update modifies the expiry of an entry in the heap
func (h *TTLHeap) Update(entry *TTLEntry, expiresAt time.Time) {
	entry.ExpiresAt = expiresAt
	heap.Fix(h, entry.Index)
}

// NewTTLHandler instantiates a new ttl handler for the distributed cache
func NewTTLHandler(cache *DistributedCache, interval time.Duration) *TTLHandler {
	return &TTLHandler{
		cache:    cache,
		heap:     &TTLHeap{},
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the ttl background process
func (t *TTLHandler) Start() {
	// start ticker to invalidate stale entries
	t.ticker = time.NewTicker(t.interval)
	go func() {
		for {
			select {
			case <-t.ticker.C:
				t.invalidateCache()
			case <-t.stopCh:
				return
			}
		}
	}()
}

// scheduleExpiration adds a key to the ttl handler to begin the expiration process. It should be called when a key is added to the cache
func (t *TTLHandler) ScheduleExpiration(key string, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	// push entry into heap
	entry := &TTLEntry{Key: key, ExpiresAt: time.Now().Add(ttl)}
	heap.Push(t.heap, entry)
}

// invalidateCache removes all stale cache entries
func (t *TTLHandler) invalidateCache() {
	// get all expired keys from the heap
	now := time.Now()
	expiredKeys := make([]string, 0)

	t.mu.Lock()

	for t.heap.Len() > 0 {
		// stop if the closest element to be expired has not expired
		entry := (*t.heap)[0]
		if entry.ExpiresAt.After(now) {
			break
		}

		// remove entry from heap
		expiredKeys = append(expiredKeys, entry.Key)
		heap.Pop(t.heap)
	}
	// release lock
	t.mu.Unlock()

	// send keys to the cluster to expire
	t.cache.DeleteBulk(expiredKeys)

}

// Stop stops the ttl handler and cleanup resources
func (t *TTLHandler) Stop() {
	if t.ticker != nil {
		t.ticker.Stop()
	}
	close(t.stopCh)
}
