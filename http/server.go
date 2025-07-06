package httpd

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mrshabel/shache/cache"
)

type Server struct {
	router     *gin.Engine
	listenAddr string
	cache      *cache.DistributedCache
	logger     *log.Logger
}

// api errors
var (
	ErrNotFound    = errors.New("entry not found")
	ErrInvalidData = errors.New("data validation error")
	ErrTTLDuration = errors.New("invalid ttl duration. standard format is: 15s, 10m")
)

func NewServer[T any](addr string, cache *cache.DistributedCache) *Server {
	return &Server{
		listenAddr: addr,
		cache:      cache,
		router:     gin.Default(),
		logger:     log.New(os.Stderr, "[http handler]", log.LstdFlags),
	}
}

// route handlers here

func pingHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

// EntryGetHandler retrieves a given entry from the distributed cache
func (s *Server) EntryGetHandler(c *gin.Context) {
	// retrieve entry from cache
	key := c.Query("key")
	if key == "" {
		c.JSON(422, gin.H{
			"message": ErrInvalidData.Error(),
		})
		return
	}

	entry, err := s.cache.Get(key)
	if err != nil {
		if errors.Is(err, cache.ErrKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to retrieve entry for given key",
		})
		return
	}

	// return data
	c.JSON(http.StatusOK, gin.H{
		"message": "Entry successfully retrieved",
		"data":    entry,
	})
}

type CacheEntryRequest struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value" binding:"required"`
	// ttl as a duration representation. eg: 15s, 10m, 1hr
	TTL string `json:"ttl"`
}

// EntrySetHandler adds a new entry to the distributed cache
func (s *Server) EntrySetHandler(c *gin.Context) {
	var entry CacheEntryRequest
	// bind request data into entry request struct
	if err := c.ShouldBind(&entry); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}
	// parse duration
	ttl, err := time.ParseDuration(entry.TTL)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": ErrTTLDuration.Error()})
		return
	}

	data := cache.CacheEntry{Value: entry.Value, TTL: ttl}
	// forward request to leader if current node is a follower
	if !s.cache.IsLeader() {
		s.forwardToLeader(c)
		return
	}

	// set cache entry
	if err := s.cache.Put(entry.Key, data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to add new cache entry"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Entry created/updated successfully"})
}

// EntryDeleteHandler removes an entry the distributed cache
func (s *Server) EntryDeleteHandler(c *gin.Context) {
	key := c.Query("key")
	if key == "" {
		c.JSON(422, gin.H{
			"message": ErrInvalidData.Error(),
		})
		return
	}

	// forward request to leader if current node is a follower
	if !s.cache.IsLeader() {
		s.forwardToLeader(c)
		return
	}

	if err := s.cache.Delete(key); err != nil {
		if errors.Is(err, cache.ErrKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to remove entry for the given key",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Entry deleted successfully"})
}

// forwardToLeader proxies the current request to the leader node
func (s *Server) forwardToLeader(c *gin.Context) {
	// get leader details
	leaderAddr := s.cache.GetLeaderAddr()
	if leaderAddr == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "Leader not available"})
		return
	}
	target, err := url.Parse(fmt.Sprintf("http://%s", leaderAddr))
	if err != nil {
		s.logger.Printf("invalid leader address %s\n", leaderAddr)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Leader address is invalid"})
		return
	}

	// forward request
	s.logger.Printf("proxying request received on follower at %s to leader node at %s\n", c.Request.URL, target.String())

	// TODO: fix transport connection broken error when proxying request to leader
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(c.Writer, c.Request)
}

func (s *Server) Start() error {
	// register handlers
	s.router.GET("/ping", pingHandler)
	s.router.GET("/cache", s.EntryGetHandler)
	s.router.POST("/cache", s.EntrySetHandler)

	// cluster handlers
	s.router.POST("/join", s.ClusterJoinHandler)
	s.router.GET("/status", s.ClusterStatusHandler)

	log.Printf("server started on %s\n", s.listenAddr)
	return s.router.Run(s.listenAddr)
}

// Shutdown closes the distributed cache server and the http server
func (s *Server) Shutdown() error {
	if err := s.cache.Leave(); err != nil {
		return err
	}

	return s.cache.Close()
}
