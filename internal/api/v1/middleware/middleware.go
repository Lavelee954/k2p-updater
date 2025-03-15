package middleware

//import (
//	"fmt"
//	"github.com/gin-gonic/gin"
//	"log"
//	"net/http"
//	"time"
//)
//
//// LoggingMiddleware adds request logging with timing information
//func LoggingMiddleware() gin.HandlerFunc {
//	return func(c *gin.Context) {
//		// Start time
//		startTime := time.Now()
//
//		// Log request
//		path := c.Request.URL.Path
//		method := c.Request.Method
//		log.Printf("Request: %s %s", method, path)
//
//		// Process request
//		c.Next()
//
//		// End time
//		endTime := time.Now()
//		latency := endTime.Sub(startTime)
//
//		// Log response with latency
//		status := c.Writer.Status()
//		log.Printf("Response: %s %s [%d] (%v)", method, path, status, latency)
//	}
//}
//
//// Event reasons for status updates
//const (
//	StatusUpdateReason       = "StatusUpdate"
//	StatusWebhookReason      = "WebhookStatusUpdate"
//	StatusUpdateFailedReason = "StatusUpdateFailed"
//	StatusRetrievalReason    = "StatusRetrieval"
//	StatusRetrievalFailed    = "StatusRetrievalFailed"
//)
//
//// StatusHandler handles HTTP requests related to node status
//type StatusHandler struct {
//	updaterService *service.Service
//}
//
//// NewStatusHandler creates a new status handler
//func NewStatusHandler(updaterService *service.Service) *StatusHandler {
//	if updaterService == nil {
//		log.Fatal("updater service cannot be nil")
//	}
//
//	return &StatusHandler{
//		updaterService: updaterService,
//	}
//}
//
//// SetupRoutes registers handler routes to the router
//func (h *StatusHandler) SetupRoutes(r *gin.Engine) {
//	api := r.Group("/api/v1")
//	{
//		// Node status endpoints
//		api.POST("/nodes/:nodeName/status", h.updateNodeStatus)
//		api.GET("/nodes/:nodeName/status", h.getNodeStatus)
//
//		// Webhook endpoint
//		api.POST("/webhook/status", h.handleStatusWebhook)
//	}
//}
//
//// updateNodeStatus updates the status of a specific node
//func (h *StatusHandler) updateNodeStatus(c *gin.Context) {
//	nodeName := c.Param("nodeName")
//	if nodeName == "" {
//		c.JSON(http.StatusBadRequest, gin.H{
//			"error": "node name is required",
//		})
//		return
//	}
//
//	var req struct {
//		Status  string `json:"status" binding:"required"`
//		Message string `json:"message"`
//	}
//
//	if err := c.ShouldBindJSON(&req); err != nil {
//		c.JSON(http.StatusBadRequest, gin.H{
//			"error": fmt.Sprintf("invalid request format: %v", err),
//		})
//		return
//	}
//
//	// Create a context with timeout for the operation
//	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
//	defer cancel()
//
//	// Create a node reference for events
//	nodeRef := &v1.ObjectReference{
//		Kind:      "Node",
//		Name:      nodeName,
//		Namespace: "",
//		UID:       "",
//	}
//
//	// Record the status update attempt
//	middleware.RecordNormalEvent(c, nodeRef, StatusUpdateReason,
//		"Updating node %s status to '%s'", nodeName, req.Status)
//
//	if err := h.updaterService.ProcessStatusUpdate(ctx, nodeName, req.Status, req.Message); err != nil {
//		log.Printf("Error updating status for node %s: %v", nodeName, err)
//
//		// Record the failure event
//		middleware.RecordWarningEvent(c, nodeRef, StatusUpdateFailedReason,
//			"Failed to update node %s status: %v", nodeName, err)
//
//		c.JSON(http.StatusInternalServerError, gin.H{
//			"error": fmt.Sprintf("failed to update status: %v", err),
//		})
//		return
//	}
//
//	// Record successful update
//	middleware.RecordNormalEvent(c, nodeRef, StatusUpdateReason,
//		"Successfully updated node %s status to '%s'", nodeName, req.Status)
//
//	c.JSON(http.StatusOK, gin.H{
//		"message": "status updated successfully",
//		"node":    nodeName,
//		"status":  req.Status,
//	})
//}
//
//// getNodeStatus retrieves the current status of a node
//func (h *StatusHandler) getNodeStatus(c *gin.Context) {
//	nodeName := c.Param("nodeName")
//	if nodeName == "" {
//		c.JSON(http.StatusBadRequest, gin.H{
//			"error": "node name is required",
//		})
//		return
//	}
//
//	// Create a context with timeout for the operation
//	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
//	defer cancel()
//
//	// Create a node reference for events
//	nodeRef := &v1.ObjectReference{
//		Kind:      "Node",
//		Name:      nodeName,
//		Namespace: "",
//		UID:       "",
//	}
//
//	// Record the status retrieval attempt
//	middleware.RecordNormalEvent(c, nodeRef, StatusRetrievalReason,
//		"Retrieving status for node %s", nodeName)
//
//	status, err := h.updaterService.GetNodeStatus(ctx, nodeName)
//	if err != nil {
//		log.Printf("Error retrieving status for node %s: %v", nodeName, err)
//
//		// Record the failure event
//		middleware.RecordWarningEvent(c, nodeRef, StatusRetrievalFailed,
//			"Failed to retrieve node %s status: %v", nodeName, err)
//
//		c.JSON(http.StatusInternalServerError, gin.H{
//			"error": fmt.Sprintf("failed to retrieve status: %v", err),
//		})
//		return
//	}
//
//	c.JSON(http.StatusOK, status)
//}
//
//// handleStatusWebhook processes status update webhooks from backend systems
//func (h *StatusHandler) handleStatusWebhook(c *gin.Context) {
//	var status domain.UpgradeStatus
//	if err := c.ShouldBindJSON(&status); err != nil {
//		c.JSON(http.StatusBadRequest, gin.H{
//			"error": fmt.Sprintf("invalid webhook payload: %v", err),
//		})
//		return
//	}
//
//	// Create a context with timeout for the operation
//	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
//	defer cancel()
//
//	log.Printf("Webhook received: Status update for node %s, status: %s", status.NodeID, status.Status)
//
//	// Create a node reference for events if we have a node ID
//	var nodeRef *v1.ObjectReference
//	if status.NodeID != "" {
//		nodeRef = &v1.ObjectReference{
//			Kind:      "Node",
//			Name:      status.NodeID,
//			Namespace: "",
//			UID:       "",
//		}
//
//		// Record the webhook received event
//		middleware.RecordNormalEvent(c, nodeRef, StatusWebhookReason,
//			"Received webhook status update for node %s: %s", status.NodeID, status.Status)
//	}
//
//	if err := h.updaterService.ProcessBackendStatusUpdate(ctx, status); err != nil {
//		log.Printf("Error processing backend status update: %v", err)
//
//		if nodeRef != nil {
//			// Record the failure event
//			middleware.RecordWarningEvent(c, nodeRef, StatusUpdateFailedReason,
//				"Failed to process webhook status update: %v", err)
//		}
//
//		c.JSON(http.StatusInternalServerError, gin.H{
//			"error": fmt.Sprintf("failed to process status update: %v", err),
//		})
//		return
//	}
//
//	if nodeRef != nil {
//		// Record successful update
//		middleware.RecordNormalEvent(c, nodeRef, StatusUpdateReason,
//			"Successfully processed webhook status update for node %s", status.NodeID)
//	}
//
//	c.JSON(http.StatusOK, gin.H{
//		"message": "status update processed successfully",
//	})
//}
