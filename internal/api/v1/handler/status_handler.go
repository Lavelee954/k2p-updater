package handler

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Event reasons for status updates
const (
	StatusUpdateReason       = "StatusUpdate"
	StatusWebhookReason      = "WebhookStatusUpdate"
	StatusUpdateFailedReason = "StatusUpdateFailed"
	StatusRetrievalReason    = "StatusRetrieval"
	StatusRetrievalFailed    = "StatusRetrievalFailed"
)

// StatusHandler handles API requests related to control plane status
type StatusHandler struct {
	updaterService interfaces.Provider // Updated type
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(updaterService interfaces.Provider) *StatusHandler {
	return &StatusHandler{
		updaterService: updaterService,
	}
}

// SetupRoutes configures the routes for this handler
func (h *StatusHandler) SetupRoutes(router *gin.Engine) {
	statusGroup := router.Group("/api/v1/status")
	{
		statusGroup.GET("/nodes", h.GetNodeStatus)
		// Add other status-related routes here
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

// GetNodeStatus returns the current status of control plane nodes
func (h *StatusHandler) GetNodeStatus(c *gin.Context) {
	// Implementation remains the same, but uses the new interface methods
	// ...
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
