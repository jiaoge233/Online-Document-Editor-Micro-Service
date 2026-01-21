package mysqldb

import (
	"context"
	"errors"
	"social-contact-service/backend/internal/entity"
	"social-contact-service/backend/internal/repo"

	"gorm.io/gorm"
)

type mysqlDocRepo struct {
	db *gorm.DB
}

func NewMySQLDocRepo(db *gorm.DB) repo.DocStatsRepo {
	return &mysqlDocRepo{db: db}
}

func (r *mysqlDocRepo) GetDocStats(ctx context.Context, docID string) (*entity.DocStats, error) {
	var stats entity.DocStats
	err := r.db.WithContext(ctx).Where("doc_id = ?", docID).First(&stats).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 没找到，返回 nil, nil
		}
		return nil, err
	}
	// 返回结构体，包含文档的全部信息
	return &stats, nil
}
