package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"auth-service/backend/internal/cache"

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

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

// updateCache 更新用户缓存
func updateCache(ctx context.Context, rdb *redis.ClusterClient, user *User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	// 缓存1小时
	return rdb.Set(ctx, cache.AuthLoginKey(user.Username), data, time.Hour).Err()
}

func CreateUser(ctx context.Context, rdb *redis.ClusterClient, db *sql.DB, username string, passwordHash []byte) (uint64, error) {
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

	// 异步或同步写入缓存
	// 这里选择同步写入，保证一致性
	user := &User{
		ID:           uint64(id),
		Username:     username,
		PasswordHash: passwordHash,
	}
	_ = updateCache(ctx, rdb, user)

	return uint64(id), nil
}

func GetUserByUsername(ctx context.Context, db *sql.DB, rdb *redis.ClusterClient, username string) (*User, error) {
	// 1. 查缓存
	cacheKey := cache.AuthLoginKey(username)
	val, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var user User
		if err := json.Unmarshal([]byte(val), &user); err == nil {
			return &user, nil
		}
	}

	// 2. 查 DB
	const ddl = `
	SELECT id, username, password_hash FROM users WHERE username = ?;
	`
	var user User
	err = db.QueryRowContext(ctx, ddl, username).Scan(&user.ID, &user.Username, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	// 3. 回写缓存
	_ = updateCache(ctx, rdb, &user)

	return &user, err
}
