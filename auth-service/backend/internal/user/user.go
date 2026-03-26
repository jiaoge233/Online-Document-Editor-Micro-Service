package user

import (
	"context"
	"errors"

	"auth-service/backend/internal/cache"
	"auth-service/backend/internal/entity"
	mysqldb "auth-service/backend/internal/mysql_db" // 实际包名是 mysqldb
)

var (
	ErrUserNotFound  = errors.New("user not found")
	ErrUsernameTaken = errors.New("username already taken")
)

// Service 负责业务逻辑编排（缓存 + DB）
type Service struct {
	repo     *mysqldb.MySQLDocRepo
	presence cache.RedisInteraction
}

func NewService(repo *mysqldb.MySQLDocRepo, presence cache.RedisInteraction) *Service {
	return &Service{
		repo:     repo,
		presence: presence,
	}
}

// updateCache 更新用户缓存
func (s *Service) updateCache(ctx context.Context, user *entity.User) error {
	return s.presence.WriteCache(ctx, cache.AuthLoginKey(user.Username), user)
}

func (s *Service) CreateUser(ctx context.Context, username string, passwordHash []byte) (uint64, error) {
	// 1. 写库
	id, err := s.repo.MysqlCreateUser(ctx, username, passwordHash)
	if err != nil {
		return 0, err
	}

	// 2. 写缓存
	user := &entity.User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
	}
	_ = s.updateCache(ctx, user)

	return id, nil
}

func (s *Service) GetUserByUsername(ctx context.Context, username string) (*entity.User, error) {
	user, err := s.presence.GetWithProtection(ctx, cache.AuthLoginKey(username), func() (*entity.User, bool, error) {
		return s.repo.MysqlGetUserByUsername(ctx, username)
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}
