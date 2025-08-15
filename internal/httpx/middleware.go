package httpx

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set("rid", rid)
		c.Writer.Header().Set("X-Request-ID", rid)
		c.Next()
	}
}

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		rid, _ := c.Get("rid")
		log.Printf("[http] rid=%v %s %s status=%d dur=%s",
			rid, c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
	}
}
