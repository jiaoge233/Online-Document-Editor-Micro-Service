package middleware

import (
	"bytes"
	"context"
	"encoding/json"
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
	Type     string `json:"type"`
}

func AuthMiddleware(authBaseURL string) gin.HandlerFunc {
	client := &http.Client{}
	// authBaseURL 只传服务根地址，例如 http://localhost:3001
	// 这里统一补上 /v1/auth/verify，避免每个调用方自己拼路径。
	verifyURL := strings.TrimRight(authBaseURL, "/") + "/v1/auth/verify"

	return func(c *gin.Context) {
		// 浏览器普通 HTTP 请求优先走 Authorization 头；
		// 如果没有，再兼容从 query 里拿 token，和 WebSocket 场景保持一致。
		tokenString := extractBearer(c.Request.Header.Get("Authorization"))
		if tokenString == "" {
			tokenString = strings.TrimSpace(c.Query("token"))
		}
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHENTICATED",
				"message": "Authorization header is missing or invalid",
			})
			return
		}

		// 给鉴权请求一个较短超时，避免 auth 服务异常时把 gateway 长时间拖住。
		// 1200ms 不是固定标准值，只是这里作为微服务间调用的一个保守上限。
		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
		defer cancel()

		// verify 接口只需要 token，本项目里 body 内容不重要，所以传一个空 JSON 即可。
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, bytes.NewReader([]byte("{}")))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    "INTERNAL",
				"message": "build verify request failed",
			})
			return
		}

		req.Header.Set("Authorization", "Bearer "+tokenString)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
				"code":    "AUTH_UPSTREAM_ERROR",
				"message": "auth-service verify failed",
			})
			return
		}
		defer resp.Body.Close()

		// 401 说明 token 本身有问题；
		// 非 200/401 则通常表示上游 auth 服务异常，所以这里返回 502。
		if resp.StatusCode == http.StatusUnauthorized {
			var e verifyErrResp
			_ = json.NewDecoder(resp.Body).Decode(&e)
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

		var claims VerifyClaims
		// claims 是认证服务返回的“已验证用户身份”。
		// 后面的业务处理函数就可以从 gin.Context 里直接拿 userId / username。
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

	// 标准 Authorization 头格式一般是：
	// Authorization: Bearer <token>
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}

	return ""
}
