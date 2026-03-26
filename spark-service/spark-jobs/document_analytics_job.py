import argparse

from pyspark.sql import SparkSession, Window
from pyspark.sql import functions as F
from pyspark.sql.types import IntegerType


def parse_args() -> argparse.Namespace:
    # 这些参数主要是为了让 spark-submit 更容易复用：
    # 换机器时通常只需要改 MySQL 连接信息和 JDBC jar 路径。
    parser = argparse.ArgumentParser(description="Compute document analytics with Apache Spark.")
    parser.add_argument("--mysql-host", default="localhost")
    parser.add_argument("--mysql-port", default="3306")
    parser.add_argument("--mysql-database", default="app")
    parser.add_argument("--mysql-user", default="root")
    parser.add_argument("--mysql-password", default="123456")
    parser.add_argument("--jdbc-jar", default="", help="Path to mysql-connector-j jar.")
    parser.add_argument("--output-table", default="document_analytics")
    parser.add_argument("--app-name", default="document-analytics-batch")
    return parser.parse_args()


def build_spark(app_name: str, jdbc_jar: str) -> SparkSession:
    # app_name 会显示在 Spark UI / 日志里，方便区分不同批任务。
    # jdbc_jar 是 MySQL 驱动；不传的话，Spark 就不知道怎么连 MySQL。
    builder = SparkSession.builder.appName(app_name)
    if jdbc_jar:
        builder = builder.config("spark.jars", jdbc_jar)
    return builder.getOrCreate()


def build_jdbc_url(args: argparse.Namespace) -> str:
    return (
        f"jdbc:mysql://{args.mysql_host}:{args.mysql_port}/{args.mysql_database}"
        "?useUnicode=true&characterEncoding=UTF-8&serverTimezone=Asia/Shanghai"
    )


def read_table(spark: SparkSession, jdbc_url: str, table: str, props: dict):
    # spark.read.jdbc 是 Spark 读关系型数据库最直接的方式。
    # 这里先保持最小实现：直接整表读取，方便学习和演示。
    return spark.read.jdbc(url=jdbc_url, table=table, properties=props)


def count_words(text: str) -> int:
    if not text:
        return 0
    return len(text.split())


def count_paragraphs(text: str) -> int:
    if not text:
        return 0
    normalized = text.replace("\r\n", "\n")
    return sum(1 for part in normalized.split("\n\n") if part.strip())


def compute_document_analytics(spark: SparkSession, jdbc_url: str, props: dict):
    # documents：文档基础信息
    # document_snapshots：历史快照，正文内容在这里
    # doc_stats：社交互动统计
    documents = read_table(spark, jdbc_url, "documents", props)
    snapshots = read_table(spark, jdbc_url, "document_snapshots", props)
    doc_stats = read_table(spark, jdbc_url, "doc_stats", props)

    # 一个文档会有多条快照，这里用窗口函数选“最新的一条”：
    # partitionBy(document_id) 表示按文档分组；
    # orderBy(revision desc, id desc) 表示版本越新越靠前。
    latest_snapshot_window = Window.partitionBy("document_id").orderBy(F.desc("revision"), F.desc("id"))
    latest_snapshots = (
        snapshots.withColumn("rn", F.row_number().over(latest_snapshot_window))
        .filter(F.col("rn") == 1)
        .drop("rn", "id")
    )

    # 这里把 Python 函数注册成 UDF，交给 Spark 在分布式执行阶段调用。
    # 对学习项目来说好理解；如果后续追求性能，可以再改成更多原生 Spark SQL 表达式。
    word_count_udf = F.udf(count_words, IntegerType())
    paragraph_count_udf = F.udf(count_paragraphs, IntegerType())

    # coalesce 的作用是“取第一个非空值”。
    # 这里把 NULL 内容转成空字符串，避免 length / split / UDF 计算时报错。
    content_col = F.coalesce(F.col("content"), F.lit(""))
    analytics = (
        documents.alias("d")
        .join(latest_snapshots.alias("s"), F.col("d.id") == F.col("s.document_id"), "left")
        .join(doc_stats.alias("st"), F.col("d.id") == F.col("st.doc_id"), "left")
        .select(
            F.col("d.id").alias("document_id"),
            F.col("d.title").alias("title"),
            F.col("d.owner_id").alias("owner_id"),
            F.col("d.archived").alias("archived"),
            F.coalesce(F.col("s.revision"), F.lit(0)).alias("current_revision"),
            F.length(content_col).alias("character_count"),
            word_count_udf(content_col).alias("word_count"),
            F.when(F.length(content_col) == 0, F.lit(0))
            .otherwise(F.size(F.split(content_col, "\n")))
            .alias("line_count"),
            paragraph_count_udf(content_col).alias("paragraph_count"),
            F.ceil(word_count_udf(content_col) / F.lit(200.0)).cast("int").alias("estimated_read_time_min"),
            F.coalesce(F.col("st.like_count"), F.lit(0)).alias("like_count"),
            F.coalesce(F.col("st.view_count"), F.lit(0)).alias("view_count"),
            F.coalesce(F.col("st.share_count"), F.lit(0)).alias("share_count"),
            F.coalesce(F.col("st.question_mark_count"), F.lit(0)).alias("question_mark_count"),
            F.col("s.created_at").alias("snapshot_created_at"),
            F.lit("spark_batch").alias("analytics_source"),
            F.current_timestamp().alias("computed_at"),
        )
        # 这里额外修正一下阅读时长：空文档不应该出现 1 分钟。
        .withColumn(
            "estimated_read_time_min",
            F.when(F.col("word_count") <= 0, F.lit(0)).otherwise(F.col("estimated_read_time_min")),
        )
        # hot_score 是我们自己定义的一个简单热度分公式，
        # 目的是让前端或后续榜单展示时，有一个可直接排序的综合指标。
        .withColumn(
            "hot_score",
            F.col("like_count") + F.col("view_count") + (F.col("share_count") * 3) + (F.col("question_mark_count") * 2),
        )
    )
    return analytics


def write_document_analytics(df, jdbc_url: str, table: str, props: dict) -> None:
    # overwrite + truncate=true 的组合表示：
    # 每次跑批都用最新结果覆盖整张统计表，而不是一批批往后追加。
    # 这样查询层读取会更简单，也适合当前这个“离线汇总表”场景。
    (
        df.write.mode("overwrite")
        .option("truncate", "true")
        .jdbc(url=jdbc_url, table=table, properties=props)
    )


def main() -> None:
    args = parse_args()
    spark = build_spark(args.app_name, args.jdbc_jar)
    jdbc_url = build_jdbc_url(args)
    # driver 必须和你传入的 JDBC jar 对应；这里用的是 MySQL 8+ 常见驱动类名。
    jdbc_props = {
        "user": args.mysql_user,
        "password": args.mysql_password,
        "driver": "com.mysql.cj.jdbc.Driver",
    }

    analytics_df = compute_document_analytics(spark, jdbc_url, jdbc_props)
    write_document_analytics(analytics_df, jdbc_url, args.output_table, jdbc_props)
    spark.stop()


if __name__ == "__main__":
    main()
