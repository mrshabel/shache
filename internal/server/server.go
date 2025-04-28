package server

import (
	"errors"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mrshabel/shache/cache"
)

type Server struct {
	router     *gin.Engine
	listenAddr string
	cache      cache.Cacher[any, any]
}

// api errors
var (
	ErrNotFound    = errors.New("entry not found")
	ErrInvalidData = errors.New("data validation error")
)

type CacheEntryRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// duration in milliseconds
	TTL time.Duration `json:"ttl"`
}

func NewServer[T any](addr string, cache cache.Cacher[any, any]) *Server {
	return &Server{
		listenAddr: addr,
		cache:      cache,
	}
}

// route handlers here
func pingHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func (s *Server) getEntryHandler(c *gin.Context) {
	// retrieve entry from cache
	key := c.Query("key")
	if key == "" {
		c.JSON(422, gin.H{
			"message": ErrInvalidData.Error(),
		})
		return
	}

	entry, ok := s.cache.Get(key)
	if !ok {
		c.JSON(404, gin.H{
			"message": ErrNotFound.Error(),
		})
		return
	}

	// return data
	c.JSON(200, gin.H{
		"message": "Entry successfully retrieved",
		"data":    entry,
	})
}

func (s *Server) setEntryHandler(c *gin.Context) {
	var entry CacheEntryRequest
	// bind request data into entry request struct
	if err := c.Bind(&entry); err != nil {
		c.JSON(422, gin.H{"message": err.Error()})
		return
	}

	// set cache entry
	if ok := s.cache.Put(entry.Key, cache.CacheEntry{Value: entry.Value, TTL: entry.TTL}); ok {
		c.JSON(200, gin.H{"message": "Entry updated successfully"})
		return
	}

	c.JSON(201, gin.H{"message": "Entry created successfully"})
}

func (s *Server) Start() error {
	// register handlers
	s.router.GET("/ping", pingHandler)
	s.router.GET("/entries", s.getEntryHandler)
	s.router.POST("/entries", s.setEntryHandler)

	log.Printf("server started on %s\n", s.listenAddr)
	return s.router.Run(s.listenAddr)
}
