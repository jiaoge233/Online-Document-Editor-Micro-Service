package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type verifyErrResp struct {
	Error string `json:"error"`
}

type VerifyClaims struct {
	UserID   uint64 `json:"userId"`
	Username string `json:"username"`
	Type     string `json:"type"` // "access"
}

// authBaseURL 不要带路径：建议是 http://localhost:3001，middleware 自己拼 + "/verify"；否则很容易拼成 .../auth/verify 或双斜杠。
func AuthMiddleware(authBaseURL string) gin.HandlerFunc {
	client := &http.Client{}

	// 统一拼接 verify URL（避免 double slash）
	verifyURL := strings.TrimRight(authBaseURL, "/") + "/v1/auth/verify"

	// 返回一个符合 gin.HandlerFunc 类型的函数
	return func(c *gin.Context) {
		// 1. 从 Authorization 头中提取令牌
		tokenString := extractBearer(c.Request.Header.Get("Authorization"))
		if tokenString == "" {
			// 兼容 WebSocket：浏览器无法自定义 Header，允许从 query ?token= 中获取
			// strings.TrimSpace(...): 防御性处理，去掉可能出现的前后空格或换行，避免无效匹配。
			tokenString = strings.TrimSpace(c.Query("token"))
		}
		if tokenString == "" {
			c.AbortWithStatusJSON(401, gin.H{
				"code":    "UNAUTHENTICATED",
				"message": "Authorization header is missing or invalid",
			})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, bytes.NewReader([]byte("{}")))
		if err != nil {
			c.AbortWithStatusJSON(500, gin.H{"code": "INTERNAL", "message": "build verify request failed"})
			return
		}

		req.Header.Set("Authorization", "Bearer "+tokenString)
		req.Header.Set("Content-Type", "application/json")

		// 发起请求调用
		log.Println("req", req)
		resp, err := client.Do(req)
		if err != nil {
			// 这里包含超时：context deadline exceeded
			c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
				"code":    "AUTH_UPSTREAM_ERROR",
				"message": "auth-service verify failed",
			})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			var e verifyErrResp
			_ = json.NewDecoder(resp.Body).Decode(&e) // 尽力解析错误信息
			msg := e.Error
			if msg == "" {
				msg = "invalid token"
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHENTICATED",
				"message": msg,
			})
			return
		}
		if resp.StatusCode != http.StatusOK {
			c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
				"code":    "AUTH_UPSTREAM_ERROR",
				"message": "auth-service verify non-200",
			})
			return
		}

		// 返回的c.JSON(http.StatusOK, claims) 在 HTTP 响应体（response body）里
		// type Claims struct {
		// 	// Go的结构体标签需要用反引号
		// 	UserID   uint64 `json:"sub"`
		// 	Username string `json:"username"`
		// 	Type     string `json:"typ"`
		// 	jwt.RegisteredClaims
		// }

		var claims VerifyClaims
		if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
			c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
				"code":    "AUTH_UPSTREAM_ERROR",
				"message": "invalid verify response",
			})
			return
		}

		if claims.Type != "" && claims.Type != "access" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHENTICATED",
				"message": "access token required",
			})
			return
		}

		c.Set("userId", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

func extractBearer(header string) string {
	if header == "" {
		return ""
	}

	// 处理 "Bearer" 前缀（大小写不敏感）
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}

	return ""
}
