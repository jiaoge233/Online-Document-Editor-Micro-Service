package user

import (
	"context"
	"database/sql"
	"time"

	"errors"

	"github.com/go-sql-driver/mysql"
)

type mysqlRepository struct {
	db *sql.DB
}

type User struct {
	ID           uint64
	Username     string
	PasswordHash []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

var ErrUserNotFound = errors.New("user not found")
var ErrUsernameTaken = errors.New("username already taken")
var ErrDeadlineExceeded = errors.New("deadline exceeded")

type Repository interface {
	CreateUser(ctx context.Context, username string, passwordHash []byte) (uint64, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
}

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 3*time.Second)
}

func CreateUser(ctx context.Context, db *sql.DB, username string, passwordHash []byte) (uint64, error) {
	ctx, cancel := withTimeout(ctx)
	// 释放资源
	defer cancel()

	const sql = `
	INSERT INTO users (username, password_hash) VALUES (?, ?);
	`
	// 使用ctx防止超时
	res, err := db.ExecContext(ctx, sql, username, passwordHash)
	if err != nil {
		// 1062 = duplicate key
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return 0, ErrUsernameTaken
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, ErrDeadlineExceeded
		}
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

func GetUserByUsername(ctx context.Context, db *sql.DB, username string) (*User, error) {
	const ddl = `
	SELECT id, username, password_hash FROM users WHERE username = ?;
	`
	var user User
	err := db.QueryRowContext(ctx, ddl, username).Scan(&user.ID, &user.Username, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, err
}
