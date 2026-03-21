package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func GuestAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		guestIDStr := c.GetHeader("X-Guest-ID")
		if guestIDStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "X-Guest-ID header required"})
			c.Abort()
			return
		}
		guestID, err := uuid.Parse(guestIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid X-Guest-ID format"})
			c.Abort()
			return
		}
		c.Set("guest_id", guestID)
		c.Next()
	}
}
