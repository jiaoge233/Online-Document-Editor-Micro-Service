package store

import (
	"context"
	"database/sql"
)

type DocumentStore struct{ db *sql.DB }

func NewDocumentStore(db *sql.DB) *DocumentStore {
	return &DocumentStore{db: db}
}

func (s *DocumentStore) GetDocumentID(ctx context.Context, title string) (string, error) {
	var docID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM documents WHERE title = ?`,
		title,
	).Scan(&docID)
	// sql.ErrNoRows
	return docID, err
}

func (s *DocumentStore) CreateDocument(ctx context.Context, ownerID uint64, title string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO documents (owner_id, title) VALUES (?, ?)`,
		ownerID,
		title,
	)
	if err != nil {
		return err
	}
	return nil
}
