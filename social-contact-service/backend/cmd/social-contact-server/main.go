package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"golang.org/x/sync/singleflight"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"social-contact-service/backend/internal/cache"
	"social-contact-service/backend/internal/handler"
	"social-contact-service/backend/internal/httpapi/middleware"
	"social-contact-service/backend/internal/mysqldb"
)

type SocialContactConfig struct {
	Running struct {
		Port int `mapstructure:"port"`
	} `mapstructure:"running"`
	Redis struct {
		Addrs    []string `mapstructure:"addrs"`
		Password string   `mapstructure:"password"`
	} `mapstructure:"redis"`
	MySQL struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"mysql"`
	Kafka struct {
		Brokers []string `mapstructure:"brokers"`
		Topic   string   `mapstructure:"topic"`
	} `mapstructure:"kafka"`
	Auth struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"auth"`
}

func initConfig() (*SocialContactConfig, error) {
	var cfg = &SocialContactConfig{}
	viper.SetConfigName("SocialContactConfig")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./backend/config")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
func main() {
	cfg, err := initConfig()
	if err != nil {
		log.Fatalf("init config failed: %v", err)
	}
	log.Printf("config: %+v", cfg)

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    cfg.Redis.Addrs,
		Password: cfg.Redis.Password,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("ping redis failed: %v", err)
	}
	defer rdb.Close()

	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("open mysql failed: %v", err)
	}

	docStatsRepo := mysqldb.NewMySQLDocRepo(db)
	sf := singleflight.Group{}
	interactionRepo := cache.NewRedisInteraction(rdb, sf, docStatsRepo)
	h := handler.NewPresenceHandler(interactionRepo)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// CORS：如果你是通过 3000 网关/转发访问（网关也会加 CORS），本服务再加一次会导致
	// Access-Control-Allow-Origin 变成 "null, null" 这种重复值，浏览器会直接拦截。
	// 所以默认关闭；需要直连本服务调试时，设置环境变量 SOCIAL_ENABLE_CORS=1 再启用。
	if os.Getenv("SOCIAL_ENABLE_CORS") == "1" {
		router.Use(cors.New(cors.Config{
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
	}
	// 路由
	r := router.Group("/social")
	r.Use(middleware.AuthMiddleware(cfg.Auth.Path))
	{
		r.POST("/like/increment", h.IncrLike())
		r.POST("/question_mark/increment", h.IncrQuestionMark())
		r.POST("/share/increment", h.IncrShare())

		r.POST("/like/decrement", h.DecrLike())
		r.POST("/question_mark/decrement", h.DecrQuestionMark())
		r.POST("/share/decrement", h.DecrShare())

		r.GET("/like/value", h.GetLike())
		r.GET("/question_mark/value", h.GetQuestionMark())
		r.GET("/share/value", h.GetShare())
	}
	router.Run(fmt.Sprintf(":%d", cfg.Running.Port))
}
