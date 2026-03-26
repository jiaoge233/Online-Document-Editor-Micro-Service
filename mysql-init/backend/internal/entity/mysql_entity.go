package entity

import (
	"gorm.io/gorm"
)

type MysqlEntity struct {
	DB *gorm.DB
}

func NewMysqlEntity(db *gorm.DB) *MysqlEntity {
	return &MysqlEntity{DB: db}
}

func (e *MysqlEntity) CreateUserTable() error {
	sql := `
	CREATE TABLE IF NOT EXISTS users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(255) NOT NULL,
		email VARCHAR(255),
		password VARCHAR(255) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	)
	`
	return e.DB.Exec(sql).Error
}

func (e *MysqlEntity) CreateDocumentsTable() error {
	sql := `
	CREATE TABLE IF NOT EXISTS documents (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		title VARCHAR(256) NOT NULL,
		owner_id BIGINT UNSIGNED NOT NULL,
		archived TINYINT(1) NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX (owner_id)
	)
	`
	return e.DB.Exec(sql).Error
}

func (e *MysqlEntity) CreateDocumentSnapshotsTable() error {
	sql := `
	CREATE TABLE IF NOT EXISTS document_snapshots (
		id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		document_id BIGINT UNSIGNED NOT NULL,
		revision BIGINT UNSIGNED NOT NULL,
		content LONGTEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX (document_id)
	)
	`
	return e.DB.Exec(sql).Error
}

func (e *MysqlEntity) CreateDocumentAnalyticsTable() error {
	// 这张表不是在线写入的业务表，而是 Spark 的“结果表”。
	// 理解上可以把它看成一个离线汇总层：Spark 算完后统一写到这里，
	// gateway 再从这里读，前端就不用直接接触 Spark 任务本身。
	sql := `
	CREATE TABLE IF NOT EXISTS document_analytics (
		document_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
		title VARCHAR(256) NOT NULL,
		owner_id BIGINT UNSIGNED NOT NULL,
		archived TINYINT(1) NOT NULL DEFAULT 0,
		current_revision BIGINT UNSIGNED NOT NULL DEFAULT 0,
		character_count INT NOT NULL DEFAULT 0,
		word_count INT NOT NULL DEFAULT 0,
		line_count INT NOT NULL DEFAULT 0,
		paragraph_count INT NOT NULL DEFAULT 0,
		estimated_read_time_min INT NOT NULL DEFAULT 0,
		like_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
		view_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
		share_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
		question_mark_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
		hot_score BIGINT UNSIGNED NOT NULL DEFAULT 0,
		-- snapshot_created_at: 本次统计使用的最新快照时间
		snapshot_created_at TIMESTAMP NULL DEFAULT NULL,
		-- analytics_source: 标记结果来自哪种计算方式，当前是 spark_batch
		analytics_source VARCHAR(64) NOT NULL DEFAULT 'spark_batch',
		-- computed_at: 这条分析结果是什么时候被 Spark 算出来的
		computed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX (owner_id),
		INDEX (computed_at)
	)
	`
	return e.DB.Exec(sql).Error
}
