package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func AgentOriginProtection() gin.HandlerFunc {
	protection := http.NewCrossOriginProtection()
	return func(c *gin.Context) {
		if protection.Check(c.Request) != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Cross-origin agent writes are not allowed", "code": "forbidden"})
			return
		}
		c.Next()
	}
}
