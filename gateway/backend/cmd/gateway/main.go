package main

import (
	"log"
	"time"

	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/spf13/viper"

	"gateway/backend/config"
)

var (
	buildVersion = "dev"
	buildCommit  = "local"
	buildTime    = ""
)

func initConfig() (*config.Config, error) {
	var cfg config.Config
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./backend/config")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("Failed to unmarshal config: %v", err)
	}
	return &cfg, nil
}

func main() {
	if buildTime == "" {
		buildTime = time.Now().Format(time.RFC3339)
	}
	cfg, err := initConfig()
	if err != nil {
		log.Fatalf("Failed to init config: %v", err)
	}

	authPath := cfg.Auth.Path
	collabPath := cfg.Collab.Path
	port := cfg.Running.Port

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 添加全局 CORS 中间件
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // 允许所有来源
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 文档协作服务
	collabUrl, _ := url.Parse(collabPath)
	collabProxy := httputil.NewSingleHostReverseProxy(collabUrl)

	// 认证服务
	authUrl, _ := url.Parse(authPath)
	authProxy := httputil.NewSingleHostReverseProxy(authUrl)

	r.Any("/auth/*any", func(c *gin.Context) {
		// 把 /auth/... 映射到 /v1/auth/...
		c.Request.URL.Path = "/v1" + c.Request.URL.Path
		authProxy.ServeHTTP(c.Writer, c.Request)
	})

	r.Any("/ws", func(c *gin.Context) {
		log.Printf("ws: %s", c.Request.URL.Path)
		c.Request.URL.Path = "/collab" + c.Request.URL.Path
		collabProxy.ServeHTTP(c.Writer, c.Request)
	})
	r.Any("/ws/*any", func(c *gin.Context) {
		log.Printf("ws: %s", c.Request.URL.Path)
		c.Request.URL.Path = "/collab" + c.Request.URL.Path
		collabProxy.ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	_ = r.Run(":" + strconv.Itoa(port))
}
