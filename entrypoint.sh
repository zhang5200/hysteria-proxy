#!/bin/sh
set -e

mkdir -p /app/data
chown -R hysteria:hysteria /app/data
cd /app

# 启动鉴权服务
su -s /bin/sh hysteria -c "/usr/local/bin/auth-server" &

# 以 hysteria 用户身份运行主服务
exec su -s /bin/sh hysteria -c "/usr/local/bin/hysteria server -c /etc/hysteria/config.yaml"