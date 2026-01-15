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
	GetMembers(ctx context.Context, docID string) ([]uint64, error)
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

var redis_addr = "127.0.0.1:6379"

var rdb = redis.NewClient(&redis.Options{Addr: redis_addr})

func (p *redisPresence) AddMember(ctx context.Context, docID string, userID uint64, username string, ttl time.Duration) error {
	pipe := p.rdb.Pipeline()
	// 为房间添加成员
	pipe.SAdd(ctx, roomKey(docID), userID)
	// 为成员添加心跳键
	pipe.Set(ctx, memberKey(docID, userID), "1", ttl)
	// 为房间添加名字表(哈希)
	pipe.HSet(ctx, namesKey(docID), userID, username)
	_, err := pipe.Exec(ctx)
	return err

}

func (p *redisPresence) GetMembers(ctx context.Context, docID string) ([]uint64, error) {
	key := "presence:room:" + docID
	members, err := p.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	members_uint64 := make([]uint64, len(members))
	for i, member := range members {
		members_uint64[i], err = strconv.ParseUint(member, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	return members_uint64, nil
}

func (p *redisPresence) GetDocuments(ctx context.Context) ([]string, error) {
	var documents []string
	var documents_unprocessed []string
	iter := p.rdb.Scan(ctx, 0, "presence:room:*", 0).Iterator()
	for iter.Next(ctx) {
		documents_unprocessed = append(documents_unprocessed, iter.Val())
	}
	for _, documents_unprocessed := range documents_unprocessed {
		documents = append(documents, strings.ReplaceAll(documents_unprocessed, "presence:room:", ""))
	}
	return documents, nil
}

func NewPresenceCache() PresenceCache {
	return &redisPresence{rdb: rdb}
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
	// step1: get members
	key := "presence:room:" + docID
	userIDs, err := p.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		return nil, nil
	}

	// step2: check TTL
	existscmds := make([]*redis.IntCmd, 0, len(userIDs))
	pipe := p.rdb.Pipeline()
	for _, userID := range userIDs {
		uid, err := strconv.ParseUint(userID, 10, 64)
		if err != nil {
			return nil, err
		}
		// pipe.Exists(ctx, memberKey(docID, uid)) 把“检查该用户的心跳存活键是否存在”的命令加入管道队列。
		// .什么就是加入什么指令
		existscmds = append(existscmds, pipe.Exists(ctx, memberKey(docID, uid)))
	}
	// .Exec() 执行管道队列中的所有命令，并返回结果。
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	// redis中存在的用户就是TTL未过期的用户
	aliveIDs := make([]uint64, 0, len(userIDs))
	aliveKeyFields := make([]string, 0, len(userIDs))
	for i, cmd := range existscmds {
		if cmd.Val() == 1 {
			uid, err := strconv.ParseUint(userIDs[i], 10, 64)
			if err != nil {
				return nil, err
			}
			aliveIDs = append(aliveIDs, uid)
			aliveKeyFields = append(aliveKeyFields, userIDs[i])
		}
	}
	if len(aliveIDs) == 0 {
		return nil, nil
	}

	// step3: get names
	// fields ...string： 可变参数，用于传递多个字段名
	// aliveKeyFields...： 将 aliveKeyFields 展开为多个参数逐个传入
	names, err := p.rdb.HMGet(ctx, namesKey(docID), aliveKeyFields...).Result()
	if err != nil {
		return nil, err
	}
	members := make([]PresenceMember, 0, len(aliveIDs))
	for i, v := range names {
		name := ""
		if v != nil {
			name, _ = v.(string)
		}
		members = append(members, PresenceMember{UserID: aliveIDs[i], Username: name})
	}
	return members, nil
}
