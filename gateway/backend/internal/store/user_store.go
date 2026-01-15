package store

import (
	"context"
	"database/sql"
)

type UserStore struct{ db *sql.DB }

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) GetUserID(ctx context.Context, username string) (uint64, error) {
	var userID uint64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE username = ?`,
		username,
	).Scan(&userID)
	return userID, err
}
