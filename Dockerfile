FROM debian:bookworm-slim

# 安装必要的工具
RUN apt-get update && \
    apt-get install -y curl openssl ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 创建 hysteria 用户和目录
RUN useradd -r -s /bin/false hysteria && \
    mkdir -p /etc/hysteria

# 下载并安装 Hysteria2
RUN bash -c "$(curl -fsSL https://get.hy2.sh/)" && \
    chmod +x /usr/local/bin/hysteria

# 生成自签证书
RUN openssl req -x509 -nodes -newkey ec:<(openssl ecparam -name prime256v1) \
    -keyout /etc/hysteria/server.key \
    -out /etc/hysteria/server.crt \
    -subj "/CN=bing.com" \
    -days 36500 && \
    chown hysteria:hysteria /etc/hysteria/server.key /etc/hysteria/server.crt

# 复制配置文件
COPY config.yaml /etc/hysteria/config.yaml

# 暴露端口
EXPOSE 443/tcp 443/udp

# 使用 hysteria 用户运行
USER hysteria

# 直接运行 hysteria server (不使用 systemctl)
CMD ["/usr/local/bin/hysteria", "server", "-c", "/etc/hysteria/config.yaml"]
