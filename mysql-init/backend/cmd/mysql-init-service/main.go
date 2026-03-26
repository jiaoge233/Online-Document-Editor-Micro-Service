package main

import (
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"mysql-init-service/backend/internal/entity"
)

type CollabConfig struct {
	Mysql struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"Mysql"`
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

	dsn := cfg.Mysql.DSN
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	mysqlEntity := entity.NewMysqlEntity(db)
	mysqlEntity.CreateUserTable()
	mysqlEntity.CreateDocumentsTable()
	mysqlEntity.CreateDocumentSnapshotsTable()
	mysqlEntity.CreateDocumentAnalyticsTable()
}
