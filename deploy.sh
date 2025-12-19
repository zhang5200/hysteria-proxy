#!/bin/bash

# Docker Swarm 部署脚本
# 用于快速部署 Hysteria 服务到 Docker Swarm 集群

set -e

echo "=== Hysteria Docker Swarm 部署脚本 ==="
echo ""

# 节点信息
NODE_MANAGER="9xsywa43991p65xzubuo1hysa"    # zzx.ai.com
NODE_WORKER1="p6axku581yst5b02akryakwvd"    # zhangzhengxing
NODE_WORKER2="vnngxpfansht3u8q1k8dq7dvp"    # www.zzxai.asia

# 选择面板节点 (默认是 zhangzhengxing)
PANEL_NODE="${PANEL_NODE:-$NODE_WORKER1}"

echo "步骤 1: 为节点打标签..."
echo "----------------------------------------"

# 为所有节点打上 hysteria 标签
echo "为 zzx.ai.com 打标签..."
docker node update --label-add hysteria=true $NODE_MANAGER

echo "为 zhangzhengxing 打标签..."
docker node update --label-add hysteria=true $NODE_WORKER1

echo "为 www.zzxai.asia 打标签..."
docker node update --label-add hysteria=true $NODE_WORKER2

# 为面板节点打标签
echo "为面板节点打标签..."
docker node update --label-add role=panel $PANEL_NODE

echo ""
echo "步骤 2: 验证节点标签..."
echo "----------------------------------------"
docker node ls
echo ""
echo "节点标签详情:"
docker node inspect $NODE_MANAGER --format 'zzx.ai.com: {{.Spec.Labels}}'
docker node inspect $NODE_WORKER1 --format 'zhangzhengxing: {{.Spec.Labels}}'
docker node inspect $NODE_WORKER2 --format 'www.zzxai.asia: {{.Spec.Labels}}'

echo ""
echo "步骤 3: 构建 Docker 镜像..."
echo "----------------------------------------"

if [ ! -f "Dockerfile.auth" ]; then
    echo "错误: Dockerfile.auth 不存在!"
    exit 1
fi

if [ ! -f "Dockerfile" ]; then
    echo "错误: Dockerfile 不存在!"
    exit 1
fi

echo "构建 hysteria-auth 镜像..."
docker build -f Dockerfile.auth -t hysteria-auth:latest .

echo "构建 hysteria-proxy 镜像..."
docker build -t hysteria-proxy:latest .

echo ""
echo "步骤 4: 检查配置文件..."
echo "----------------------------------------"

if [ ! -f "config.yaml" ]; then
    echo "警告: config.yaml 不存在,请确保配置文件已准备好!"
fi

if [ ! -f "docker-compose.yml" ]; then
    echo "错误: docker-compose.yml 不存在!"
    exit 1
fi

echo ""
echo "步骤 5: 部署服务栈..."
echo "----------------------------------------"

# 检查是否已存在同名 stack
if docker stack ls | grep -q "hysteria"; then
    echo "检测到已存在的 hysteria stack,是否要删除并重新部署? (y/n)"
    read -r response
    if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo "删除现有 stack..."
        docker stack rm hysteria
        echo "等待服务完全停止..."
        sleep 10
    else
        echo "取消部署"
        exit 0
    fi
fi

echo "部署 hysteria stack..."
docker stack deploy -c docker-compose.yml hysteria

echo ""
echo "步骤 6: 查看部署状态..."
echo "----------------------------------------"

echo "等待服务启动..."
sleep 5

echo ""
echo "服务列表:"
docker stack services hysteria

echo ""
echo "服务任务分布:"
docker service ps hysteria_hysteria-proxy
docker service ps hysteria_auth-server

echo ""
echo "=== 部署完成! ==="
echo ""
echo "使用以下命令查看服务状态:"
echo "  docker stack services hysteria"
echo "  docker service logs -f hysteria_hysteria-proxy"
echo "  docker service logs -f hysteria_auth-server"
echo ""
echo "使用以下命令删除服务:"
echo "  docker stack rm hysteria"
echo ""
