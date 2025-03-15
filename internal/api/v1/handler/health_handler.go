package handler

//import (
//	"net/http"
//
//	"github.com/gin-gonic/gin"
//)
//
//// HealthHandler provides health check endpoints
//type HealthHandler struct{}
//
//// NewHealthHandler creates a new health handler
//func NewHealthHandler() *HealthHandler {
//	return &HealthHandler{}
//}
//
//// SetupRoutes registers handler routes to the router
//func (h *HealthHandler) SetupRoutes(r *gin.Engine) {
//	api := r.Group("/api/v1")
//	{
//		api.GET("/health", h.healthCheck)
//		api.GET("/readiness", h.readinessCheck)
//	}
//}
//
//// healthCheck confirms the service is running
//func (h *HealthHandler) healthCheck(c *gin.Context) {
//	c.JSON(http.StatusOK, gin.H{
//		"status":  "ok",
//		"message": "Service is running",
//	})
//}
//
//// readinessCheck confirms the service is ready to accept requests
//func (h *HealthHandler) readinessCheck(c *gin.Context) {
//	c.JSON(http.StatusOK, gin.H{
//		"status":  "ready",
//		"message": "Service is ready to accept requests",
//	})
//}
