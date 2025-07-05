// this file implements a distributed cache that uses the raft consensus algorithm
package cache

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
)

const (
	RaftTimeout                 = 10 * time.Second
	RaftTransportPoolSize       = 5
	RaftMaxSnapshotRetained     = 2
	cacheCapacity               = 1000
	DefaultInvalidationInterval = 3 * time.Minute
)

// request types for the distributed cache
type RequestType uint8

const (
	PutRequestType RequestType = iota
	DeleteRequestType
	DeleteBulkRequestType
)

var (
	ErrNodeNotLeader = fmt.Errorf("node is not the leader")
	ErrKeyNotFound   = fmt.Errorf("key not found in cache")
)

// Node represents a node in the cluster
type Node struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

// NodeStatus represents the status of the nodes in the cluster
type NodeStatus struct {
	Me        Node   `json:"me"`
	Leader    Node   `json:"leader"`
	Followers []Node `json:"followers"`
}

// KVPair represents a cache key-value pair
type KVPair struct {
	Key   string     `json:"key"`
	Value CacheEntry `json:"value,omitempty"`
}

// raft finite state machine for handling business logic
type fsm struct {
	cache *Cache[string]
}

// enforce raft fsm interface compliance
var _ raft.FSM = (*fsm)(nil)

type DistributedCache struct {
	// directory to store raft data
	DataDir string
	// ID of the current node
	NodeID string
	// TCP listener address for the node
	BindAddr string
	// Bootstrap indicates whether the current node should bootstrap the cluster or not
	Bootstrap bool

	// internal cache
	cache *Cache[string]

	// background ttl handler
	ttlHandler *TTLHandler

	// consensus mechanism
	raft *raft.Raft
	// persist store for keeping snapshots
	raftStore *raftboltdb.BoltStore
	logger    *log.Logger
}

// DistributedCacheConfig is the configuration for the distributed cache
type DistributedCacheConfig struct {
	// directory to store raft data
	DataDir string
	// ID of the current node
	NodeID string
	// TCP listener address for the node
	BindAddr string
	// Bootstrap indicates whether the current node should start the cluster or not. Should be used for first node in the cluster only
	Bootstrap bool
}

func (cfg *DistributedCacheConfig) validate() error {
	if cfg.DataDir == "" {
		return fmt.Errorf("DataDir is required")
	}
	if cfg.NodeID == "" {
		return fmt.Errorf("NodeID is required")
	}
	if cfg.BindAddr == "" {
		return fmt.Errorf("BindAddr is required")
	}
	return nil
}

// NewDistributedCache returns a new distributed cache
func NewDistributedCache(cfg DistributedCacheConfig) (*DistributedCache, error) {
	// setup and validate configs
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	c := &DistributedCache{
		NodeID:    cfg.NodeID,
		BindAddr:  cfg.BindAddr,
		DataDir:   cfg.DataDir,
		Bootstrap: cfg.Bootstrap,
		logger:    log.New(os.Stderr, "[shache] ", log.LstdFlags),
	}
	// setup cache and raft
	c.cache = NewCache[string](cacheCapacity)

	if err := c.setupRaft(); err != nil {
		return nil, err
	}

	// setup ttl handler
	c.ttlHandler = NewTTLHandler(c, DefaultInvalidationInterval)
	return c, nil
}

// setupRaft sets up raft on the current node. If no existing peers exists, the node becomes the leader of the cluster
func (c *DistributedCache) setupRaft() error {
	// setup the finite state machine
	fsm := &fsm{cache: c.cache}

	// setup raft configuration
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(c.NodeID)

	// setup raft listener and transport to communicate with peer nodes
	addr, err := net.ResolveTCPAddr("tcp", c.BindAddr)
	if err != nil {
		return err
	}
	transport, err := raft.NewTCPTransport(c.BindAddr, addr, RaftTransportPoolSize, RaftTimeout, os.Stderr)
	if err != nil {
		return err
	}

	// setup snapshot store to allow raft truncate logs
	snapshots, err := raft.NewFileSnapshotStore(c.DataDir, RaftMaxSnapshotRetained, os.Stderr)
	if err != nil {
		return fmt.Errorf("file snapshot store: %w", err)
	}

	// setup log store and stable store (where raft stores cluster metadata)
	c.raftStore, err = raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(c.DataDir, "raft.db"),
	})
	if err != nil {
		return fmt.Errorf("raft store: %w", err)
	}

	// instantiate raft
	c.raft, err = raft.NewRaft(config, fsm, c.raftStore, c.raftStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("new raft: %w", err)
	}

	// check if server has any existing state
	hasState, err := raft.HasExistingState(c.raftStore, c.raftStore, snapshots)
	if err != nil {
		return err
	}

	// if bootstrap is specified and not existing state existing, the current node is setup as a leader of the raft cluster
	if c.Bootstrap && !hasState {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{ID: config.LocalID, Address: transport.LocalAddr()},
			},
		}
		err = c.raft.BootstrapCluster(configuration).Error()
	}
	return err
}

