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
