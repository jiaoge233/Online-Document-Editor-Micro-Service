package authservice

import (
	"errors"
	"net/http"
	"time"

	"database/sql"

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

func Login(c *gin.Context, db *sql.DB) {
	// 假设前端发送：
	// POST /api/login
	// Content-Type: application/json
	// {"username": "admin", "password": "123456"}
	// ShouldBindJSON 会自动：
	// 1. 解析 JSON 请求体
	// 2. 验证字段是否符合 binding 规则
	// 3. 将数据填充到 req 结构体
	// req.Username = "admin"
	// req.Password = "123456"
	var login_req loginReq
	if err := c.ShouldBindJSON(&login_req); err != nil {
		// http.StatusBadRequest:错误码400
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "JSON格式错误",
			"details": err.Error(),
		})
		return
	}

	u, err := user.GetUserByUsername(c.Request.Context(), db, login_req.Username)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户失败"})
		return
	}

	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(login_req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "用户名或密码错误",
		})
		return
	}

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
		"expiresIn":    30 * 60, // 30分钟，单位秒
		"tokenType":    "Bearer",
		"user": gin.H{
			//"id":       login_req.ID,
			"username": login_req.Username,
			//"role":     login_req.Role,
		},
	})

}

func Register(c *gin.Context, db *sql.DB) {
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
	userID, err := user.CreateUser(c.Request.Context(), db, register_req.Username, passwordHash)
	if err != nil {
		if errors.Is(err, user.ErrUsernameTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"userID": userID,
	})
}

func Refresh(c *gin.Context) {
	// 1) 解析 refreshToken；校验 typ == "refresh"
	// 2) 重新签发新的 access 与 refresh
	var refresh_req RefreshReq

	if err := c.ShouldBindJSON(&refresh_req); err != nil {
		// http.StatusBadRequest:错误码400
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