// Get returns the entry associated with a given key on the current node. [ErrKeyNotFound] is returned if key is not present in cache
func (c *DistributedCache) Get(key string) (*CacheEntry, error) {
	entry, exists := c.cache.Get(key)
	if !exists {
		return nil, ErrKeyNotFound
	}

	// passively remove key if it has expired
	if entry.IsExpired() {
		if err := c.Delete(key); err != nil {
			c.logger.Printf("failed to delete key=(%s) passively\n: %v", key, err)
		}
		return nil, ErrKeyNotFound
	}
	return &entry, nil
}

// Put sets the value for a given key through the leader node. It cache entry is sent to the raft cluster for replication.
func (c *DistributedCache) Put(key string, val CacheEntry) error {
	if c.raft.State() != raft.Leader {
		return ErrNodeNotLeader
	}
	var buf bytes.Buffer
	// write a single byte indicating the request type, followed by the data to be added
	if _, err := buf.Write([]byte{byte(PutRequestType)}); err != nil {
		return err
	}

	// marshal record and write to byte buffer
	b, err := json.Marshal(&KVPair{Key: key, Value: val})
	if err != nil {
		return err
	}
	if _, err := buf.Write(b); err != nil {
		return err
	}

	// finally write to raft log
	future := c.raft.Apply(buf.Bytes(), RaftTimeout)
	if err := future.Error(); err != nil {
		return err
	}

	// schedule expiration if ttl if present
	if val.TTL > 0 {
		c.ttlHandler.ScheduleExpiration(key, val.TTL)
	}
	return nil
}

// Delete removes the entry for a given key through the leader node
func (c *DistributedCache) Delete(key string) error {
	if c.raft.State() != raft.Leader {
		return ErrNodeNotLeader
	}
	var buf bytes.Buffer
	// write a single byte indicating the request type, followed by the data to be added
	if _, err := buf.Write([]byte{byte(DeleteRequestType)}); err != nil {
		return err
	}

	// marshal record and write to byte buffer
	b, err := json.Marshal(&KVPair{Key: key})
	if err != nil {
		return err
	}
	if _, err := buf.Write(b); err != nil {
		return err
	}

	// finally write to raft log
	future := c.raft.Apply(buf.Bytes(), RaftTimeout)
	return future.Error()
}

// DeleteBulk removes the entries for the given keys through the leader node
func (c *DistributedCache) DeleteBulk(keys []string) error {
	if c.raft.State() != raft.Leader {
		return ErrNodeNotLeader
	}
	var buf bytes.Buffer
	// write a single byte indicating the request type, followed by the data to be added
	if _, err := buf.Write([]byte{byte(DeleteRequestType)}); err != nil {
		return err
	}

	var records []*KVPair
	for _, key := range keys {
		records = append(records, &KVPair{Key: key})
	}
	// marshal records and write to byte buffer
	b, err := json.Marshal(records)
	if err != nil {
		return err
	}
	if _, err := buf.Write(b); err != nil {
		return err
	}

	// finally write to raft log
	future := c.raft.Apply(buf.Bytes(), RaftTimeout)
	return future.Error()
}

// Join adds a node to the raft cluster with its node ID and bind address. This should be called only on the leader node
func (c *DistributedCache) Join(nodeID, addr string) error {
	c.logger.Printf("received join request for remote node %s at %s\n", c.NodeID, c.BindAddr)

	// get the latest raft cluster configuration
	configFuture := c.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		c.logger.Printf("failed to get raft configuration: %v\n", err)
		return err
	}

	for _, srv := range configFuture.Configuration().Servers {
		// if server id or address is found, the node will be removed. if both id and address are present, however, skip initialization
		if srv.Address == raft.ServerAddress(addr) && srv.ID == raft.ServerID(nodeID) {
			c.logger.Printf("node %s at %s already a member of the cluster, ignoring join request", nodeID, addr)
			return nil
		}

		// remove existing node
		if srv.Address == raft.ServerAddress(addr) || srv.ID == raft.ServerID(nodeID) {
			future := c.raft.RemoveServer(srv.ID, 0, 0)
			if err := future.Error(); err != nil {
				return fmt.Errorf("error removing existing node %s at %s: %w", nodeID, addr, err)
			}
		}
	}

	// add node as voter
	future := c.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, 0)
	if future.Error() != nil {
		return future.Error()
	}

	c.logger.Printf("node %s at %s joined cluster successfully", nodeID, addr)
	return nil
}

