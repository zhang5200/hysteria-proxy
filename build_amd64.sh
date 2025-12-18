#!/bin/bash

# Enable Buildx
docker buildx create --use --name mybuilder || true
docker buildx inspect --bootstrap

# Build Hysteria Server (AMD64)
echo "Building Hysteria Server (AMD64)..."
docker buildx build --platform linux/amd64 -t hysteria-server-amd64:latest --load -f Dockerfile .

# Build Auth Server (AMD64)
echo "Building Auth Server (AMD64)..."
docker buildx build --platform linux/amd64 -t hysteria-auth-amd64:latest --load -f Dockerfile.auth .

echo "Build Complete!"
echo "Images created:"
echo "- hysteria-server-amd64:latest"
echo "- hysteria-auth-amd64:latest"

echo ""
echo "To export these images to transfer to another server, run:"
echo "docker save -o hysteria-server-amd64.tar hysteria-server-amd64:latest"
echo "docker save -o hysteria-auth-amd64.tar hysteria-auth-amd64:latest"
