package cache

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	BaseTTL          = 24 * time.Hour   // 基础过期时间
	Jitter           = 60 * time.Minute // 随机抖动范围
	EmptyCacheMarker = -1               // 空值标记
)

// 获取随机TTL，防止缓存雪崩
func getRandomTTL() time.Duration {
	// Int63n返回一个int64的值
	return BaseTTL + time.Duration(rand.Int63n(int64(Jitter)))
}

func (r *redisInteraction) readCache(ctx context.Context, key string) (uint64, bool, error) {
	res, err := r.redisClusterClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, false, nil
		}
		return 0, false, err
	}
	// 不能使用ParseUint，遇到 -1 会报错 invalid syntax
	v, err := strconv.ParseInt(res, 10, 64)
	// 空值标记
	if v == EmptyCacheMarker {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return uint64(v), true, nil
}

func (r *redisInteraction) writeCache(ctx context.Context, key string, val uint64) error {
	return r.redisClusterClient.Set(ctx, key, val, getRandomTTL()).Err()
}

// 标记空值缓存，防止缓存穿透
func (r *redisInteraction) writeNullCache(ctx context.Context, key string) error {
	return r.redisClusterClient.Set(ctx, key, EmptyCacheMarker, 5*time.Minute).Err()
}

// 组合策略 (Singleflight + 上述原子操作)
func (r *redisInteraction) getWithProtection(
	ctx context.Context,
	key string,
	fetchDB func() (uint64, bool, error),
) (uint64, error) {
	// 使用 Singleflight 包裹整个流程
	val, err, _ := r.sf.Do(key, func() (interface{}, error) {

		v, hit, err := r.readCache(ctx, key)
		if err != nil && err != redis.Nil {
			return 0, err
		}
		if hit {
			return v, nil
		}

		// 回源 (Redis Miss)，查数据库
		count, exists, err := fetchDB()
		if err != nil {
			return 0, err
		}

		// 填入真实值或者空值缓存，防止缓存穿透
		if !exists {
			r.writeNullCache(ctx, key)
			return 0, nil
		}
		r.writeCache(ctx, key, count)
		return count, nil
	})
	if err != nil {
		return 0, err
	}
	// 使用断言确保不会panic
	if v, ok := val.(uint64); ok {
		return v, nil
	}
	return 0, errors.New("internal type error")
}
