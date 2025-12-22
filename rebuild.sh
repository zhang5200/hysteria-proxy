#!/bin/bash

echo "🔄 开始重新构建 Hysteria 服务..."

# 停止并删除容器
echo "📦 停止并删除现有容器..."
docker-compose down

# 删除旧镜像（可选，节省空间）
echo "🗑️  删除旧镜像..."
docker-compose rm -f
docker rmi hysteria-proxy-auth-server hysteria-proxy-hysteria-proxy 2>/dev/null || true

# 重新构建镜像（不使用缓存）
echo "🔨 重新构建镜像（不使用缓存）..."
docker-compose build --no-cache

# 启动服务
echo "🚀 启动服务..."
docker-compose up -d

# 查看服务状态
echo "✅ 服务状态："
docker-compose ps

echo ""
echo "📋 查看日志请运行："
echo "   docker-compose logs -f"
