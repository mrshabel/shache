package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrshabel/shache/cache"
	httpd "github.com/mrshabel/shache/http"
)

type Config struct {
	Bootstrap bool
	HTTPAddr  string
	RaftAddr  string
	JoinAddr  string
	NodeID    string
	DataDir   string
}

// validate checks if all required configuration values are set
func (c *Config) validate() error {
	if c.NodeID == "" {
		return fmt.Errorf("node-id is required")
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("http-addr is required")
	}
	if c.RaftAddr == "" {
		return fmt.Errorf("raft-addr is required")
	}
	// JoinAddr is only required when not bootstrapping
	if !c.Bootstrap && c.JoinAddr == "" {
		return fmt.Errorf("join address is required when not bootstrapping")
	}
	return nil
}

func main() {
	cfg := Config{}

	flag.BoolVar(&cfg.Bootstrap, "bootstrap", false, "Bootstrap the cluster")
	flag.StringVar(&cfg.HTTPAddr, "http-addr", "", "Set the HTTP bind address")
	flag.StringVar(&cfg.RaftAddr, "raft-addr", "", "Set Raft bind address")
	flag.StringVar(&cfg.JoinAddr, "join", "", "Set join address")
	flag.StringVar(&cfg.NodeID, "node-id", "", "Node ID")
	flag.StringVar(&cfg.DataDir, "data-dir", "./data", "Data directory")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: shache [options] ")
		flag.PrintDefaults()
	}

	// validate cli configs
	flag.Parse()
	if err := cfg.validate(); err != nil {
		flag.Usage()
		log.Fatalf("invalid configuration: %v\n", err)
	}

	// setup cache
	cacheConfig := cache.DistributedCacheConfig{
		DataDir:              cfg.DataDir,
		NodeID:               cfg.NodeID,
		BindAddr:             cfg.RaftAddr,
		Bootstrap:            cfg.Bootstrap,
		InvalidationInterval: 30 * time.Second,
	}

	cache, err := cache.NewDistributedCache(cacheConfig)
	if err != nil {
		log.Fatalf("failed to create distributed cache: %v\n", err)
	}

	// cache with stringified keys
	server := httpd.NewServer[string](cfg.HTTPAddr, cache)

	// start server in background
	fmt.Println("####################################")
	fmt.Println("#              SHACHE              #")
	fmt.Println("####################################")
	fmt.Println()

	go func() {
		log.Printf("starting http server")
		if err := server.Start(); err != nil {
			log.Fatalf("http server startup failed: %v\n", err)
		}
	}()

	// join cluster if not bootstrapping
	if !cfg.Bootstrap && cfg.JoinAddr != "" {
		log.Printf("joining cluster via %s\n", cfg.JoinAddr)
		if err := httpd.JoinCluster(cfg.JoinAddr, cfg.RaftAddr, cfg.NodeID); err != nil {
			log.Fatalf("failed to join cluster: %v\n", err)
		}
	}

	log.Printf("Node %s started successfully\n", cfg.NodeID)
	log.Printf("HTTP API available at %s\n", cfg.HTTPAddr)
	log.Printf("Raft listening on %s\n", cfg.RaftAddr)

	cleanup(server)
}

func cleanup(server *httpd.Server) {
	// channels for os signal events
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	<-sigChan
	log.Println("program has been interrupted. shutting down server down")
	go func() {
		if err := server.Shutdown(); err != nil {
			log.Printf("error shutting down server: %v\n", err)
			os.Exit(1)
		}
		done <- struct{}{}
	}()

	// wait for cleanup completion or timeout
	select {
	case <-done:
		log.Println("server shutdown completed")
	case <-ctx.Done():
		log.Println("server timeout while shutting down. exiting now...")
	}
}
