package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger records one line per request. It logs the matched route rather than
// the raw path so that identifiers in the URL do not end up in the logs, and it
// never logs headers or bodies, which carry credentials.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		method := c.Request.Method

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		log.Printf("%s %s %d %s %s",
			method, route, c.Writer.Status(), time.Since(start), c.ClientIP())
	}
}
