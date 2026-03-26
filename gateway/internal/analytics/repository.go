package analytics

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

type DocumentMetrics struct {
	// 这里的字段基本和 document_analytics 表一一对应，
	// 这样查询出来后可以直接序列化成 JSON 返回给前端。
	CharacterCount       int    `json:"character_count"`
	WordCount            int    `json:"word_count"`
	LineCount            int    `json:"line_count"`
	ParagraphCount       int    `json:"paragraph_count"`
	EstimatedReadTimeMin int    `json:"estimated_read_time_min"`
	LikeCount            uint64 `json:"like_count"`
	ViewCount            uint64 `json:"view_count"`
	ShareCount           uint64 `json:"share_count"`
	QuestionMarkCount    uint64 `json:"question_mark_count"`
	HotScore             uint64 `json:"hot_score"`
}

type DocumentInfo struct {
	DocID             string          `json:"doc_id"`
	Title             string          `json:"title"`
	OwnerID           uint64          `json:"owner_id"`
	Archived          bool            `json:"archived"`
	CurrentRevision   uint64          `json:"current_revision"`
	DocumentCreatedAt time.Time       `json:"document_created_at"`
	DocumentUpdatedAt time.Time       `json:"document_updated_at"`
	SnapshotCreatedAt *time.Time      `json:"snapshot_created_at,omitempty"`
	AnalyticsSource   string          `json:"analytics_source,omitempty"`
	ComputedAt        *time.Time      `json:"computed_at,omitempty"`
	Metrics           DocumentMetrics `json:"metrics"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetDocumentInfo(ctx context.Context, docID string) (*DocumentInfo, error) {
	var info DocumentInfo
	var archived uint8
	// NullTime 用来接收数据库里的可空时间列。
	// 如果直接用 time.Time，遇到 NULL 时会扫描失败。
	var snapshotCreatedAt sql.NullTime
	var computedAt sql.NullTime

	// 这里查的是 Spark 预计算结果表，而不是在线临时计算。
	// LEFT JOIN documents 的作用是顺手把文档创建/更新时间也带出来，避免前端再查一次。
	err := r.db.QueryRowContext(
		ctx,
		`SELECT
			da.document_id,
			da.title,
			da.owner_id,
			da.archived,
			da.current_revision,
			d.created_at,
			d.updated_at,
			da.snapshot_created_at,
			da.analytics_source,
			da.computed_at,
			da.character_count,
			da.word_count,
			da.line_count,
			da.paragraph_count,
			da.estimated_read_time_min,
			da.like_count,
			da.view_count,
			da.share_count,
			da.question_mark_count,
			da.hot_score
		FROM document_analytics da
		LEFT JOIN documents d ON d.id = da.document_id
		WHERE da.document_id = ?`,
		docID,
	).Scan(
		&info.DocID,
		&info.Title,
		&info.OwnerID,
		&archived,
		&info.CurrentRevision,
		&info.DocumentCreatedAt,
		&info.DocumentUpdatedAt,
		&snapshotCreatedAt,
		&info.AnalyticsSource,
		&computedAt,
		&info.Metrics.CharacterCount,
		&info.Metrics.WordCount,
		&info.Metrics.LineCount,
		&info.Metrics.ParagraphCount,
		&info.Metrics.EstimatedReadTimeMin,
		&info.Metrics.LikeCount,
		&info.Metrics.ViewCount,
		&info.Metrics.ShareCount,
		&info.Metrics.QuestionMarkCount,
		&info.Metrics.HotScore,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		// 1146 = 表不存在。
		// 这通常表示你还没先运行 mysql-init 或还没同步环境。
		// 这里返回 nil, nil，让上层用 404 或空结果处理，而不是直接把服务打挂。
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1146 {
			return nil, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	// MySQL 里常把布尔值存成 TINYINT(1)，这里手动转回 Go 的 bool。
	info.Archived = archived == 1
	if snapshotCreatedAt.Valid {
		info.SnapshotCreatedAt = &snapshotCreatedAt.Time
	}
	if computedAt.Valid {
		info.ComputedAt = &computedAt.Time
	}
	return &info, nil
}
