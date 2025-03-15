package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthHandler provides health check endpoints
type HealthHandler struct {
	startTime time.Time
}

// NewHealthHandler creates a new health handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		startTime: time.Now(),
	}
}

// SetupRoutes registers handler routes to the router
func (h *HealthHandler) SetupRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		api.GET("/health", h.healthCheck)
		api.GET("/readiness", h.readinessCheck)
		api.GET("/liveness", h.livenessCheck)
	}
}

// healthCheck confirms the service is running
func (h *HealthHandler) healthCheck(c *gin.Context) {
	uptime := time.Since(h.startTime).String()

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Service is running",
		"uptime":  uptime,
	})
}

// readinessCheck confirms the service is ready to accept requests
func (h *HealthHandler) readinessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ready",
		"message": "Service is ready to accept requests",
	})
}

// livenessCheck provides a health endpoint for Kubernetes liveness probe
func (h *HealthHandler) livenessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
	})
}
