package httpd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type JoinRequest struct {
	NodeID string `json:"nodeId" binding:"required"`
	Addr   string `json:"address" binding:"required"`
}

// ClusterJoin adds a new node to the cluster
func (s *Server) ClusterJoinHandler(c *gin.Context) {
	// forward request to leader
	if !s.cache.IsLeader() {
		s.forwardToLeader(c)
		return
	}

	// send join event to the cluster
	var req JoinRequest
	// bind request data into entry request struct
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": err.Error()})
		return
	}

	if err := s.cache.Join(req.NodeID, req.Addr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Node joined cluster successfully"})
}

// ClusterStatus returns the status of nodes in the cluster
func (s *Server) ClusterStatusHandler(c *gin.Context) {
	status, err := s.cache.Status()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Cluster status retrieve successfully", "data": status})
}

// joinCluster is a helper function that will be called on app initialization to join the current node to an existing cluster through its leader address
func JoinCluster(joinAddr, raftAddr, nodeID string) error {
	req := JoinRequest{NodeID: nodeID, Addr: raftAddr}

	b, err := json.Marshal(&req)
	if err != nil {
		return err
	}
	resp, err := http.Post(fmt.Sprintf("http://%s/join", joinAddr), "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
