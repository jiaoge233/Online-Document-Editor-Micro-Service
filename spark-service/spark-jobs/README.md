# Spark Jobs

这个目录专门放项目里的 **Apache Spark 批处理任务**。  
我把它单独拿出来，是因为我希望项目里“谁负责计算、谁负责对外提供接口”这件事更清楚：

- `spark-jobs` 负责 **离线计算**
- `gateway` 负责 **对外查询**

这样前端不需要直接接触 Spark，Spark 也不用关心网页怎么展示。

## 当前任务

目前这里有一个批处理任务：`document_analytics_job.py`

这个脚本做的事情可以按下面理解：

1. 从 MySQL 读取 `documents`
2. 从 MySQL 读取 `document_snapshots`
3. 从 MySQL 读取 `doc_stats`
4. 用 PySpark 计算文档统计信息
5. 把结果写回 `document_analytics`
6. 最后由 `gateway` 提供 `/spark/document/info` 给前端查询

## 我为什么这样设计

一开始我也考虑过直接在 Go 服务里临时计算，但那样更像“普通后端统计逻辑”。  
为了让项目里真正体现 Spark 的作用，我把“计算”这一步放到了 Spark 批任务中。

这样做之后，项目里 Spark 的职责就比较明确了：

- Spark 负责批量分析数据
- 网关负责读取结果
- 前端负责展示结果

从简历表达上，也更容易写成：

- 使用 Apache Spark 对文档快照和互动数据做离线分析
- 将分析结果落库，并通过网关统一提供查询接口

## 运行前提

在运行这个脚本前，我需要先准备这些环境：

1. 已安装 Apache Spark
2. 已安装 `pyspark`
3. 已准备好 MySQL JDBC 驱动，例如 `mysql-connector-j-9.x.x.jar`
4. 已先运行 `mysql-init-service`，确保 `document_analytics` 表已经创建
5. MySQL 里已经有文档、快照和互动统计数据

## 示例命令

下面是我本地测试时可以参考的 `spark-submit` 写法：

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

## 参数说明

这个脚本现在支持的主要参数有：

- `--mysql-host`
  MySQL 主机地址，默认是 `localhost`
- `--mysql-port`
  MySQL 端口，默认是 `3306`
- `--mysql-database`
  数据库名，当前项目默认是 `app`
- `--mysql-user`
  MySQL 用户名
- `--mysql-password`
  MySQL 密码
- `--jdbc-jar`
  MySQL JDBC 驱动 jar 路径。这个参数很重要，没有它 Spark 无法通过 JDBC 连接 MySQL
- `--output-table`
  输出表名，默认是 `document_analytics`
- `--app-name`
  Spark 作业名，运行时会显示在 Spark UI 或日志中

## 这个任务会产出什么

Spark 会把结果写到 `document_analytics` 表里，里面主要是这些信息：

- 文档基础信息：标题、作者、是否归档
- 最新快照信息：当前版本、快照时间
- 正文统计信息：字符数、词数、行数、段落数、预计阅读时长
- 互动统计信息：点赞数、浏览数、分享数、疑问数
- 综合热度分：`hot_score`
- 结果元信息：`analytics_source`、`computed_at`

## 在整体项目里的位置

我现在把这部分理解成下面这条链路：

- `collab-service` 负责在线协作编辑
- `social-contact-service` 负责互动数据维护
- `spark-jobs/document_analytics_job.py` 负责离线分析
- `gateway` 负责把 Spark 结果返回给前端
- `frontend/test.html` 负责展示分析结果

## 后续还能怎么扩展

如果后面我想把它继续升级成“更像大数据项目”的版本，可以继续做这些事：

- 接入 Kafka，把文档操作事件流接进来
- 使用 Spark Structured Streaming 做实时分析
- 增加更多指标，比如活跃作者、编辑频率、热门文档排行

目前这个版本已经属于一个比较完整的 **Spark 批处理分析链路**。
