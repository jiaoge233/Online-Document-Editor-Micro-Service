package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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
		// PersistTopic 是首次持久化任务进入的主队列。
		PersistTopic string `mapstructure:"persistTopic"`
		// PersistRetryTopic 用于承接业务落库失败后、仍允许继续重试的任务。
		PersistRetryTopic string `mapstructure:"persistRetryTopic"`
		// PersistDLQTopic 存放达到最大重试次数后仍失败的任务，默认不自动消费。
		PersistDLQTopic     string `mapstructure:"persistDLQTopic"`
		PersistGroup        string `mapstructure:"persistGroup"`
		SnapshotDebounceMs  int    `mapstructure:"snapshotDebounceMs"`
		PersistQueueSize    int    `mapstructure:"persistQueueSize"`
		PersistWorkers      int    `mapstructure:"persistWorkers"`
		PersistJobTimeoutMs int    `mapstructure:"persistJobTimeoutMs"`
		// PersistMaxRetry 是业务落库失败后的最大重试次数。
		PersistMaxRetry int `mapstructure:"persistMaxRetry"`
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	// Kafka 本地队列 + worker 重试发送（操作事件）
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

	persistDispatcher := collab.NewPersistDispatcher(
		producer,
		cfg.Kafka.PersistTopic,
		kafkatSem,
		collab.PersistDispatcherOptions{
			QueueSize:   cfg.Kafka.PersistQueueSize,
			Workers:     cfg.Kafka.PersistWorkers,
			MaxRetry:    3,
			BaseBackoff: 50 * time.Millisecond,
			MaxBackoff:  1 * time.Second,
		},
	)

	snapshotDebounce := time.Duration(cfg.Kafka.SnapshotDebounceMs) * time.Millisecond
	if snapshotDebounce <= 0 {
		snapshotDebounce = 2 * time.Second
	}

	svc := collab.NewInMemoryService(
		snapshotStore,
		documentStore,
		producer,
		cfg.Kafka.Topic,
		kafkaDispatcher,
		persistDispatcher,
		snapshotDebounce,
	)

	persistWorkerPool := collab.NewPersistWorkerPool(
		svc,
		cfg.Kafka.PersistQueueSize,
		cfg.Kafka.PersistWorkers,
		time.Duration(cfg.Kafka.PersistJobTimeoutMs)*time.Millisecond,
	)
	persistConsumer, err := collab.NewPersistConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.PersistGroup,
		// consumer 同时订阅主队列和重试队列；死信队列仅用于兜底留存。
		[]string{cfg.Kafka.PersistTopic, cfg.Kafka.PersistRetryTopic},
		cfg.Kafka.PersistRetryTopic,
		cfg.Kafka.PersistDLQTopic,
		cfg.Kafka.PersistMaxRetry,
		producer,
		persistWorkerPool,
	)
	if err != nil {
		log.Fatalf("failed to init persist consumer: %v", err)
	}
	defer persistConsumer.Close()
	persistConsumer.Start(ctx)

	manager := ws.NewManager(hub, svc, wsSem)

	// 清理心跳过期用户
	go func(ctx context.Context) {
		// 创建一个“定时器”，每隔一段时间往一个 channel 里发一个“时间点”。当到了时间点，channel 会收到一个信号。
		// 自带循环语义，会一直循环下去，直到 ctx 被取消。
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// 服务关闭时退出
				return
			// ticker.C ：只读 channel，不会阻塞，当到了时间点，channel 会收到一个信号。
			case <-ticker.C:
				docs, err := presenceCache.GetDocuments(ctx)
				if err != nil {
					log.Printf("failed to get documents: %v", err)
					continue
				}
				for _, doc := range docs {
					if err := presenceCache.CleanExpiredMembers(ctx, doc); err != nil {
						log.Printf("failed to clean expired members for doc %s: %v", doc, err)
					}
				}
			}
		}
	}(ctx)

	r := gin.New()
	// 中间件
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 路由
	//v1 := r.Group("/v1")
	collab := r.Group("/collab")
	// 鉴权中间件（会从 Authorization 或 ?token= 提取 token，调用 /v1/auth/verify，并写入 userId/username）
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
