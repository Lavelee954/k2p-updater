package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"k2p-updater/internal/features/updater/domain"
)

// Event reasons for status updates
const (
	StatusUpdateReason       = "StatusUpdate"
	StatusWebhookReason      = "WebhookStatusUpdate"
	StatusUpdateFailedReason = "StatusUpdateFailed"
	StatusRetrievalReason    = "StatusRetrieval"
	StatusRetrievalFailed    = "StatusRetrievalFailed"
)

// StatusHandler handles HTTP requests related to node status
type StatusHandler struct {
	updaterService domain.Provider
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(updaterService domain.Provider) *StatusHandler {
	if updaterService == nil {
		log.Fatal("updater service cannot be nil")
	}

	return &StatusHandler{
		updaterService: updaterService,
	}
}

// SetupRoutes registers handler routes to the router
func (h *StatusHandler) SetupRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		// Node status endpoints
		api.GET("/nodes/:nodeName/status", h.getNodeStatus)
		api.GET("/nodes/:nodeName/cpu", h.getNodeCPU)
	}
}

// getNodeStatus retrieves the current status of a node
func (h *StatusHandler) getNodeStatus(c *gin.Context) {
	nodeName := c.Param("nodeName")
	if nodeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "node name is required",
		})
		return
	}

	// Create a context with timeout for the operation
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	status, err := h.updaterService.GetStateMachine().GetStatus(ctx, nodeName)
	if err != nil {
		log.Printf("Error retrieving status for node %s: %v", nodeName, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to retrieve status: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// getNodeCPU retrieves the current CPU utilization of a node
func (h *StatusHandler) getNodeCPU(c *gin.Context) {
	nodeName := c.Param("nodeName")
	if nodeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "node name is required",
		})
		return
	}

	// Create a context with timeout for the operation
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	current, windowAvg, err := h.updaterService.GetNodeCPUUtilization(ctx, nodeName)
	if err != nil {
		log.Printf("Error retrieving CPU utilization for node %s: %v", nodeName, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to retrieve CPU utilization: %v", err),
		})
		return
	}

	// Check if node is in cooldown
	inCooldown, remaining, err := h.updaterService.IsCooldownActive(ctx, nodeName)
	if err != nil {
		log.Printf("Error checking cooldown for node %s: %v", nodeName, err)
	}

	response := gin.H{
		"nodeName":            nodeName,
		"currentCPU":          current,
		"windowAverageCPU":    windowAvg,
		"inCooldown":          inCooldown,
		"remainingCooldownMs": remaining.Milliseconds(),
	}

	c.JSON(http.StatusOK, response)
}
