package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// InternalOnly enforces a shared secret for admin/private endpoints.
// Looks for the secret in header 'X-API-KEY'. If no API key is configured,
// admin is considered disabled and requests are rejected.
func InternalOnly(billingAPIKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if billingAPIKey == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "admin access disabled"})
			c.Abort()
			return
		}
		token := c.GetHeader("X-API-KEY")
		if token == "" || token != billingAPIKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			c.Abort()
			return
		}
		c.Next()
	}
}
