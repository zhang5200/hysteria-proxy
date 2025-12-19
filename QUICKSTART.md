# 快速部署指南

## 节点信息

- **zzx.ai.com** (Manager) - ID: `9xsywa43991p65xzubuo1hysa`
- **zhangzhengxing** - ID: `p6axku581yst5b02akryakwvd`
- **www.zzxai.asia** - ID: `vnngxpfansht3u8q1k8dq7dvp`

## 快速部署 (推荐)

使用自动化脚本一键部署:

```bash
./deploy.sh
```

## 手动部署步骤

### 1. 为节点打标签

```bash
# 为所有节点打上 hysteria 标签 (让 hysteria-proxy 在所有节点部署)
docker node update --label-add hysteria=true 9xsywa43991p65xzubuo1hysa
docker node update --label-add hysteria=true p6axku581yst5b02akryakwvd
docker node update --label-add hysteria=true vnngxpfansht3u8q1k8dq7dvp

# 为面板节点打标签 (选择 zhangzhengxing 作为面板节点)
docker node update --label-add role=panel p6axku581yst5b02akryakwvd
```

### 2. 验证标签

```bash
docker node inspect 9xsywa43991p65xzubuo1hysa --format '{{.Spec.Labels}}'
docker node inspect p6axku581yst5b02akryakwvd --format '{{.Spec.Labels}}'
docker node inspect vnngxpfansht3u8q1k8dq7dvp --format '{{.Spec.Labels}}'
```

### 3. 构建镜像

```bash
docker build -f Dockerfile.auth -t hysteria-auth:latest .
docker build -t hysteria-proxy:latest .
```

### 4. 部署服务

```bash
docker stack deploy -c docker-compose.yml hysteria
```

### 5. 查看状态

```bash
# 查看服务列表
docker stack services hysteria

# 查看服务任务分布
docker service ps hysteria_hysteria-proxy
docker service ps hysteria_auth-server

# 查看日志
docker service logs -f hysteria_hysteria-proxy
docker service logs -f hysteria_auth-server
```

## 服务分布

- **auth-server**: 部署在 `zhangzhengxing` 节点 (通过 `role=panel` 标签)
- **hysteria-proxy**: 在所有三个节点上各部署一个副本 (通过 `hysteria=true` 标签)

## 常用命令

```bash
# 更新服务
docker service update --image hysteria-proxy:latest hysteria_hysteria-proxy
docker service update --image hysteria-auth:latest hysteria_auth-server

# 扩缩容 (不适用于 global 模式的 hysteria-proxy)
docker service scale hysteria_auth-server=2

# 删除服务栈
docker stack rm hysteria

# 查看节点状态
docker node ls

# 查看服务详情
docker service inspect hysteria_hysteria-proxy
```

## 故障排查

```bash
# 查看失败的任务
docker service ps --no-trunc hysteria_hysteria-proxy

# 查看容器日志
docker service logs --tail 100 hysteria_hysteria-proxy

# 查看网络
docker network ls | grep hysteria
docker network inspect hysteria_hysteria-net
```

## 端口说明

- **8080**: auth-server 管理面板 (仅在 zhangzhengxing 节点)
- **1443**: Hysteria 主服务端口 (TCP/UDP, 所有节点)
- **8081**: Hysteria 管理端口 (所有节点)

## 注意事项

1. 确保 `config.yaml` 文件存在于当前目录
2. 端口使用 `host` 模式,确保端口未被占用
3. 镜像需要在 manager 节点构建,或使用 registry 分发
4. 数据持久化在 Docker volume 中,注意备份
