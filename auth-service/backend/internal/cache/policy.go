package cache

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"

	"auth-service/backend/internal/entity"
)

var (
	BaseTTL          = 24 * time.Hour   // 基础过期时间
	Jitter           = 60 * time.Minute // 随机抖动范围
	EmptyCacheMarker = "NULL"           // 空值标记
)

// 获取随机TTL，防止缓存雪崩
func getRandomTTL() time.Duration {
	// Int63n返回一个int64的值
	return BaseTTL + time.Duration(rand.Int63n(int64(Jitter)))
}

func (p *PresenceCache) readCache(ctx context.Context, key string) (*entity.User, bool, error) {
	res, err := p.redisClusterClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	// 空值标记
	if res == EmptyCacheMarker {
		return nil, true, nil
	}
	var user entity.User
	err = json.Unmarshal([]byte(res), &user)
	if err != nil {
		return nil, false, err
	}
	return &user, true, nil
}

func (p *PresenceCache) WriteCache(ctx context.Context, key string, val *entity.User) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return p.redisClusterClient.Set(ctx, key, data, getRandomTTL()).Err()
}

// 标记空值缓存，防止缓存穿透
func (p *PresenceCache) writeNullCache(ctx context.Context, key string) error {
	return p.redisClusterClient.Set(ctx, key, EmptyCacheMarker, 5*time.Minute).Err()
}

// 组合策略 (Singleflight + 原子操作)
func (p *PresenceCache) GetWithProtection(
	ctx context.Context,
	key string,
	fetchDB func() (*entity.User, bool, error),
) (user *entity.User, err error) {
	// 使用 Singleflight 包裹整个流程
	val, err, _ := p.sf.Do(key, func() (interface{}, error) {

		v, hit, err := p.readCache(ctx, key)
		if err != nil && err != redis.Nil {
			return 0, err
		}
		if hit {
			return v, nil
		}

		// 回源 (Redis Miss)，查数据库
		user, exists, err := fetchDB()
		if err != nil {
			return nil, err
		}

		// 填入空值缓存，防止缓存穿透
		if !exists {
			p.writeNullCache(ctx, key)
			return nil, nil
		}
		p.WriteCache(ctx, key, user)
		return user, nil
	})
	if err != nil {
		return nil, err
	}
	// 使用断言确保不会panic
	if user, ok := val.(*entity.User); ok {
		return user, nil
	}
	return nil, errors.New("internal type error")
}
