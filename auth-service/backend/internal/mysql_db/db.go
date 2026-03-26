package mysqldb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"auth-service/backend/internal/entity"

	"github.com/go-sql-driver/mysql"
)

var ErrUserNotFound = errors.New("user not found")
var ErrUsernameTaken = errors.New("username already taken")
var ErrDeadlineExceeded = errors.New("deadline exceeded")

type MySQLDocRepo struct {
	db *sql.DB
}

func NewMySQLDocRepo(db *sql.DB) *MySQLDocRepo {
	return &MySQLDocRepo{db: db}
}

func (r *MySQLDocRepo) MysqlCreateUser(ctx context.Context, username string, passwordHash []byte) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	// 释放资源
	defer cancel()

	const query = `
	INSERT INTO users (username, password_hash) VALUES (?, ?);
	`
	// 使用ctx防止超时
	res, err := r.db.ExecContext(ctx, query, username, passwordHash)
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
	// res.LastInsertId() 是 Go 语言 database/sql 包中用于获取刚刚插入数据库记录的自增 ID 的方法。
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint64(id), nil
}

func (r *MySQLDocRepo) MysqlGetUserByUsername(ctx context.Context, username string) (*entity.User, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	//
	const query = `
	SELECT id, username, password_hash FROM users WHERE username = ?;
	`
	var user entity.User
	// scan 是 Go 语言 database/sql 包中用于将数据库查询结果扫描到结构体中的方法。
	err := r.db.QueryRowContext(ctx, query, username).Scan(&user.ID, &user.Username, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, ErrUserNotFound
		}
		return nil, false, err
	}
	return &user, true, nil
}
