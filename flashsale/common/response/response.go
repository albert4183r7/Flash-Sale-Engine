// Package response defines the JSON envelope shared by every HTTP endpoint in
// the system, so that clients can parse success and failure the same way
// regardless of which service or route produced the response.
package response

import "github.com/gin-gonic/gin"

// Body is the standard response envelope.
type Body struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Success writes a success envelope with the given status code.
func Success(c *gin.Context, status int, message string, data any) {
	c.JSON(status, Body{Success: true, Message: message, Data: data})
}

// Error writes a failure envelope with the given status code. detail should be
// safe to show to a client: never pass raw database or internal errors.
func Error(c *gin.Context, status int, message, detail string) {
	c.JSON(status, Body{Success: false, Message: message, Error: detail})
}

// Abort writes a failure envelope and stops the middleware chain.
func Abort(c *gin.Context, status int, message, detail string) {
	c.AbortWithStatusJSON(status, Body{Success: false, Message: message, Error: detail})
}
