package authservice

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

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

	// 1. 查找用户
	u, err := service.GetUserByUsername(c.Request.Context(), login_req.Username)
	if err != nil {
		// 这里虽然 service 返回了 err，但需要判断具体原因
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"}) // 为了安全，统一报这个
		return
	}
	// 虽然 GetWithProtection 保证了 err=nil 时 u!=nil (除非命中空缓存)，但如果逻辑有变动...
	// 如果命中空值缓存，GetWithProtection 返回 nil, nil
	if u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(login_req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "用户名或密码错误",
		})
		return
	}

	// 2.签发 token
	access_token, _, err := SignAccessToken(u.ID, login_req.Username, 30*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "生成访问令牌失败",
		})
		return
	}

	refresh_token, _, err := SignRefreshToken(u.ID, login_req.Username, 7*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "生成刷新令牌失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accessToken":  access_token,
		"refreshToken": refresh_token,
		"expiresIn":    30 * 60, 
		"tokenType":    "Bearer",
		"user": gin.H{
			"username": login_req.Username,
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
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(register_req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "生成密码哈希失败",
		})
		return
	}
	userID, err := service.CreateUser(c.Request.Context(), register_req.Username, passwordHash)
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

	claims, err := ParseToken(refresh_req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refreshToken 无效"})
		return
	}
	if claims.Type != "refresh" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refreshToken 类型错误"})
		return
	}

	new_access_token, _, err := SignAccessToken(claims.UserID, claims.Username, 30*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "更新访问令牌失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accessToken": new_access_token,
		"expiresIn":   30 * 60,
		"tokenType":   "Bearer",
		"user": gin.H{
			"username": claims.Username,
		},
	})
}
