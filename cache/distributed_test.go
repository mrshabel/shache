package cache_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mrshabel/shache/cache"
	"github.com/mrshabel/shache/utils"
)

// TestDistributedCache runs an end-to-end test of all components in the distributed cache
func TestDistributedCache(t *testing.T) {
	var cacheCluster []*cache.DistributedCache
	nodeCount := 3
	commitTimeout := 500 * time.Millisecond
	initTimeout := 3 * time.Second

	for i := range nodeCount {
		// setup configs
		cfg := cache.DistributedCacheConfig{}
		dataDir, err := os.MkdirTemp("", "distributed_cache_test")
		utils.AssertNoError(t, err)
		defer func(dataDir string) {
			_ = os.RemoveAll(dataDir)
		}(dataDir)

		// ports: 8100, 8101, 8102
		cfg.BindAddr = fmt.Sprintf("127.0.0.1:810%v", i)
		cfg.NodeID = fmt.Sprintf("node%v", i)
		cfg.DataDir = dataDir

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
		defer func() {
			_ = server.Close()
		}()
	}

	// add new record
	records := []*cache.KVPair{
		{Key: "first", Value: cache.CacheEntry{Value: "second record", TTL: 0 * time.Minute}},
		{Key: "second", Value: cache.CacheEntry{Value: "second record", TTL: 0 * time.Minute}},
		{Key: "third", Value: cache.CacheEntry{Value: "third record", TTL: 0 * time.Minute}},
	}

	// add records to the leader, wait for the records to be committed then read from all nodes
	for _, record := range records {
		leader := cacheCluster[0]
		err := leader.Put(record.Key, record.Value)
		utils.AssertNoError(t, err)

		time.Sleep(commitTimeout)

		for _, node := range cacheCluster {
			got, err := node.Get(record.Key)
			utils.AssertNoError(t, err)

			// match values
			utils.AssertDeepEqual(t, got.Value, record.Value.Value)
		}
	}

	// start a new server, wait for leader to replicate logs then read log
	cfg := cache.DistributedCacheConfig{}
	dataDir, err := os.MkdirTemp("", "distributed_cache_test")
	utils.AssertNoError(t, err)
	defer func(dataDir string) {
		_ = os.RemoveAll(dataDir)
	}(dataDir)

	// ports: 8100, 8101, 8102
	cfg.BindAddr = fmt.Sprintf("127.0.0.1:810%v", len(cacheCluster)+1)
	cfg.NodeID = fmt.Sprintf("node%v", len(cacheCluster)+1)
	cfg.DataDir = dataDir

	// setup server
	server, err := cache.NewDistributedCache(cfg)
	utils.AssertNoError(t, err)
	defer server.Close()

	err = cacheCluster[0].Join(server.NodeID, server.BindAddr)
	utils.AssertNoError(t, err)
	// wait for membership to be gossiped
	time.Sleep(commitTimeout)

	// read logs
	for _, record := range records {
		got, err := server.Get(record.Key)
		utils.AssertNoError(t, err)

		// match values
		utils.AssertDeepEqual(t, got.Value, record.Value.Value)
	}
}
