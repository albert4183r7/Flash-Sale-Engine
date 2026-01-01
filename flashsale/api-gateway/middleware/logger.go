package middleware

import (
	"encoding/json"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// LogEntry represents a structured log entry compatible with Google Cloud Logging
type LogEntry struct {
	Severity    string            `json:"severity"`
	Time        string            `json:"time"`
	Message     string            `json:"message"`
	HTTPRequest *HTTPRequestLog   `json:"httpRequest,omitempty"`
	Labels      map[string]string `json:"logging.googleapis.com/labels,omitempty"`
}

// HTTPRequestLog contains HTTP request details for Cloud Logging
type HTTPRequestLog struct {
	RequestMethod string `json:"requestMethod"`
	RequestURL    string `json:"requestUrl"`
	Status        int    `json:"status"`
	Latency       string `json:"latency"`
	UserAgent     string `json:"userAgent"`
	RemoteIP      string `json:"remoteIp"`
	Protocol      string `json:"protocol"`
}

// jsonEncoder for structured logging
var jsonEncoder = json.NewEncoder(os.Stdout)

// Logger returns a production-ready structured logging middleware
// Outputs JSON format for Cloud Logging compatibility
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Build full path with query
		fullPath := path
		if query != "" {
			fullPath = path + "?" + query
		}

		// Create structured log entry
		entry := LogEntry{
			Severity: getSeverity(c.Writer.Status()),
			Time:     time.Now().UTC().Format(time.RFC3339Nano),
			Message:  "HTTP Request",
			HTTPRequest: &HTTPRequestLog{
				RequestMethod: c.Request.Method,
				RequestURL:    fullPath,
				Status:        c.Writer.Status(),
				Latency:       latency.String(),
				UserAgent:     c.Request.UserAgent(),
				RemoteIP:      c.ClientIP(),
				Protocol:      c.Request.Proto,
			},
			Labels: map[string]string{
				"service":     "api-gateway",
				"environment": getEnv("ENV", "production"),
			},
		}

		// Output JSON to stdout for Cloud Logging
		jsonEncoder.Encode(entry)
	}
}

// getSeverity maps HTTP status codes to Cloud Logging severity levels
func getSeverity(status int) string {
	switch {
	case status >= 500:
		return "ERROR"
	case status >= 400:
		return "WARNING"
	case status >= 300:
		return "INFO"
	default:
		return "INFO"
	}
}

// getEnv gets environment variable with default fallback
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
