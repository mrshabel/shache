# Shache

Shache is a lightweight distributed caching service leveraging RAFT for distributed consensus. It supports ttl-based expiration of keys across the cluster of nodes. Data is stored in memory with additional durability provided via RAFT snapshots.

Since RAFT requires an odd number of nodes for a quorum to be reached, it is advisable to spin up an odd number of nodes (3, 5, 7) for optimal performance. Thus, the number of nodes needed to reach consensus are `N/2 + 1` where N is the total number of nodes in the cluster.

## Prerequisites

-   Go 1.23 or higher
-   Make (for running tests)

## Installation

```bash
    git clone https://github.com/mrshabel/shache
    cd shache
```

## Setup

Access the setup instruction with:

```bash
    ./bin/shache -help
```

### Single Node

```bash
    # start production server on port 8000
    ./bin/shache -node-id=node1 -http-addr=127.0.0.1:8000 -raft-addr=127.0.0.1:8100 -bootstrap=true
```

### Cluster

```bash
# start leader (node 1)
./bin/shache -node-id=node1 -http-addr=127.0.0.1:8000 -raft-addr=127.0.0.1:8100 -data-dir=./data -bootstrap=true

# start and join follower to leader (node-2)
./bin/shache -node-id=node2 -http-addr=127.0.0.1:8001 -raft-addr=127.0.0.1:8101 -data-dir=./data-1 -join=127.0.0.1:8000

# start and join follower to leader (node-3)
./bin/shache -node-id=node3 -http-addr=127.0.0.1:8002 -raft-addr=127.0.0.1:8102 -data-dir=./data-2 -join=127.0.0.1:8000

```

This sets up a 3-node cluster where the leader bootstraps the cluster.

## Usage

1. Interact with the cache via HTTP:

-   Write an entry to shache via the leader node. The duration is represented with units `s`, `m`, `hr` for seconds, minutes, and hours respectively

```bash
curl -X POST localhost:8000/cache -H "Content-Type: application/json" -d '{"key":"name", "value":"shabel", "ttl":"5m"}'
```

-   Retrieve an entry from shache via any node in the cluster

```bash
curl localhost:8000/cache?key=name
```

1. Interact with cluster via HTTP:

-   View cluster status from any node. Information about the leader and all nodes are returned and may be used to explicitly join the cluster

```bash
curl localhost:8000/status
```

## Running Tests

```bash
make test
```

This runs unit and integration tests across all components in the shache
