package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-sql-driver/mysql"
)

type SnapshotStore struct{ db *sql.DB }

func NewSnapshotStore(db *sql.DB) *SnapshotStore {
	return &SnapshotStore{db: db}
}

func (s *SnapshotStore) SaveDocumentSnapshot(ctx context.Context, docID string, rev uint64, content string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO document_snapshots (document_id, revision, content)
		VALUES (?, ?, ?)`,
		docID,
		rev,
		content,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil
			// return fmt.Errorf("duplicate snapshot for doc %s rev %d: %w", docID, rev, err)
		}
		return err
	}
	return nil
}
