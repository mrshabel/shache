package main

import (
	"fmt"
	"log"

	"github.com/mrshabel/shache/cache"
	"github.com/mrshabel/shache/internal/server"
)

func main() {
	// cache with stringified keys
	cache := cache.NewCache[string](100)
	server := server.NewServer[string]("127.0.0.1:8000", cache)

	fmt.Println("####################################")
	fmt.Println("#              SHACHE              #")
	fmt.Println("####################################")
	fmt.Println()
	log.Fatal(server.Start())
}
