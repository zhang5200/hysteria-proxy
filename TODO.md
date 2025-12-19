docker buildx build --platform linux/amd64 -f Dockerfile.auth -t hysteria-auth:latest .
docker buildx build --platform linux/amd64 -t hysteria-proxy:latest .