// Status returns information about the distributed cache through the current node
func (c *DistributedCache) Status() (*NodeStatus, error) {
	status := &NodeStatus{}
	// get leader information
	leaderAddr, leaderID := c.raft.LeaderWithID()
	status.Leader = Node{ID: string(leaderID), Address: string(leaderAddr)}

	// current node
	status.Me.ID = string(c.NodeID)
	status.Me.Address = string(c.BindAddr)

	// get followers
	configFuture := c.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		c.logger.Printf("failed to get raft configuration: %v\n", err)
		return nil, err
	}
	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID != leaderID {
			status.Followers = append(status.Followers, Node{ID: string(srv.ID), Address: string(srv.Address)})
		}

		if srv.ID == raft.ServerID(c.BindAddr) {
			status.Me.ID = string(srv.ID)
			status.Me.Address = string(srv.Address)
		}
	}
	return status, nil
}

// Close shuts down the raft cluster
func (c *DistributedCache) Close() error {
	future := c.raft.Shutdown()
	return future.Error()
}

// raft finite state machine methods

// Apply will be called by the raft fsm when it receives a log entry
func (f *fsm) Apply(l *raft.Log) interface{} {
	// read first byte for request type and process remaining bytes in individual handlers
	reqType := RequestType(l.Data[0])

	// delete bulk request type takes in an array of keys so it is processed differently
	if reqType == DeleteBulkRequestType {
		var data []KVPair
		// unmarshal data
		if err := json.Unmarshal(l.Data[1:], &data); err != nil {
			panic(fmt.Sprintf("failed to unmarshal array kv-pair: %s", err))
		}
		return f.applyDeleteBulk(data)
	} else {
		var data KVPair
		// unmarshal data
		if err := json.Unmarshal(l.Data[1:], &data); err != nil {
			panic(fmt.Sprintf("failed to unmarshal kv-pair: %s", err))
		}

		switch reqType {
		case PutRequestType:
			return f.applyPut(data)
		case DeleteRequestType:
			return f.applyDelete(data)
		default:
			// panic for unknown commands
			panic(fmt.Sprintf("unknown request type: %v", reqType))
		}
	}

}

// applyPut is called by the fsm to execute the the Put operation on the node
func (f *fsm) applyPut(data KVPair) interface{} {
	f.cache.Put(data.Key, data.Value)
	return nil
}

// applyDelete is called by the fsm to execute the the Delete operation on the node
func (f *fsm) applyDelete(data KVPair) interface{} {
	if exists := f.cache.Delete(data.Key); !exists {
		return ErrKeyNotFound
	}
	return nil
}

// applyDeleteBulk is called by the fsm to execute the the DeleteBulk operation on the node. The void keys are returned if any
func (f *fsm) applyDeleteBulk(data []KVPair) interface{} {
	keys := make([]string, 0, len(data))
	for _, d := range data {
		keys = append(keys, d.Key)
	}
	return f.cache.DeleteBulk(keys)
}

// Snapshot returns a point in time snapshot of the cache
func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	// clone the cache
	data := f.cache.Copy()
	return &fsmSnapshot{data: data}, nil
}

// Restore stores the cache kv-store to a previous state
func (f *fsm) Restore(rc io.ReadCloser) error {
	// start from clean state
	data := make(map[string]CacheEntry)

	// load json data into clean map
	if err := json.NewDecoder(rc).Decode(&data); err != nil {
		return err
	}

	f.cache.data = data
	return nil
}

// fsm snapshotting

type fsmSnapshot struct {
	data map[string]CacheEntry
}

// enforce interface implementation
var _ raft.FSMSnapshot = (*fsmSnapshot)(nil)

// Persist writes the snapshotted data to the underlying store
func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	// persist data to sink (raft store)
	err := func() error {
		b, err := json.Marshal(f.data)
		if err != nil {
			return err
		}

		if _, err = sink.Write(b); err != nil {
			return err
		}
		return sink.Close()
	}()
	// cancel sink operation on error
	if err != nil {
		sink.Cancel()
	}

	return err
}

// Release is called when snapshotting is done
func (f *fsmSnapshot) Release() {}
