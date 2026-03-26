package main

import (
	"database/sql"
	"log"
	"time"

	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"

	"gateway/backend/config"
	"gateway/internal/analytics"
	"gateway/internal/controller"
	"gateway/internal/middleware"
)

var (
	buildTime = ""
)

func initConfig() (*config.Config, error) {
	var cfg config.Config
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	// 这里读取 gateway 自己的配置文件。
	// 当前我们把 Spark 结果查询也放进 gateway 了，所以 MySQL DSN 也在这里配置。
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
	authGRPCAddr := cfg.Auth.GRPCAddr
	collabPath := cfg.Collab.Path
	socialPath := cfg.Social.Path
	mysqlDSN := cfg.MySQL.DSN
	port := cfg.Running.Port

	// sql.Open 只是在本地创建一个 DB 句柄，真正连库通常发生在首次查询时。
	// 这里提前初始化，是为了后面的 /spark/document/info 直接复用这个连接池。
	db, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		log.Fatalf("Failed to open mysql: %v", err)
	}
	defer db.Close()
	// repo 这一层只做一件事：从 Spark 批处理落下来的结果表 document_analytics 读数据。
	repo := analytics.NewRepository(db)

	r := gin.New()
	r.Use(controller.SemaphoreMiddleware(cfg.Semaphore.Limit))

	r.Use(gin.Logger(), gin.Recovery())

	// 添加全局 CORS 中间件
	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool { return true },
		AllowMethods:    []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:    []string{"Origin", "Content-Type", "Accept", "Authorization", "docId"},
		ExposeHeaders:   []string{"Content-Length"},
		// 不依赖 Cookie（多数 token 都放 Authorization）时，这里设置 false，避免某些浏览器对 * / null 的限制
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

	// 这里不再把 /spark 代理给单独服务，而是 gateway 直接查 MySQL。
	// 这样前端入口不变，但真正的计算已经由 spark-jobs 完成并提前落表。
	spark := r.Group("/spark")
	spark.Use(middleware.AuthMiddleware(authPath, authGRPCAddr))
	spark.GET("/document/info", func(c *gin.Context) {
		// 兼容多种传参方式：
		// 1. ?doc_id=1
		// 2. ?docId=1
		// 3. Header: docid / docId
		// 这样前端调接口时更灵活，也和原项目其它接口风格保持一致。
		docID := c.Query("doc_id")
		if docID == "" {
			docID = c.Query("docId")
		}
		if docID == "" {
			docID = c.GetHeader("docid")
		}
		if docID == "" {
			docID = c.GetHeader("docId")
		}
		if docID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing doc_id"})
			return
		}

		// c.Request.Context() 会跟随本次 HTTP 请求生命周期，
		// 如果客户端取消请求，数据库查询也可以尽早感知并停止。
		info, err := repo.GetDocumentInfo(c.Request.Context(), docID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if info == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "spark analytics not found"})
			return
		}
		c.JSON(http.StatusOK, info)
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
