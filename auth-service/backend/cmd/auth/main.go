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
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"auth-service/backend/internal/authservice"
	"auth-service/backend/internal/cache"
	mysqldb "auth-service/backend/internal/mysql_db"
	"auth-service/backend/internal/user"
)

type AuthConfig struct {
	Running struct {
		Port int `mapstructure:"Port"`
	} `mapstructure:"Running"`
	Mysql struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"Mysql"`
	Redis struct {
		Addrs    []string `mapstructure:"addrs"`
		Password string   `mapstructure:"password"`
	} `mapstructure:"Redis"`
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

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    cfg.Redis.Addrs,
		Password: cfg.Redis.Password,
	})

	presence := cache.NewPresenceCache(rdb)
	repo := mysqldb.NewMySQLDocRepo(db)
	// 修改：注入 presence 而不是 rdb
	service := user.NewService(repo, presence)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}
	defer db.Close()

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 路由
	v1 := r.Group("/v1")
	auth := v1.Group("/auth")
	// 修改：只传 service
	auth.POST("/login", func(c *gin.Context) { authservice.Login(c, service) })
	auth.POST("/register", func(c *gin.Context) { authservice.Register(c, service) })
	auth.POST("/verify", func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
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
