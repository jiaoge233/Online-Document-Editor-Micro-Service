package cache

import (
	"context"
	"strconv"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type PresenceCache interface {
	AddMember(ctx context.Context, docID string, userID uint64, username string, ttl time.Duration) error
	GetDocuments(ctx context.Context) ([]string, error)
	GetAliveMembersWithNames(ctx context.Context, docID string) ([]PresenceMember, error)
	SetCursor(ctx context.Context, docID string, userID uint64, jsonData []byte, ttl time.Duration) error
	GetCursor(ctx context.Context, docID string, userID uint64) ([]byte, error)
}

// 具体实现：基于 redis 的 PresenceCache
type redisPresence struct {
	rdb *redis.Client
}

type PresenceMember struct {
	UserID   uint64
	Username string
}

func NewRedisPresence(rdb *redis.Client) PresenceCache {
	return &redisPresence{rdb: rdb}
}

func (p *redisPresence) AddMember(ctx context.Context, docID string, userID uint64, username string, ttl time.Duration) error {
	// 刷新TTL也直接调用AddMember即可
	tx := p.rdb.TxPipeline()
	// ZSET score 使用 expireAt（Unix 秒），用于表达“逻辑 TTL”
	expireAt := time.Now().Add(ttl).Unix()
	tx.ZAdd(ctx, roomKey(docID), redis.Z{Score: float64(expireAt), Member: userID})
	// 名字表（Hash）
	tx.HSet(ctx, namesKey(docID), userID, username)
	_, err := tx.Exec(ctx)
	return err

}

func (p *redisPresence) GetDocuments(ctx context.Context) ([]string, error) {
	var documents []string
	iter := p.rdb.Scan(ctx, 0, "presence:room:*", 0).Iterator()
	for iter.Next(ctx) {
		k := iter.Val()
		// 注意：namesKey 也是以 presence:room: 开头（presence:room:names:{docID}），需要过滤掉
		if strings.Contains(k, ":names:") {
			continue
		}
		docID := strings.TrimPrefix(k, "presence:room:")
		if docID != "" {
			documents = append(documents, docID)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return documents, nil
}

func (p *redisPresence) SetCursor(ctx context.Context, docID string, userID uint64, jsonData []byte, ttl time.Duration) error {
	key := "presence:cursor:" + docID + ":" + strconv.FormatUint(userID, 10)
	if err := p.rdb.Set(ctx, key, jsonData, ttl).Err(); err != nil {
		return err
	}
	return nil
}

func (p *redisPresence) GetCursor(ctx context.Context, docID string, userID uint64) ([]byte, error) {
	key := "presence:cursor:" + docID + ":" + strconv.FormatUint(userID, 10)
	cursor, err := p.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	return cursor, nil
}

func (p *redisPresence) GetAliveMembersWithNames(ctx context.Context, docID string) ([]PresenceMember, error) {
	// step1: 清理过期成员，并查询在线成员
	// 约定：score=expireAt（Unix 秒），expireAt <= now 视为过期
	now := time.Now().Unix()
	// lua脚本
	luaScript := `
	-- KEYS[1] = roomKey(docID)   e.g. presence:room:{docID}
	-- KEYS[2] = namesKey(docID)  e.g. presence:room:names:{docID}
	-- ARGV[1] = now (unix seconds)

	local expired = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
	if #expired > 0 then
		redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
		redis.call("HDEL", KEYS[2], unpack(expired))
	end
	return #expired
	`

	script := redis.NewScript(luaScript)
	_, err := script.Run(ctx, p.rdb, []string{roomKey(docID), namesKey(docID)}, now).Int()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	// step2: 查询在线成员
	aliveIDs, err := p.rdb.ZRangeByScore(ctx, roomKey(docID), &redis.ZRangeBy{
		Min: "(" + strconv.FormatInt(now, 10), // > now
		Max: "+inf",
	}).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	if len(aliveIDs) == 0 {
		return nil, nil
	}
	// ZRangeByScore 返回的是 member 的字符串表示，这里解析回 uint64
	aliveIDsUint64 := make([]uint64, 0, len(aliveIDs))
	for _, aliveID := range aliveIDs {
		uid, err := strconv.ParseUint(aliveID, 10, 64)
		if err != nil && err != redis.Nil {
			return nil, err
		}
		aliveIDsUint64 = append(aliveIDsUint64, uid)
	}

	// step3: 批量获取名字
	names, err := p.rdb.HMGet(ctx, namesKey(docID), aliveIDs...).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	members := make([]PresenceMember, 0, len(aliveIDsUint64))
	for i, v := range names {
		name := ""
		if v != nil {
			name, _ = v.(string)
		}
		members = append(members, PresenceMember{UserID: aliveIDsUint64[i], Username: name})
	}
	return members, nil
}
