package cache

import (
	"context"
	"testing"

	redis "github.com/redis/go-redis/v9"
)

func TestAddMember(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	// 若 Redis 未启动则跳过
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("skip: redis not available: %v", err)
	}
	defer rdb.FlushAll(context.Background()).Err()

	// string test
	ctx := context.Background()
	key := "name"
	value := "John Doe"

	err := rdb.Set(ctx, key, value, 0).Err()
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}
	t.Logf("SET %q -> %q", key, value)

	result, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	t.Logf("GET %q -> %q", key, result)
	if result != value {
		t.Fatalf("expected %s, got %s", value, result)
	}

	// list test
	list_name := "test list"
	list_elements := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"}

	result_1, err := rdb.LPush(ctx, list_name, list_elements[0]).Result()
	if err != nil {
		t.Fatalf("LPush error: %v", err)
	}
	t.Logf("LPush %q -> %q", list_name, result_1)

	result_2, err := rdb.LRange(ctx, list_name, 0, -1).Result()
	if err != nil {
		t.Fatalf("LRange error: %v", err)
	}
	t.Logf("LRange %q -> %q", list_name, result_2)

	result_3, err := rdb.LPop(ctx, list_name).Result()
	if err != nil {
		t.Fatalf("LPop error: %v", err)
	}
	t.Logf("LPop %q -> %q", list_name, result_3)

	// set test
	// 不能重复
	set_name := "test set"
	set_elements := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"}

	result_4, err := rdb.SAdd(ctx, set_name, set_elements[0]).Result()
	if err != nil {
		t.Fatalf("SAdd error: %v", err)
	}
	t.Logf("SAdd %q -> %q", set_name, result_4)

	result_5, err := rdb.SMembers(ctx, set_name).Result()
	if err != nil {
		t.Fatalf("SMembers error: %v", err)
	}
	t.Logf("SMembers %q -> %q", set_name, result_5)

	result_6, err := rdb.SIsMember(ctx, set_name, set_elements[0]).Result()
	if err != nil {
		t.Fatalf("SIsMember error: %v", err)
	}
	t.Logf("SIsMember %q -> %t", set_name, result_6)

	result_7, err := rdb.SRem(ctx, set_name, set_elements[0]).Result()
	if err != nil {
		t.Fatalf("SRem error: %v", err)
	}
	t.Logf("SRem %q -> %d", set_name, result_7)

	// zset test
	// 有序集合，每个元素关联一个分数，元素不可以重复，分数可以重复
	// 分数从小到大
	zset_name := "test zset"
	zset_elements := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"}
	zset_scores := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26}

	for i := 0; i < len(zset_elements); i++ {
		// 使用redis.Z结构体包装， CLI中用空格隔开
		_, err := rdb.ZAdd(ctx, zset_name, redis.Z{Score: zset_scores[i], Member: zset_elements[i]}).Result()
		if err != nil {
			t.Fatalf("ZAdd error: %v", err)
		}
		// t.Logf("ZAdd %q -> %q", zset_name, result_8)
	}

	_, _ = rdb.ZRangeWithScores(ctx, zset_name, 0, -1).Result()
	// for i, z := range result_9 {
	// 	// t.Logf("  [%d] Member: %q, Score: %f", i, z.Member, z.Score)
	// }

	result_10, err := rdb.ZRem(ctx, zset_name, zset_elements[0]).Result()
	if err != nil {
		t.Fatalf("ZRem error: %v", err)
	}
	t.Logf("ZRem %q -> %d", zset_name, result_10)

	result_11, err := rdb.ZScore(ctx, zset_name, zset_elements[10]).Result()
	if err != nil {
		t.Fatalf("ZScore error: %v", err)
	}
	t.Logf("ZScore [%d] %q -> %f", 10, zset_name, result_11)

	result_12, err := rdb.ZRank(ctx, zset_name, zset_elements[10]).Result()
	if err != nil {
		t.Fatalf("ZRank error: %v", err)
	}
	t.Logf("ZScore [%d] %q -> %d", 10, zset_name, result_12)

	result_13, err := rdb.ZRevRank(ctx, zset_name, zset_elements[5]).Result()
	if err != nil {
		t.Fatalf("ZRevRank error: %v", err)
	}
	t.Logf("ZRevRank [%d] %q -> %d", 5, zset_name, result_13)

	// Hash test
	// 每个字段（field）只能有一个值。
	hash_name := "test hash"
	hash_field_1 := "name"
	hash_field_2 := "age"
	hash_value_1_list := []string{"John Doe", "Jane Doe", "Jim Doe"}
	hash_value_2_list := []int{20, 21, 22}

	for i := 0; i < len(hash_value_1_list); i++ {
		result_14, err := rdb.HSet(ctx, hash_name, hash_field_1, hash_value_1_list[i]).Result()
		if err != nil {
			t.Fatalf("HSet error: %v", err)
		}
		t.Logf("HSet_1 %q -> %q", hash_name, result_14)
		result_15, err := rdb.HSet(ctx, hash_name, hash_field_2, hash_value_2_list[i]).Result()
		if err != nil {
			t.Fatalf("HSet error: %v", err)
		}
		t.Logf("HSet_2 %q -> %q", hash_name, result_15)
	}

	result_16, err := rdb.HGetAll(ctx, hash_name).Result()
	if err != nil {
		t.Fatalf("HGetAll error: %v", err)
	}
	t.Logf("HGetAll %q -> %q", hash_name, result_16)

	// bitmap test
	bitmap_name := "test bitmap"

	result_17, err := rdb.SetBit(ctx, bitmap_name, 0, 0).Result()
	if err != nil {
		t.Fatalf("SetBit error: %v", err)
	}
	t.Logf("SetBit %q -> %d", bitmap_name, result_17)

	// 提供十六进制设置需要使用Set命令，而不是SetBit命令
	result_19, err := rdb.Set(ctx, bitmap_name, "\xF0", 0).Result()
	if err != nil {
		t.Fatalf("SetBit error: %v", err)
	}
	t.Logf("Set %q -> %s", bitmap_name, result_19)

	result_18, err := rdb.GetBit(ctx, bitmap_name, 1).Result()
	if err != nil {
		t.Fatalf("GetBit error: %v", err)
	}
	t.Logf("GetBit %q -> %d", bitmap_name, result_18)

	result_20, err := rdb.BitCount(ctx, bitmap_name, &redis.BitCount{Start: 0, End: -1}).Result()
	if err != nil {
		t.Fatalf("BitCount error: %v", err)
	}
	t.Logf("BitCount %q -> %d", bitmap_name, result_20)

	// bitfield test
	bitfield_name := "test bitfield"

	result_21, err := rdb.BitField(ctx, bitfield_name,
		"SET", "u8", "0", "1", // level = 1 (占用位 0-7)
		"SET", "u8", "8", "1", // id = 1 (占用位 8-15)
		"SET", "u32", "16", "1000", // money = 1000 (占用位 16-47)
	).Result()
	if err != nil {
		t.Fatalf("BitField error: %v", err)
	}
	t.Logf("BitField %q -> %v (类型: %T)", bitfield_name, result_21, result_21)

	_, err = rdb.BitField(ctx, bitfield_name,
		"INCRBY", "u32", "16", "1000", // 读取 money
	).Result()
	if err != nil {
		t.Fatalf("BitField error: %v", err)
	}

	result_22, err := rdb.BitField(ctx, bitfield_name,
		"GET", "u8", "0",
		"GET", "u8", "8",
		"GET", "u32", "16",
	).Result()
	if err != nil {
		t.Fatalf("BitField error: %v", err)
	}
	t.Logf("读取结果: %v", result_22)
	t.Logf("u8@0: %d", result_22[0])
	t.Logf("u16@8: %d", result_22[1])
	t.Logf("u32@24: %d", result_22[2])
}
