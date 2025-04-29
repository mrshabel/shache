# Shache

Shache is a lightweight, thread-safe caching service built in Go. It provides a simple API for storing, retrieving, and managing cached data with support for TTL (Time-To-Live) expiration. Shache is designed to be fast, scalable, and easy to integrate into your applications with support for different eviction policies such as LRU and LFU. The LFU policy uses the algorithm detailed in this [paper](http://dhruvbird.com/lfu.pdf)

## Features

-   **Thread-Safe**
-   **TTL Support**
-   **RESTful API**: Exposes a simple HTTP API for interacting with the cache.

## Installation

1. Clone the repository:

    ```bash
    git clone https://github.com/mrshabel/shache.git
    cd shache
    ```

2. Build the project:

    ```bash
    make build
    ```

3. Run the server:

    ```bash
    # production
    ./bin/shache

    # development
    make run
    ```

The server will start on localhost:8000 by default.

## TODO
