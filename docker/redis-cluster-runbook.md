# Redis 集群运行说明（本机宿主机访问）

## 前置条件
- 宿主机 IP：10.197.73.12
- Compose 文件：docker-compose-redis-cluster.yaml
- 密码：123456

## 启动（第一次）
```powershell
docker compose -f docker-compose-redis-cluster.yaml up -d
```

### 只需执行一次：创建集群
```powershell
docker run --rm -it redis:7 redis-cli --cluster create `
  10.197.73.12:7001 10.197.73.12:7002 10.197.73.12:7003 `
  10.197.73.12:7004 10.197.73.12:7005 10.197.73.12:7006 `
  --cluster-replicas 1 -a 123456
```

## 验证状态
```powershell
docker exec -it redis-node-1 redis-cli -a 123456 cluster info
docker exec -it redis-node-1 redis-cli -a 123456 cluster nodes
```

## 关闭 / 重启
- 停止：`docker compose -f docker-compose-redis-cluster.yaml stop`
- 启动：`docker compose -f docker-compose-redis-cluster.yaml start`
- 重启：`docker compose -f docker-compose-redis-cluster.yaml restart`

## 重建集群（清理旧配置后重来）
```powershell
docker compose -f docker-compose-redis-cluster.yaml down -v
docker compose -f docker-compose-redis-cluster.yaml up -d
```
然后再次执行“创建集群”命令。
