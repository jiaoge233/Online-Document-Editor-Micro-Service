# Redis 缓存高可用方案设计文档

## 1. 概述
本文档旨在为 `social-contact-service` 提供一套针对 Redis 集群的缓存可靠性增强方案。通过分层架构设计，将**缓存策略**（防击穿/雪崩/穿透）与**业务逻辑**解耦，确保代码的可维护性和系统的高可用性。

## 2. 系统业务架构设计

下图展示了从用户请求到数据存储的完整业务分层架构：

```text
+-------------------------------------------------------------+
|                1. 接入层 (Access Layer)                      |
|                                                             |
|   [ 客户端 ] ---> [ Gin HTTP API ] ---> [ Auth Middleware ] |
|                                                 |           |
|                                                 v           |
|                                        [ Presence Handler ] |
+-------------------------------------------------------------+
                               |
                               v
+-------------------------------------------------------------+
|                2. 业务逻辑层 (Logic Layer)                   |
|                                                             |
|           [ repo/interaction.go (Interface Definition) ]    |
|           职责: 定义业务契约 (InteractionRepo)                 |
+-------------------------------------------------------------+
                               |
                               v
+-------------------------------------------------------------+
|                3. 基础设施层 (Infrastructure Layer)          |
|                                                             |
|           [ cache/redis_interaction.go (Implementation) ]   |
|           职责: 实现接口 / 组装 Key / 调用策略                 |
|                                                             |
|                          | 调用                              |
|                          v                                  |
|                                                             |
|           [ cache/policy.go (Cache Policy) ]                |
|           职责: 缓存高可用策略 (Singleflight/Jitter/NullObj) |
+-------------------------------------------------------------+
                               |
                               v
+-------------------------------------------------------------+
|                4. 数据持久层 (Data Layer)                    |
|                                                             |
|      [ Redis Cluster ]              [ MySQL Database ]      |
|      (抗热点流量)                     (数据兜底/持久化)       |
+-------------------------------------------------------------+
```

### 层级职责说明
*   **接入层**: 负责 HTTP 协议解析、参数校验、JWT 鉴权。
*   **业务逻辑层 (Domain)**: `repo/interaction.go`。定义纯净的业务接口 `InteractionRepo`，不依赖具体实现技术（Redis/MySQL）。
*   **基础设施层 (Infra)**:
    *   `cache/redis_interaction.go`: 具体的 Redis 实现。负责拼接 Key、执行 Lua 脚本，并持有 `DocStatsRepo` 用于回源。
    *   `cache/policy.go`: **核心策略层**。拦截对 Redis 的读取请求，透明地处理 Singleflight、Jitter TTL 和空值过滤。
    *   `mysqldb/mysql_repo.go`: 具体的 MySQL 实现。负责使用 GORM 查询数据库。
*   **数据持久层**: 物理存储。Redis 负责抗热点流量，MySQL 负责数据兜底和持久化。

## 3. 风险与对策

| 风险类型 | 现象描述 | 核心对策 | 策略层实现 (`policy.go`) |
| :--- | :--- | :--- | :--- |
| **缓存雪崩** | 大量 Key 同时过期 | **随机过期时间 (Jitter)** | `getRandomTTL()`: 基础时间 + 随机浮动 |
| **缓存击穿** | 热点 Key 过期并发穿透 | **请求归并 (Singleflight)** | `sf.Do()`: 合并同 Key 请求，单次执行 |
| **缓存穿透** | 查询不存在的数据 | **空值缓存 (Null Object)** | 查不到时存入 `-1` 并设短 TTL |

## 4. 详细代码落地指南

### 4.1 文件结构

```text
backend/internal/
├── repo/
│   └── interaction.go       # 接口定义：InteractionRepo, DocStatsRepo
├── entity/
│   └── doc_stats.go         # 实体定义：DocStats
├── mysqldb/
│   └── mysql_repo.go        # DB实现：GORM 操作
└── cache/
    ├── redis_interaction.go # 缓存实现：包含 Incr/Decr/Get 及回源逻辑
    └── policy.go            # 策略实现：包含 Singleflight、随机TTL、getWithProtection 等通用方法
```

### 4.2 策略层实现 (`backend/internal/cache/policy.go`)
此文件封装所有脏活累活，对内提供保护能力。

```go
// 核心方法：getWithProtection
// 封装了：Singleflight(防击穿) + Redis Get + 空值判断(防穿透) + 随机TTL(防雪崩)
func (r *redisInteraction) getWithProtection(
    ctx context.Context, 
    key string, 
    fetchDB func() (uint64, bool, error),
) (uint64, error) {
    // 1. Singleflight 合并请求
    val, err, _ := r.sf.Do(key, func() (interface{}, error) {
        // 2. 查缓存
        v, hit, err := r.readCache(ctx, key)
        if hit { return v, nil }
        
        // 3. 回源查 DB
        count, exists, err := fetchDB()
        if err != nil { return 0, err }
        
        // 4. 写回缓存 (存在则写值，不存在则写空值)
        if !exists {
            r.writeNullCache(ctx, key)
            return 0, nil
        }
        r.writeCache(ctx, key, count)
        return count, nil
    })
    return val.(uint64), err
}
```

### 4.3 缓存层集成 (`backend/internal/cache/redis_interaction.go`)
集成 DB Repo，实现自动回源。

```go
func (r *redisInteraction) GetLike(ctx context.Context, docID string) (uint64, error) {
    key := GetLikeKey(docID)
    
    // 调用策略层，传入回源闭包
    return r.getWithProtection(ctx, key, func() (uint64, bool, error) {
        // 真实回源逻辑
        stats, err := r.docStatsRepo.GetDocStats(ctx, docID)
        if err != nil { return 0, false, err }
        
        if stats == nil {
            return 0, false, nil // 数据库也不存在 -> 触发写空值缓存
        }
        return stats.LikeCount, true, nil // 数据库存在 -> 触发写正常缓存
    })
}
```

## 5. 总结
这种**Repository 模式 + 策略分离**的架构使得：
1.  **接口纯净**：`repo` 层不依赖任何第三方库，业务逻辑只依赖接口。
2.  **策略复用**：`policy.go` 中的保护逻辑被所有 Get 方法复用。
3.  **数据兜底**：通过集成 MySQL Repo，确保了缓存失效时数据的可靠性，同时利用 Singleflight 保护了数据库免受高并发冲击。
