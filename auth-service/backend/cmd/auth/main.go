package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"

	"auth-service/backend/internal/authservice"
)

type AuthConfig struct {
	Running struct {
		Port int `mapstructure:"Port"`
	} `mapstructure:"Running"`
	Mysql struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"Mysql"`
}

func initConfig() (*AuthConfig, error) {
	cfg := &AuthConfig{}
	v := viper.New()
	v.SetConfigName("authConfig")
	v.SetConfigType("yaml")
	// 兼容从项目根目录或 backend 目录启动
	v.AddConfigPath("./backend/config")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func main() {
	fmt.Println("Hello, World!")

	cfg, err := initConfig()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}
	log.Printf("Config file loaded: %+v", cfg)
	port := cfg.Running.Port

	dsn := cfg.Mysql.DSN
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}
	defer db.Close()

	r := gin.New()
	// 使用gin.Logger()和gin.Recovery()中间件，记录请求日志和恢复panic
	r.Use(gin.Logger(), gin.Recovery())

	// 路由
	v1 := r.Group("/v1")
	auth := v1.Group("/auth")
	auth.POST("/login", func(c *gin.Context) { authservice.Login(c, db) })
	auth.POST("/register", func(c *gin.Context) { authservice.Register(c, db) })
	auth.POST("/verify", func(c *gin.Context) {
		// 正确返回 200 + JSON(claims)；失败返回 401 + JSON(error)
		authz := c.GetHeader("Authorization")
		// 请求头通常长这样： Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
		parts := strings.SplitN(authz, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid Authorization header"})
			return
		}

		claims, err := authservice.ParseToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"userId":   claims.UserID,
			"username": claims.Username,
			"typ":      claims.Type,
			"exp":      claims.ExpiresAt,
		})
	})
	auth.POST("/refresh", authservice.Refresh)
	auth.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "OK",
		})
	})
	_ = r.Run(fmt.Sprintf(":%d", port))

}
