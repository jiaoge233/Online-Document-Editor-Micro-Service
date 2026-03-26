# Online Document Editor Micro Service

这是一个我用来练习 **微服务协作编辑** 和 **Spark 数据分析** 的项目。
**需要特别说明，此项目前端页面完全由AI生产，而后端由我在AI指导下手写完成。**

我希望它不只是一个“能编辑文档”的系统，也能体现出：

- 在线协作编辑
- 用户认证与网关转发
- 文档互动统计
- 基于 Apache Spark 的离线分析

所以我把这个项目拆成了多个服务，并补了一条 Spark 批处理分析链路。

## 我对这个项目的理解

如果用一句话概括，这个项目做的事情是：

**用户通过前端进入文档协作页面，后端负责认证、协作编辑和互动统计，Spark 负责对文档和互动数据做离线分析，最后再通过网关把分析结果返回给前端展示。**

## 项目结构

我目前把项目大致分成这些部分：

- `gateway`
  系统统一入口，负责路由转发，也负责对外提供 `/spark/document/info`
- `auth-service`
  负责登录、注册、Token 刷新和鉴权；现在同时提供 HTTP 和 gRPC 两套接口
- `collab-service`
  负责在线文档协作编辑和 WebSocket 通信
- `social-contact-service`
  负责点赞、分享、疑问等互动统计
- `mysql-init`
  负责初始化数据库表
- `spark-service/spark-jobs`
  负责 Spark 批处理分析任务
- `frontend`
  测试前端页面，用来演示登录、加入文档、编辑、互动和查看分析结果

## 技术思路

这个项目里我有两条核心链路。

### 1. 在线业务链路

这条链路主要负责“用户正在用系统时”的功能：

- 用户从前端发起登录请求
- `gateway` 把请求转发到 `auth-service`
- `gateway` 在需要鉴权时，会优先通过 gRPC 调用 `auth-service` 的 `VerifyToken`
- 如果 gRPC 暂时不可用，`gateway` 会回退到原来的 HTTP `/v1/auth/verify`
- 用户登录成功后，前端带着 Token 进入 WebSocket 协作
- `collab-service` 处理文档编辑、版本推进、快照保存
- `social-contact-service` 处理点赞、分享、疑问等互动

这部分更偏普通后端微服务。

### 2. Spark 分析链路

这条链路主要负责“离线计算文档分析结果”：

- Spark 从 MySQL 读取 `documents`
- Spark 从 MySQL 读取 `document_snapshots`
- Spark 从 MySQL 读取 `doc_stats`
- Spark 计算正文统计和热度分
- Spark 把结果写回 `document_analytics`
- `gateway` 再从 `document_analytics` 查询结果并返回给前端

我觉得这样拆分比较合理，因为：

- Spark 专注于批处理分析
- 网关专注于提供统一接口
- 前端不用直接接触 Spark

## 当前服务通信方式

目前这个项目的通信方式我没有做成“全量 gRPC”，而是采用了一个更适合当前阶段的混合方式：

- 前端到 `gateway`：还是走普通 HTTP
- 前端到协作链路：还是走 WebSocket
- `gateway` 到 `auth-service` 的鉴权：优先走 gRPC
- `gateway` 到 `auth-service` 的登录 / 注册转发：暂时还是 HTTP
- gRPC 不可用时：`gateway` 会自动回退到 HTTP 鉴权接口

这样处理的原因：

- 前端直接配 gRPC 并不如 HTTP / WebSocket 方便
- 当前最适合先改成 gRPC 的位置，是内部的鉴权调用
- 这样既能体现服务间 RPC 调用，也不会把前端联调链路一下子改复杂

## gRPC 这部分我现在怎么接进去

目前我先把 gRPC 加在了 `auth-service` 上，主要做了这些事情：

- 增加 `proto/auth.proto`
- 给 `auth-service` 增加 gRPC Server
- 给 `gateway` 增加 gRPC Client
- 在 `gateway` 的鉴权中间件里优先调用 `VerifyToken`
- 保留原来的 HTTP `/v1/auth/verify` 作为兜底

换句话说，项目里的认证链路：

- 登录：前端 -> `gateway` -> `auth-service` HTTP
- 鉴权：`gateway` -> `auth-service` gRPC `VerifyToken`
- 回退：如果 gRPC 不通，就退回 HTTP `Verify`
## 当前已经支持的分析内容

Spark 现在会计算这些指标：

- 字符数
- 词数
- 行数
- 段落数
- 预计阅读时长
- 点赞数
- 浏览数
- 分享数
- 疑问数
- 综合热度分 `hot_score`

这些结果会落到 MySQL 的 `document_analytics` 表里。

## 端口说明

当前配置里几个主要服务端口是：

- `gateway`: `3000`
- `auth-service`: `3001`
- `auth-service gRPC`: `50051`
- `collab-service`: `3002`
- `social-contact-service`: `3003`

数据库和缓存相关配置目前默认是：

- MySQL: `localhost:3306`
- Redis Cluster: `localhost:7001` 到 `localhost:7006`
- Kafka: `localhost:9092`

## 运行顺序

如果我要在本地完整跑起来，我会按这个顺序来：

1. 先启动 MySQL
2. 再启动 Redis
3. 再启动 Kafka
4. 运行 `mysql-init`
5. 启动 `auth-service`
6. 启动 `collab-service`
7. 启动 `social-contact-service`
8. 启动 `gateway`
9. 打开 `frontend/test.html` 做联调
10. 在有文档和快照数据后，运行 Spark 批处理任务

## Spark 这部分怎么运行

Spark 脚本放在：

`spark-service/spark-jobs/document_analytics_job.py`

在运行前，我需要确保：

- Spark 已安装
- `pyspark` 可用
- MySQL JDBC 驱动 jar 已准备好
- `document_analytics` 表已经由 `mysql-init` 创建

示例命令可以参考：

```powershell
spark-submit `
  --jars "E:\path\to\mysql-connector-j.jar" `
  "e:\Go_Projects\Online-Document-Editor-Micro-Service\spark-service\spark-jobs\document_analytics_job.py" `
  --mysql-host localhost `
  --mysql-port 3306 `
  --mysql-database app `
  --mysql-user root `
  --mysql-password 123456 `
  --jdbc-jar "E:\path\to\mysql-connector-j.jar"
```

## 前端怎么查看分析结果

前端测试页面在：

`frontend/test.html`

我已经在页面里加了一个 `Spark 文档分析` 面板。  
只要文档已经存在、并且 Spark 批处理任务跑过一次，就可以通过网关接口：

`GET /spark/document/info`

查询并展示分析结果。

## 为什么我觉得这个项目适合写进简历

因为它不只是单一的 CRUD 项目，而是把几种能力放在了一起：

- 微服务拆分
- 网关统一入口
- HTTP + gRPC 混合调用
- WebSocket 协作编辑
- Redis / MySQL / Kafka 配合
- Spark 批处理分析
- 前端可视化展示

如果后续继续扩展，我还可以把它升级成：

- Kafka + Spark Structured Streaming 的实时分析版
- 热门文档排行
- 作者活跃度统计
- 文档编辑趋势分析

## 我后面还想继续完善的点

目前这个项目已经能体现 Spark 的批处理分析思路，但还可以继续增强：

- 增加更完整的 Docker Compose 环境
- 补充统一的启动脚本
- 给 Spark 任务增加定时调度
- 扩展更多分析指标
- 增加更正式的前端页面

对我来说，这个项目现在已经算是一个“在线协作编辑 + Spark 分析”的完整学习型项目。
