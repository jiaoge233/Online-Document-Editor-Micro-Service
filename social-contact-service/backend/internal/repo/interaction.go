package repo

import (
	"context"
	"social-contact-service/backend/internal/entity"
)

// InteractionRepo 定义了文档互动的业务契约
type InteractionRepo interface {
	IncrLike(ctx context.Context, docID string, userID uint64) (uint64, error)
	IncrQuestionMark(ctx context.Context, docID string, userID uint64) (uint64, error)
	IncrShare(ctx context.Context, docID string, userID uint64) (uint64, error)

	DecrLike(ctx context.Context, docID string, userID uint64) (uint64, error)
	DecrQuestionMark(ctx context.Context, docID string, userID uint64) (uint64, error)
	DecrShare(ctx context.Context, docID string, userID uint64) (uint64, error)

	GetLike(ctx context.Context, docID string) (uint64, error)
	GetQuestionMark(ctx context.Context, docID string) (uint64, error)
	GetShare(ctx context.Context, docID string) (uint64, error)
}

type DocStatsRepo interface {
	GetDocStats(ctx context.Context, docID string) (*entity.DocStats, error)
	// SetDocStats(ctx context.Context, docID string, stats *entity.DocStats) error
}
