package cache_test

import (
	"container/heap"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mrshabel/shache/cache"
	"github.com/mrshabel/shache/utils"
)

// TestTTLHeap tests the isolated heap
func TestTTLHeap(t *testing.T) {
	ttlHeap := &cache.TTLHeap{}
	now := time.Now()

	// add entries with different expirations
	entries := []*cache.TTLEntry{
		{Key: "key1", ExpiresAt: now.Add(300 * time.Millisecond)},
		{Key: "key2", ExpiresAt: now.Add(100 * time.Millisecond)},
		{Key: "key3", ExpiresAt: now.Add(200 * time.Millisecond)},
	}
	for _, entry := range entries {
		heap.Push(ttlHeap, entry)
	}

	// verify heap pop order
	first := heap.Pop(ttlHeap).(*cache.TTLEntry)
	second := heap.Pop(ttlHeap).(*cache.TTLEntry)
	third := heap.Pop(ttlHeap).(*cache.TTLEntry)

	utils.AssertEqual(t, "key2", first.Key)
	utils.AssertEqual(t, "key3", second.Key)
	utils.AssertEqual(t, "key1", third.Key)
}

// TestTTL runs an end-to-end test on the ttl handler and its components
func TestTTL(t *testing.T) {
	// setup cluster
	cluster, cleanUps := setupCluster(t)
	// run cleanup functions
	for _, fn := range cleanUps {
		defer fn()
	}

	// test table
	tests := map[string]func(t *testing.T, cluster []*cache.DistributedCache){
		"handles ttl expiration correctly": testTTLHandler,
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt(t, cluster)
		})
	}
}

func testTTLHandler(t *testing.T, cluster []*cache.DistributedCache) {
	commitTimeout := 500 * time.Millisecond

	record := cache.KVPair{Key: "first", Value: cache.CacheEntry{Value: "second record", TTL: 3 * time.Second}}
	leader := cluster[0]
	err := leader.Put(record.Key, record.Value)
	utils.AssertNoError(t, err)

	time.Sleep(commitTimeout)

	// verify key exists
	got, err := cluster[1].Get(record.Key)
	utils.AssertNoError(t, err)
	// match values
	utils.AssertDeepEqual(t, got.Value, record.Value.Value)

	// wait for key to expire
	time.Sleep(3 * time.Second)

	// verify that key is removed
	got, err = leader.Get(record.Key)
	utils.AssertEqual(t, err, cache.ErrKeyNotFound)
	utils.AssertEqual(t, got, nil)

}

// setup cluster is a helper method for setting up the distributed cluster for the test suites. The distributed cache and a list of cleanup functions are returned
func setupCluster(t *testing.T) ([]*cache.DistributedCache, []func() error) {
	t.Helper()

	var cacheCluster []*cache.DistributedCache
	var cleanupFns []func() error

	nodeCount := 3
	commitTimeout := 500 * time.Millisecond
	initTimeout := 3 * time.Second
	cacheInvalidationInterval := 3 * time.Second

	for i := range nodeCount {
		// setup configs
		cfg := cache.DistributedCacheConfig{}
		dataDir, err := os.MkdirTemp("", "distributed_cache_test")
		utils.AssertNoError(t, err)
		// add cleanup function
		cleanupFns = append(cleanupFns, func() error {
			return os.RemoveAll(dataDir)
		})

		// ports: 8100, 8101, 8102
		cfg.BindAddr = fmt.Sprintf("127.0.0.1:810%v", i)
		cfg.NodeID = fmt.Sprintf("node%v", i)
		cfg.DataDir = dataDir
		cfg.InvalidationInterval = cacheInvalidationInterval

		// bootstrap cluster for first node
		isLeader := i == 0
		if isLeader {
			cfg.Bootstrap = true
		}

		// setup server
		server, err := cache.NewDistributedCache(cfg)
		utils.AssertNoError(t, err)

		// join cluster through leader immediately as follower else wait for leader initialization to be completed
		if isLeader {
			time.Sleep(initTimeout)
		} else {
			err = cacheCluster[0].Join(server.NodeID, server.BindAddr)
			utils.AssertNoError(t, err)
			// wait for membership to be gossiped
			time.Sleep(commitTimeout)
		}

		cacheCluster = append(cacheCluster, server)

		// cleanup
		cleanupFns = append(cleanupFns, func() error {
			return server.Close()
		})

	}
	return cacheCluster, cleanupFns
}
