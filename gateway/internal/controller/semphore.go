package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type semChan chan struct{}

func SemaphoreMiddleware(limit int) gin.HandlerFunc {
	sem := make(semChan, limit) // 全局共享信号量

	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		select {
		case sem <- struct{}{}:
			// 保证请求处理结束后释放信号量
			defer func() { <-sem }()

			// 继续后续 handler / 中间件
			c.Next()

		case <-ctx.Done():
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests",
			})
			return
		}
	}
}
