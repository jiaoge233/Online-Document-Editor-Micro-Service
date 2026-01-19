可以。我先给你一个**最小可用的 Docker 方式**，能快速拉起 3 主 3 从的 Redis Cluster。  
另外你这句“另外，”后面还没说完，补充一下你的问题我再继续回答。

---

## 用 Docker 启动 Redis Cluster（3 主 3 从）

### 1）准备 `docker-compose.yml`
在项目根目录新建：

```yaml
version: "3.8"
services:
  redis-1:
    image: redis:7
    container_name: redis-1
    command: ["redis-server", "--cluster-enabled", "yes", "--cluster-config-file", "nodes.conf", "--cluster-node-timeout", "5000", "--appendonly", "yes"]
    ports: ["7001:6379"]

  redis-2:
    image: redis:7
    container_name: redis-2
    command: ["redis-server", "--cluster-enabled", "yes", "--cluster-config-file", "nodes.conf", "--cluster-node-timeout", "5000", "--appendonly", "yes"]
    ports: ["7002:6379"]

  redis-3:
    image: redis:7
    container_name: redis-3
    command: ["redis-server", "--cluster-enabled", "yes", "--cluster-config-file", "nodes.conf", "--cluster-node-timeout", "5000", "--appendonly", "yes"]
    ports: ["7003:6379"]

  redis-4:
    image: redis:7
    container_name: redis-4
    command: ["redis-server", "--cluster-enabled", "yes", "--cluster-config-file", "nodes.conf", "--cluster-node-timeout", "5000", "--appendonly", "yes"]
    ports: ["7004:6379"]

  redis-5:
    image: redis:7
    container_name: redis-5
    command: ["redis-server", "--cluster-enabled", "yes", "--cluster-config-file", "nodes.conf", "--cluster-node-timeout", "5000", "--appendonly", "yes"]
    ports: ["7005:6379"]

  redis-6:
    image: redis:7
    container_name: redis-6
    command: ["redis-server", "--cluster-enabled", "yes", "--cluster-config-file", "nodes.conf", "--cluster-node-timeout", "5000", "--appendonly", "yes"]
    ports: ["7006:6379"]
```

---

### 2）启动容器
```bash
docker compose up -d
```

---

### 3）初始化集群
```bash
docker exec -it redis-1 redis-cli --cluster create \
  127.0.0.1:7001 127.0.0.1:7002 127.0.0.1:7003 \
  127.0.0.1:7004 127.0.0.1:7005 127.0.0.1:7006 \
  --cluster-replicas 1
```

---

### 4）检查集群状态
```bash
docker exec -it redis-1 redis-cli -c -p 7001 cluster nodes
```

---

### 5）在 `collabConfig.yaml` 配置
你现在的 `addrs` 列表就可以直接用 `7001~7006`。

---
