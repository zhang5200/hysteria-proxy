FROM golang:1.24-alpine AS auth-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY auth_server.go index.html ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/auth-server auth_server.go

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y curl openssl ca-certificates libcap2-bin && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN useradd -r -s /bin/false hysteria && \
    mkdir -p /etc/hysteria /app/data

WORKDIR /app

RUN bash -c "$(curl -fsSL https://get.hy2.sh/)" && \
    chmod +x /usr/local/bin/hysteria

RUN openssl ecparam -name prime256v1 -out /tmp/ecparam.pem && \
    openssl req -x509 -nodes -newkey ec:/tmp/ecparam.pem \
    -keyout /etc/hysteria/server.key \
    -out /etc/hysteria/server.crt \
    -subj "/CN=bing.com" \
    -days 36500 && \
    rm /tmp/ecparam.pem && \
    chown hysteria:hysteria /etc/hysteria/server.key /etc/hysteria/server.crt

COPY config.yaml /etc/hysteria/config.yaml
COPY --from=auth-builder /out/auth-server /usr/local/bin/auth-server
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh && \
    chown -R hysteria:hysteria /app /etc/hysteria && \
    setcap 'cap_net_bind_service=+ep' /usr/local/bin/hysteria || true

EXPOSE 443/tcp 443/udp 8080 8081

ENTRYPOINT ["/entrypoint.sh"]
