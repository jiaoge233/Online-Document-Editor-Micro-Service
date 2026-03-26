package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	authpb "gateway/backend/gen/authpb"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type verifyErrResp struct {
	Error string `json:"error"`
}

type VerifyClaims struct {
	UserID   uint64 `json:"userId"`
	Username string `json:"username"`
	Type     string `json:"type"`
}

func AuthMiddleware(authBaseURL, authGRPCAddr string) gin.HandlerFunc {
	client := &http.Client{}
	verifyURL := strings.TrimRight(authBaseURL, "/") + "/v1/auth/verify"

	var grpcClient authpb.AuthServiceClient
	if authGRPCAddr != "" {
		conn, err := grpc.NewClient(authGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			grpcClient = authpb.NewAuthServiceClient(conn)
		}
	}

	return func(c *gin.Context) {
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

		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
		defer cancel()

		if grpcClient != nil {
			claims, err := verifyByGRPC(ctx, grpcClient, tokenString)
			switch {
			case err == nil:
				applyVerifiedClaims(c, claims)
				return
			case isGRPCFallbackError(err):
				// gRPC 不可用时回退 HTTP，兼容 auth-service 只启了 HTTP 的场景。
			default:
				writeGRPCVerifyError(c, err)
				return
			}
		}

		claims, err := verifyByHTTP(ctx, client, verifyURL, tokenString)
		if err != nil {
			writeHTTPVerifyError(c, err)
			return
		}

		applyVerifiedClaims(c, claims)
	}
}

func verifyByGRPC(ctx context.Context, client authpb.AuthServiceClient, token string) (VerifyClaims, error) {
	resp, err := client.VerifyToken(ctx, &authpb.VerifyTokenRequest{Token: token})
	if err != nil {
		return VerifyClaims{}, err
	}

	return VerifyClaims{
		UserID:   resp.GetUserId(),
		Username: resp.GetUsername(),
		Type:     resp.GetTyp(),
	}, nil
}

func isGRPCFallbackError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	return st.Code() == codes.Unavailable || st.Code() == codes.DeadlineExceeded
}

func writeGRPCVerifyError(c *gin.Context, err error) {
	st, ok := status.FromError(err)
	if !ok {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"code":    "AUTH_UPSTREAM_ERROR",
			"message": "auth-service verify failed",
		})
		return
	}

	switch st.Code() {
	case codes.InvalidArgument:
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"code":    "INVALID_ARGUMENT",
			"message": st.Message(),
		})
	case codes.Unauthenticated:
		msg := st.Message()
		if msg == "" {
			msg = "invalid token"
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"code":    "UNAUTHENTICATED",
			"message": msg,
		})
	default:
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"code":    "AUTH_UPSTREAM_ERROR",
			"message": "auth-service verify failed",
		})
	}
}

func verifyByHTTP(ctx context.Context, client *http.Client, verifyURL, token string) (VerifyClaims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return VerifyClaims{}, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return VerifyClaims{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		var e verifyErrResp
		_ = json.NewDecoder(resp.Body).Decode(&e)
		msg := e.Error
		if msg == "" {
			msg = "invalid token"
		}
		return VerifyClaims{}, &httpVerifyError{statusCode: http.StatusUnauthorized, message: msg}
	}
	if resp.StatusCode != http.StatusOK {
		return VerifyClaims{}, &httpVerifyError{statusCode: http.StatusBadGateway, message: "auth-service verify non-200"}
	}

	var claims VerifyClaims
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return VerifyClaims{}, &httpVerifyError{statusCode: http.StatusBadGateway, message: "invalid verify response"}
	}

	return claims, nil
}

type httpVerifyError struct {
	statusCode int
	message    string
}

func (e *httpVerifyError) Error() string {
	return e.message
}

func writeHTTPVerifyError(c *gin.Context, err error) {
	if e, ok := err.(*httpVerifyError); ok {
		code := "AUTH_UPSTREAM_ERROR"
		if e.statusCode == http.StatusUnauthorized {
			code = "UNAUTHENTICATED"
		}
		c.AbortWithStatusJSON(e.statusCode, gin.H{
			"code":    code,
			"message": e.message,
		})
		return
	}

	c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
		"code":    "AUTH_UPSTREAM_ERROR",
		"message": "auth-service verify failed",
	})
}

func applyVerifiedClaims(c *gin.Context, claims VerifyClaims) {
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

func extractBearer(header string) string {
	if header == "" {
		return ""
	}

	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}

	return ""
}
