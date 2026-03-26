#!/bin/bash

# Redis 集群故障模拟脚本
# 用法: ./test-redis-cluster.sh [场景编号]

REDIS_PASS="123456"
PORTS=(7001 7002 7003 7004 7005 7006)

# 场景 1: 模拟主节点宕机
simulate_master_failure() {
    echo "=== 场景 1: 模拟主节点宕机 ==="
    echo "当前集群状态:"
    docker exec -it redis-node-1 redis-cli -a $REDIS_PASS cluster nodes | grep master

    echo -e "\n停止主节点 redis-node-1..."
    docker stop redis-node-1

    echo "等待故障转移..."
    sleep 10

    echo "检查集群状态:"
    docker exec -it redis-node-2 redis-cli -a $REDIS_PASS cluster nodes | grep master

    echo -e "\n重启节点:"
    docker start redis-node-1

    echo "等待重新加入集群..."
    sleep 15
    docker exec -it redis-node-1 redis-cli -a $REDIS_PASS cluster nodes
}

# 场景 2: 模拟高负载 (QPS 激增)
simulate_high_load() {
    echo "=== 场景 2: 模拟高负载 ==="
    echo "启动并发写入测试 (模拟 QPS 激增)..."

    # 并发写入测试
    for i in {1..10}; do
        redis-benchmark -p 7001 -a $REDIS_PASS -c 50 -n 10000 -t set,get --cluster &
    done

    echo "高负载运行中... (Ctrl+C 停止)"

    # 监控 QPS
    while true; do
        echo "=== 当前集群状态 ==="
        for port in "${PORTS[@]}"; do
            echo "节点 $port:"
            timeout 2 redis-cli -p $port -a $REDIS_PASS info stats | grep -E "(total_connections_received|instantaneous_ops_per_sec)" || echo "节点 $port 连接失败"
        done
        sleep 5
    done
}

# 场景 3: 模拟慢查询
simulate_slow_queries() {
    echo "=== 场景 3: 模拟慢查询 ==="

    # 注入慢操作 (Lua 脚本执行耗时操作)
    SLOW_SCRIPT='
    local start = redis.call("TIME")[1]
    while redis.call("TIME")[1] - start < 5 do
        -- 空循环 5 秒
    end
    return "slow operation completed"
    '

    echo "执行耗时 5 秒的 Lua 脚本..."
    docker exec -it redis-node-1 redis-cli -a $REDIS_PASS --eval "$SLOW_SCRIPT"

    echo -e "\n查看慢查询日志:"
    docker exec -it redis-node-1 redis-cli -a $REDIS_PASS slowlog get 5
}

# 场景 4: 网络分区模拟
simulate_network_partition() {
    echo "=== 场景 4: 模拟网络分区 ==="
    echo "断开 redis-node-1 和其他节点的网络连接..."

    # 创建网络分区 (需要管理员权限)
    docker network disconnect bridge redis-node-1 2>/dev/null || echo "需要管理员权限进行网络分区测试"

    echo "等待 30 秒观察脑裂现象..."
    sleep 30

    echo "恢复网络连接..."
    docker network connect bridge redis-node-1

    echo "检查集群恢复状态:"
    sleep 10
    docker exec -it redis-node-2 redis-cli -a $REDIS_PASS cluster nodes
}

# 场景 5: 监控集群健康状态
monitor_cluster() {
    echo "=== 场景 5: 集群监控 ==="
    while true; do
        clear
        echo "=== Redis 集群健康监控 ==="
        echo "时间: $(date)"

        # 集群整体状态
        echo -e "\n集群信息:"
        docker exec -it redis-node-1 redis-cli -a $REDIS_PASS cluster info 2>/dev/null || echo "集群连接失败"

        # 各节点状态
        echo -e "\n各节点状态:"
        for port in "${PORTS[@]}"; do
            echo -n "节点 $port: "
            timeout 2 redis-cli -p $port -a $REDIS_PASS ping 2>/dev/null && echo "✓" || echo "✗"
        done

        # 慢查询检查
        echo -e "\n慢查询统计:"
        for port in "${PORTS[@]}"; do
            slow_count=$(timeout 2 redis-cli -p $port -a $REDIS_PASS slowlog len 2>/dev/null || echo "0")
            echo "节点 $port 慢查询数量: $slow_count"
        done

        sleep 5
    done
}

# 主菜单
case "$1" in
    1) simulate_master_failure ;;
    2) simulate_high_load ;;
    3) simulate_slow_queries ;;
    4) simulate_network_partition ;;
    5) monitor_cluster ;;
    *)
        echo "Redis 集群故障模拟脚本"
        echo "用法: $0 [场景编号]"
        echo ""
        echo "可用场景:"
        echo "  1: 模拟主节点宕机"
        echo "  2: 模拟高负载 (QPS 激增)"
        echo "  3: 模拟慢查询"
        echo "  4: 模拟网络分区 (需要管理员权限)"
        echo "  5: 集群健康监控"
        echo ""
        echo "示例:"
        echo "  $0 1    # 模拟主节点故障"
        echo "  $0 5    # 启动监控"
        ;;
esac
