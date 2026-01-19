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
	socialPath := cfg.Social.Path
	port := cfg.Running.Port

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 添加全局 CORS 中间件
	r.Use(cors.New(cors.Config{
		// 允许任意来源（包含 file:// 场景的 Origin: null）；比 AllowOrigins:["*"] 更兼容
		AllowOriginFunc: func(origin string) bool { return true },
		AllowMethods:    []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		// 允许前端自定义 header（你前端在用 docid）；同时兼容 docId/doc_id 的写法
		AllowHeaders:  []string{"Origin", "Content-Type", "Accept", "Authorization", "docid", "docId", "doc_id"},
		ExposeHeaders: []string{"Content-Length"},
		// 如果你不依赖 Cookie（多数 token 都放 Authorization），这里建议 false，避免某些浏览器对 * / null 的限制
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	// 文档协作服务
	collabUrl, _ := url.Parse(collabPath)
	collabProxy := httputil.NewSingleHostReverseProxy(collabUrl)

	// 认证服务
	authUrl, _ := url.Parse(authPath)
	authProxy := httputil.NewSingleHostReverseProxy(authUrl)

	// 社交服务
	socialUrl, _ := url.Parse(socialPath)
	socialProxy := httputil.NewSingleHostReverseProxy(socialUrl)

	r.Any("/auth/*any", func(c *gin.Context) {
		// 把 /auth/... 映射到 /v1/auth/...
		c.Request.URL.Path = "/v1" + c.Request.URL.Path
		authProxy.ServeHTTP(c.Writer, c.Request)
	})

	r.Any("/social/*any", func(c *gin.Context) {
		socialProxy.ServeHTTP(c.Writer, c.Request)
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
