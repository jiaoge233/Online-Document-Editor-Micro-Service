package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"social-contact-service/backend/internal/repo"
)

// 键名
// Like: 点赞的文档ID string
// QuestionMark: 问题标记的文档ID string
// Share: 分享的文档ID string

// LikedUser: 点赞的用户ID set
// QuestionMarkedUser: 问题标记的用户ID set
// SharedUser: 分享的用户ID set

const (
	// 为何要用{}包住tag ：Redis 会对整个 Key 字符串进行 CRC16 哈希计算（仅{}内部的东西），这样包住让同一个对象分配在同一个机器上面，避免 Lua 脚本等出错
	// 例子： Like:{docID:100} -> 计算 Hash(docID:100) 、LikedUser:{docID:100} -> 计算 Hash(docID:100)
	LikeKey         = "Like:{docID:%s}" // Like:{docId}
	QuestionMarkKey = "QuestionMark:{docID:%s}"
	ShareKey        = "Share:{docID:%s}"

	LikedUserKey          = "LikedUser:{docID:%s}" // LikeUsers:{docId}
	QuestionMarkedUserKey = "QuestionMarkedUser:{docID:%s}"
	SharedUserKey         = "SharedUser:{docID:%s}"
)

type redisInteraction struct {
	redisClusterClient *redis.ClusterClient
	sf                 singleflight.Group
	docStatsRepo       repo.DocStatsRepo
}

// 确保 redisInteraction 实现了 repo.InteractionRepo 接口
var _ repo.InteractionRepo = (*redisInteraction)(nil)

func NewRedisInteraction(redisClusterClient *redis.ClusterClient, sf singleflight.Group, docStatsRepo repo.DocStatsRepo) repo.InteractionRepo {
	return &redisInteraction{
		redisClusterClient: redisClusterClient,
		sf:                 sf,
		docStatsRepo:       docStatsRepo,
	}
}

// 将 any 类型转换为 int64 类型， 无法转换返回错误
func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case uint64:
		if x > uint64(^uint64(0)>>1) {
			return 0, errors.New("integer overflow")
		}
		return int64(x), nil
	case string:
		return strconv.ParseInt(x, 10, 64)
	case []byte:
		return strconv.ParseInt(string(x), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected type: %T", v)
	}
}

// 将 Redis 的返回结果转换为 uint64 类型， 无法转换返回错误
// 用于统计点赞、问题标记、分享的次数
func evalChangedCountToUint64(res any) (changed bool, cnt uint64, err error) {
	// 判断返回结果是否为 []interface{} 类型，并且长度为 2
	// arr的值是lua返回值{1, cnt}、{0, cnt}，ch是1表示本次确实发生了变化，cnt是次数
	arr, ok := res.([]interface{})
	if !ok || len(arr) != 2 {
		return false, 0, errors.New("invalid result")
	}

	ch, err := toInt64(arr[0])
	if err != nil {
		return false, 0, errors.New("invalid result")
	}

	c, err := toInt64(arr[1])
	if err != nil {
		return false, 0, errors.New("invalid result")
	}
	if c < 0 {
		c = 0
	}

	return ch == 1, uint64(c), nil
}

// 获取 key
func GetLikeKey(docID string) string         { return fmt.Sprintf(LikeKey, docID) }
func GetQuestionMarkKey(docID string) string { return fmt.Sprintf(QuestionMarkKey, docID) }
func GetShareKey(docID string) string        { return fmt.Sprintf(ShareKey, docID) }

func GetLikedUserKey(docID string) string          { return fmt.Sprintf(LikedUserKey, docID) }
func GetQuestionMarkedUserKey(docID string) string { return fmt.Sprintf(QuestionMarkedUserKey, docID) }
func GetSharedUserKey(docID string) string         { return fmt.Sprintf(SharedUserKey, docID) }

// 获取 value
// func GetLikeValue(userID string) string { return fmt.Sprintf(LikeValue, userID) }
// func GetQuestionMarkValue(userID string) string { return fmt.Sprintf(QuestionMarkValue, userID) }
// func GetShareValue(userID string) string { return fmt.Sprintf(ShareValue, userID) }

func (r *redisInteraction) IncrLike(ctx context.Context, docID string, userID uint64) (uint64, error) {
	// added： 1：之前不在集合里（第一次点赞），0：之前就在集合里（重复点赞）
	// cnt： 点赞数
	const likeScript = `
	local added = redis.call("SADD", KEYS[1], ARGV[1])
	if added == 1 then
		local cnt = redis.call("INCR", KEYS[2])
		return {1, cnt}
	end
	local v = redis.call("GET", KEYS[2])
	if not v then v = 0 else v = tonumber(v) end
	return {0, v}
	`
	setKey := GetLikedUserKey(docID)
	countKey := GetLikeKey(docID)
	res, err := r.redisClusterClient.Eval(ctx, likeScript, []string{setKey, countKey}, userID).Result()
	if err != nil {
		return 0, err
	}
	_, cnt, err := evalChangedCountToUint64(res)
	return cnt, err
}

func (r *redisInteraction) IncrQuestionMark(ctx context.Context, docID string, userID uint64) (uint64, error) {
	const questionMarkScript = `
	local added = redis.call("SADD", KEYS[1], ARGV[1])
	if added == 1 then
		local cnt = redis.call("INCR", KEYS[2])
		return {1, cnt}
	end
	local v = redis.call("GET", KEYS[2])
	if not v then v = 0 else v = tonumber(v) end
	return {0, v}
	`
	setKey := GetQuestionMarkedUserKey(docID)
	countKey := GetQuestionMarkKey(docID)
	res, err := r.redisClusterClient.Eval(ctx, questionMarkScript, []string{setKey, countKey}, userID).Result()
	if err != nil {
		return 0, err
	}
	_, cnt, err := evalChangedCountToUint64(res)
	return cnt, err
}

