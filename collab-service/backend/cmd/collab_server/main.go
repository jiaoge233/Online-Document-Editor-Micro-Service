package main

import (
	"fmt"
	"log"
	"time"

	"context"
	"database/sql"

	"github.com/IBM/sarama"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"

	"collabServer/backend/internal/cache"
	"collabServer/backend/internal/collab"
	"collabServer/backend/internal/httpapi/middleware"
	"collabServer/backend/internal/store"
	"collabServer/backend/internal/ws"
)

type CollabConfig struct {
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
	Kafka struct {
		Brokers []string `mapstructure:"brokers"`
		Topic   string   `mapstructure:"topic"`
	} `mapstructure:"Kafka"`
	Auth struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"Auth"`
}

func initConfig() (*CollabConfig, error) {
	cfg := &CollabConfig{}
	v := viper.New()
	v.SetConfigName("collabConfig")
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
	cfg, err := initConfig()
	if err != nil {
		log.Fatalf("init config failed: %v", err)
	}
	log.Printf("config: %+v", cfg)

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    cfg.Redis.Addrs,
		Password: cfg.Redis.Password,
	})
	dsn := cfg.Mysql.DSN

	if err = rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer rdb.Close()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// === 初始化 Kafka Producer ===
	kafkaCfg := sarama.NewConfig()
	// SyncProducer 必须开启 Return.Successes
	kafkaCfg.Producer.Return.Successes = true
	kafkaCfg.Producer.RequiredAcks = sarama.WaitForLocal
	producer, err := sarama.NewSyncProducer(cfg.Kafka.Brokers, kafkaCfg)
	if err != nil {
		log.Fatalf("Failed to connect kafka: %v", err)
	}
	defer producer.Close()

	presenceCache := cache.NewRedisPresence(rdb)
	hub := ws.NewHub(presenceCache)
	snapshotStore := store.NewSnapshotStore(db)
	documentStore := store.NewDocumentStore(db)

	// 构造协作引擎具体实现（内存版）
	kafkatSem := collab.NewSemaphoreControl()
	wsSem := collab.NewSemaphoreControl()

	// Kafka 本地队列 + worker 重试发送（方案A增强）
	kafkaDispatcher := collab.NewKafkaDispatcher(
		producer,
		cfg.Kafka.Topic,
		kafkatSem,
		collab.KafkaDispatcherOptions{
			//  Go 允许在数字里用下划线做分隔符，方便阅读
			QueueSize:   10_000,
			Workers:     4,
			MaxRetry:    3,
			BaseBackoff: 50 * time.Millisecond,
			MaxBackoff:  1 * time.Second,
		},
	)

	svc := collab.NewInMemoryService(snapshotStore, documentStore, producer, cfg.Kafka.Topic, kafkaDispatcher)
	manager := ws.NewManager(hub, svc, wsSem)

	r := gin.New()
	// 中间件
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 路由
	//v1 := r.Group("/v1")
	collab := r.Group("/collab")
	// 关键：挂鉴权中间件（会从 Authorization 或 ?token= 提取 token，调用 /v1/auth/verify，并写入 userId/username）
	collab.Use(middleware.AuthMiddleware(cfg.Auth.Path))
	collab.GET("/ws", func(c *gin.Context) { manager.WebSocketConnect(c, hub) })
	collab.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "ok",
		})
	})

	port := cfg.Running.Port
	_ = r.Run(fmt.Sprintf(":%d", port))
}
