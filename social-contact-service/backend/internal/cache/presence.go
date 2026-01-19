package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// 键名
// Like: 点赞的文档ID string
// QuestionMark: 问题标记的文档ID string
// Share: 分享的文档ID string

// LikedUser: 点赞的用户ID set
// QuestionMarkedUser: 问题标记的用户ID set
// SharedUser: 分享的用户ID set

const (
	LikeKey         = "Like:{docID:%s}" // Like:{docId}
	QuestionMarkKey = "QuestionMark:{docID:%s}"
	ShareKey        = "Share:{docID:%s}"

	LikedUserKey          = "LikedUser:{docID:%s}" // LikeUsers:{docId}
	QuestionMarkedUserKey = "QuestionMarkedUser:{docID:%s}"
	SharedUserKey         = "SharedUser:{docID:%s}"
)

type redisPresence struct {
	redisClusterClient *redis.ClusterClient
}

func NewPresenceCache(redisClusterClient *redis.ClusterClient) *redisPresence {
	return &redisPresence{
		redisClusterClient: redisClusterClient,
	}
}

type PresenceCache interface {
	IncrLike(ctx context.Context, docID string, userID uint64) (uint64, error)
	IncrQuestionMark(ctx context.Context, docID string, userID uint64) (uint64, error)
	IncrShare(ctx context.Context, docID string, userID uint64) (uint64, error)

	DecrLike(ctx context.Context, docID string, userID uint64) (uint64, error)
	DecrQuestionMark(ctx context.Context, docID string, userID uint64) (uint64, error)
	DecrShare(ctx context.Context, docID string, userID uint64) (uint64, error)

	GetLike(ctx context.Context, docID string) (uint64, error)
	GetQuestionMark(ctx context.Context, docID string) (uint64, error)
	GetShare(ctx context.Context, docID string) (uint64, error)
}

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

func evalChangedCountToUint64(res any) (changed bool, cnt uint64, err error) {
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

func (r *redisPresence) IncrLike(ctx context.Context, docID string, userID uint64) (uint64, error) {
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

func (r *redisPresence) IncrQuestionMark(ctx context.Context, docID string, userID uint64) (uint64, error) {
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

func (r *redisPresence) IncrShare(ctx context.Context, docID string, userID uint64) (uint64, error) {
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

func (r *redisPresence) DecrLike(ctx context.Context, docID string, userID uint64) (uint64, error) {
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

func (r *redisPresence) DecrQuestionMark(ctx context.Context, docID string, userID uint64) (uint64, error) {
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

func (r *redisPresence) DecrShare(ctx context.Context, docID string, userID uint64) (uint64, error) {
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

func (r *redisPresence) GetLike(ctx context.Context, docID string) (uint64, error) {
	key := GetLikeKey(docID)
	val, err := r.redisClusterClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return strconv.ParseUint(val, 10, 64)
}

func (r *redisPresence) GetQuestionMark(ctx context.Context, docID string) (uint64, error) {
	key := GetQuestionMarkKey(docID)
	val, err := r.redisClusterClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return strconv.ParseUint(val, 10, 64)
}

func (r *redisPresence) GetShare(ctx context.Context, docID string) (uint64, error) {
	key := GetShareKey(docID)
	val, err := r.redisClusterClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return strconv.ParseUint(val, 10, 64)
}
