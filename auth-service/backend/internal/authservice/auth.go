package authservice

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"auth-service/backend/internal/user"
)

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RefreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

// Login 只需要 service，其他依赖都在 service 内部
func Login(c *gin.Context, service *user.Service) {
	var login_req loginReq
	if err := c.ShouldBindJSON(&login_req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "JSON格式错误",
			"details": err.Error(),
		})
		return
	}

	result, err := LoginWithPassword(c.Request.Context(), service, login_req.Username, login_req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"}) // 为了安全，统一报这个
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accessToken":  result.AccessToken,
		"refreshToken": result.RefreshToken,
		"expiresIn":    result.ExpiresIn,
		"tokenType":    result.TokenType,
		"user": gin.H{
			"username": result.Username,
		},
	})

}

func Register(c *gin.Context, service *user.Service) {
	var register_req registerReq
	if err := c.ShouldBindJSON(&register_req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "JSON格式错误",
		})
		return
	}
	userID, err := RegisterUser(c.Request.Context(), service, register_req.Username, register_req.Password)
	if err != nil {
		// 这里你可以进一步细化错误类型判断，比如 user.ErrUsernameTaken
		// 为了简单演示，统一返回 500，实际项目中建议区分
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"userID": userID,
	})
}

func Refresh(c *gin.Context) {
	// 1 解析 refreshToken；校验 typ == "refresh"
	// 2 重新签发新的 access 与 refresh
	var refresh_req RefreshReq

	if err := c.ShouldBindJSON(&refresh_req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "JSON格式错误",
			"details": err.Error(),
		})
		return
	}

	result, err := RefreshTokens(refresh_req.RefreshToken)
	if err != nil {
		if err == ErrInvalidRefreshTokenType {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "refreshToken 类型错误"})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refreshToken 无效"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accessToken": result.AccessToken,
		"expiresIn":   result.ExpiresIn,
		"tokenType":   result.TokenType,
		"user": gin.H{
			"username": result.Username,
		},
	})
}
