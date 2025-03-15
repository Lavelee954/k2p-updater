package middleware

import (
	"github.com/gin-gonic/gin"
	"log"
	"time"
)

// LoggingMiddleware adds request logging with timing information
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start time
		startTime := time.Now()

		// Log request
		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()
		log.Printf("Request: %s %s from %s", method, path, clientIP)

		// Process request
		c.Next()

		// End time
		endTime := time.Now()
		latency := endTime.Sub(startTime)

		// Log response with latency
		status := c.Writer.Status()
		log.Printf("Response: %s %s [%d] (%v)", method, path, status, latency)
	}
}
