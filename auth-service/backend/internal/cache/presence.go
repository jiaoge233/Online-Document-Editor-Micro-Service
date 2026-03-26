package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"auth-service/backend/internal/entity"
)

type PresenceCache struct {
	redisClusterClient *redis.ClusterClient
	sf                 *singleflight.Group
}

// RedisInteraction 定义缓存层对外暴露的能力
type RedisInteraction interface {
	// 写入缓存 (用于注册或主动更新)
	WriteCache(ctx context.Context, key string, val *entity.User) error

	// 防击穿获取 (用于登录查询)
	GetWithProtection(ctx context.Context, key string, fetchDB func() (*entity.User, bool, error)) (*entity.User, error)
}

func NewPresenceCache(redisClusterClient *redis.ClusterClient) *PresenceCache {
	return &PresenceCache{redisClusterClient: redisClusterClient, sf: &singleflight.Group{}}
}