func (r *redisInteraction) IncrShare(ctx context.Context, docID string, userID uint64) (uint64, error) {
	const shareScript = `
	local added = redis.call("SADD", KEYS[1], ARGV[1])
	if added == 1 then
		local cnt = redis.call("INCR", KEYS[2])
		return {1, cnt}
	end
	local v = redis.call("GET", KEYS[2])
	if not v then v = 0 else v = tonumber(v) end
	return {0, v}
	`
	setKey := GetSharedUserKey(docID)
	countKey := GetShareKey(docID)
	res, err := r.redisClusterClient.Eval(ctx, shareScript, []string{setKey, countKey}, userID).Result()
	if err != nil {
		return 0, err
	}
	_, cnt, err := evalChangedCountToUint64(res)
	return cnt, err
}

func (r *redisInteraction) DecrLike(ctx context.Context, docID string, userID uint64) (uint64, error) {
	const likeScript = `
	local removed = redis.call("SREM", KEYS[1], ARGV[1])

	if removed == 1 then
	local cnt = redis.call("DECR", KEYS[2])

	-- 兜底：不让变成负数（即使外部出现异常调用也能兜住）
	if cnt < 0 then
		redis.call("SET", KEYS[2], 0)
		cnt = 0
	end

	return {1, cnt} -- 1：本次确实取消了
	end

	local v = redis.call("GET", KEYS[2])
	if not v then v = 0 else v = tonumber(v) end
	return {0, v}     -- 0：本来就没点赞，幂等
	`
	setKey := GetLikedUserKey(docID)
	countKey := GetLikeKey(docID)
	res, err := r.redisClusterClient.Eval(ctx, likeScript, []string{setKey, countKey}, userID).Result()
	if err != nil {
		return 0, err
	}
	_, cnt, err := evalChangedCountToUint64(res)
	return cnt, err
}

func (r *redisInteraction) DecrQuestionMark(ctx context.Context, docID string, userID uint64) (uint64, error) {
	const questionMarkScript = `
	local removed = redis.call("SREM", KEYS[1], ARGV[1])

	if removed == 1 then
	local cnt = redis.call("DECR", KEYS[2])

	-- 兜底：不让变成负数（即使外部出现异常调用也能兜住）
	if cnt < 0 then
		redis.call("SET", KEYS[2], 0)
		cnt = 0
	end

	return {1, cnt} -- 1：本次确实取消了
	end

	local v = redis.call("GET", KEYS[2])
	if not v then v = 0 else v = tonumber(v) end
	return {0, v}     -- 0：本来就没点赞，幂等
	`
	setKey := GetQuestionMarkedUserKey(docID)
	countKey := GetQuestionMarkKey(docID)
	res, err := r.redisClusterClient.Eval(ctx, questionMarkScript, []string{setKey, countKey}, userID).Result()
	if err != nil {
		return 0, err
	}
	_, cnt, err := evalChangedCountToUint64(res)
	return cnt, err
}

func (r *redisInteraction) DecrShare(ctx context.Context, docID string, userID uint64) (uint64, error) {
	const shareScript = `
	local removed = redis.call("SREM", KEYS[1], ARGV[1])

	if removed == 1 then
	local cnt = redis.call("DECR", KEYS[2])

	-- 兜底：不让变成负数（即使外部出现异常调用也能兜住）
	if cnt < 0 then
		redis.call("SET", KEYS[2], 0)
		cnt = 0
	end

	return {1, cnt} -- 1：本次确实取消了
	end

	local v = redis.call("GET", KEYS[2])
	if not v then v = 0 else v = tonumber(v) end
	return {0, v}     -- 0：本来就没点赞，幂等
	`
	setKey := GetSharedUserKey(docID)
	countKey := GetShareKey(docID)
	res, err := r.redisClusterClient.Eval(ctx, shareScript, []string{setKey, countKey}, userID).Result()
	if err != nil {
		return 0, err
	}
	_, cnt, err := evalChangedCountToUint64(res)
	return cnt, err
}

func (r *redisInteraction) GetLike(ctx context.Context, docID string) (uint64, error) {
	key := GetLikeKey(docID)
	return r.getWithProtection(ctx, key, func() (uint64, bool, error) {
		stats, err := r.docStatsRepo.GetDocStats(ctx, docID)
		if err != nil {
			return 0, false, err
		}
		// 触发空值缓存
		if stats == nil {
			return 0, false, nil
		}
		return stats.LikeCount, true, nil
	})
}

func (r *redisInteraction) GetQuestionMark(ctx context.Context, docID string) (uint64, error) {
	key := GetQuestionMarkKey(docID)
	return r.getWithProtection(ctx, key, func() (uint64, bool, error) {
		stats, err := r.docStatsRepo.GetDocStats(ctx, docID)
		if err != nil {
			return 0, false, err
		}
		if stats == nil {
			// 触发空值缓存
			return 0, false, nil
		}
		return stats.QuestionMarkCount, true, nil
	})
}

func (r *redisInteraction) GetShare(ctx context.Context, docID string) (uint64, error) {
	key := GetShareKey(docID)
	return r.getWithProtection(ctx, key, func() (uint64, bool, error) {
		stats, err := r.docStatsRepo.GetDocStats(ctx, docID)
		if err != nil {
			return 0, false, err
		}
		// 触发空值缓存
		if stats == nil {
			return 0, false, nil
		}
		return stats.ShareCount, true, nil
	})
}
