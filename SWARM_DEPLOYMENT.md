# Docker Swarm 部署指南

## 架构说明

本配置将服务部署到以下节点:

- **64.112.40.188**: Hysteria Proxy 节点
- **66.154.105.113**: 面板节点 (auth-server) + Hysteria Proxy 节点
- **72.18.81.35**: Hysteria Proxy 节点

## 部署前准备

### 1. 构建 Docker 镜像

在部署前,需要先在 master 节点构建镜像:

```bash
# 构建认证服务器镜像
docker build -f Dockerfile.auth -t hysteria-auth:latest .

# 构建 Hysteria 代理镜像
docker build -t hysteria-proxy:latest .
```

### 2. 初始化 Docker Swarm (如果还未初始化)

在 master 节点上执行:

```bash
docker swarm init --advertise-addr <MASTER_NODE_IP>
```

### 3. 将 Worker 节点加入 Swarm

在 master 节点获取 join token:

```bash
docker swarm join-token worker
```

然后在每个 worker 节点 (64.112.40.188, 66.154.105.113, 72.18.81.35) 上执行返回的命令。

### 4. 为节点打标签

根据你的集群节点信息:

- **zzx.ai.com** (Manager, ID: 9xsywa43991p65xzubuo1hysa)
- **zhangzhengxing** (ID: p6axku581yst5b02akryakwvd)
- **www.zzxai.asia** (ID: vnngxpfansht3u8q1k8dq7dvp)

**步骤 1: 为所有三个节点打上 hysteria 标签** (让 hysteria-proxy 在所有节点部署)

```bash
# 为 zzx.ai.com 打标签
docker node update --label-add hysteria=true 9xsywa43991p65xzubuo1hysa

# 为 zhangzhengxing 打标签
docker node update --label-add hysteria=true p6axku581yst5b02akryakwvd

# 为 www.zzxai.asia 打标签
docker node update --label-add hysteria=true vnngxpfansht3u8q1k8dq7dvp
```

**步骤 2: 为面板节点打标签** (指定 auth-server 部署位置)

根据你的需求,选择一个节点作为面板节点(建议选择 zhangzhengxing 或 www.zzxai.asia):

```bash
# 例如,选择 zhangzhengxing 作为面板节点
docker node update --label-add role=panel p6axku581yst5b02akryakwvd
```

**验证标签设置:**

```bash
# 查看节点标签
docker node inspect 9xsywa43991p65xzubuo1hysa --format '{{.Spec.Labels}}'
docker node inspect p6axku581yst5b02akryakwvd --format '{{.Spec.Labels}}'
docker node inspect vnngxpfansht3u8q1k8dq7dvp --format '{{.Spec.Labels}}'
```

## 部署步骤

### 1. 推送镜像到所有节点

由于 Docker Swarm 需要在所有节点上都有镜像,你有两个选择:

**方案 A: 使用 Docker Registry (推荐)**

```bash
# 启动本地 registry
docker service create --name registry --publish published=5000,target=5000 registry:2

# 标记镜像
docker tag hysteria-auth:latest localhost:5000/hysteria-auth:latest
docker tag hysteria-proxy:latest localhost:5000/hysteria-proxy:latest

# 推送到 registry
docker push localhost:5000/hysteria-auth:latest
docker push localhost:5000/hysteria-proxy:latest

# 修改 docker-compose.yml 中的镜像名称为 localhost:5000/hysteria-auth:latest 等
```

**方案 B: 在每个节点上构建镜像**

在每个节点上分别执行构建命令。

### 2. 部署 Stack

```bash
# 部署服务栈
docker stack deploy -c docker-compose.yml hysteria

# 查看服务状态
docker stack services hysteria

# 查看服务详情
docker service ls

# 查看特定服务的任务分布
docker service ps hysteria_hysteria-proxy
docker service ps hysteria_auth-server
```

### 3. 验证部署

```bash
# 检查服务是否正常运行
docker service logs hysteria_auth-server
docker service logs hysteria_hysteria-proxy

# 查看节点上的容器
docker ps
```

## 配置说明

### auth-server 服务

- **部署位置**: 仅在 66.154.105.113 节点
- **副本数**: 1
- **端口**: 8080
- **资源限制**: CPU 0.5 核, 内存 512M

### hysteria-proxy 服务

- **部署模式**: global (在每个标记了 `hysteria=true` 的节点上运行一个副本)
- **端口**:
  - 1443 (TCP/UDP) - Hysteria 主服务
  - 8081 (TCP) - 管理端口
- **端口模式**: host (使用主机网络,避免负载均衡)
- **资源限制**: CPU 1 核, 内存 1G

### 网络

- **类型**: overlay 网络
- **名称**: hysteria-net
- **特性**: attachable (允许独立容器连接)

## 更新服务

```bash
# 更新镜像后重新部署
docker service update --image hysteria-proxy:latest hysteria_hysteria-proxy
docker service update --image hysteria-auth:latest hysteria_auth-server

# 或者重新部署整个 stack
docker stack deploy -c docker-compose.yml hysteria
```

## 删除服务

```bash
# 删除整个 stack
docker stack rm hysteria

# 查看是否还有残留
docker service ls
docker network ls
```

## 故障排查

### 查看服务日志

```bash
# 查看服务日志
docker service logs -f hysteria_hysteria-proxy
docker service logs -f hysteria_auth-server
```

### 查看任务状态

```bash
# 查看失败的任务
docker service ps --no-trunc hysteria_hysteria-proxy

# 查看节点状态
docker node ls
```

### 检查网络

```bash
# 查看 overlay 网络
docker network ls
docker network inspect hysteria_hysteria-net
```

## 注意事项

1. **端口冲突**: 由于使用 `mode: host`,确保每个节点上的端口 1443 和 8081 没有被占用
2. **配置文件**: 确保 `config.yaml` 在部署节点上存在
3. **数据持久化**: auth-server 的数据存储在 Docker volume 中,备份时注意
4. **节点标签**: 确保正确设置了节点标签,否则服务无法调度
5. **镜像分发**: 确保所有节点都能访问到镜像

## 监控

```bash
# 实时监控服务状态
watch docker service ls

# 查看资源使用
docker stats
```
