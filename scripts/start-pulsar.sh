#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "=========================================="
echo "Starting Pulsar Standalone with Docker..."
echo "=========================================="

# 检查 Docker 是否运行
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker is not running. Please start Docker first."
    exit 1
fi

# 停止已有的容器
docker-compose down 2>/dev/null || true

# 启动 Pulsar
docker-compose up -d

echo "Waiting for Pulsar to be ready..."

# 等待 Pulsar 启动完成
MAX_RETRIES=60
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if docker exec pulsar-standalone bin/pulsar-admin brokers healthcheck > /dev/null 2>&1; then
        echo "Pulsar is ready!"
        break
    fi

    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "Waiting for Pulsar... ($RETRY_COUNT/$MAX_RETRIES)"
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "Error: Pulsar failed to start within expected time"
    docker-compose logs
    exit 1
fi

# 创建测试 topic
echo "Creating test topic..."
docker exec pulsar-standalone bin/pulsar-admin topics create persistent://public/default/memory-test 2>/dev/null || true

echo ""
echo "=========================================="
echo "Pulsar is running!"
echo "  Broker URL: pulsar://localhost:6650"
echo "  Admin URL:  http://localhost:8080"
echo "  Topic:      persistent://public/default/memory-test"
echo "=========================================="
