#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# 默认参数
TOTAL_SIZE=${TOTAL_SIZE:-200}  # MB
MESSAGE_SIZE=${MESSAGE_SIZE:-1024}  # bytes
QUEUE_SIZE=${QUEUE_SIZE:-1000}
BATCH_SIZE=${BATCH_SIZE:-50}  # MB
MAX_BATCHES=${MAX_BATCHES:-4}
SCENARIO=${SCENARIO:-"default"}
COMPRESSION=${COMPRESSION:-"none"}

echo "=========================================="
echo "Pulsar Memory Test"
echo "=========================================="
echo "Scenario:         $SCENARIO"
echo "Total size:       ${TOTAL_SIZE} MB"
echo "Message size:     ${MESSAGE_SIZE} bytes"
echo "Queue size:       ${QUEUE_SIZE}"
echo "Batch size:       ${BATCH_SIZE} MB"
echo "Max batches:      ${MAX_BATCHES}"
echo "Compression:      ${COMPRESSION}"
echo "=========================================="

# 确保目录存在
mkdir -p results

# 构建
echo "Building..."
go build -o bin/producer ./cmd/producer
go build -o bin/consumer ./cmd/consumer

# 清理旧订阅（重置 offset）
echo "Resetting subscription..."
docker exec pulsar-standalone bin/pulsar-admin topics unsubscribe \
    persistent://public/default/memory-test -s memory-test-sub 2>/dev/null || true

# 生产数据
echo ""
echo "=========================================="
echo "Phase 1: Producing test data..."
echo "=========================================="
./bin/producer \
    -total=$((TOTAL_SIZE * 1024 * 1024)) \
    -size=${MESSAGE_SIZE} \
    -compression=${COMPRESSION}

sleep 2

# 消费数据并分析内存
echo ""
echo "=========================================="
echo "Phase 2: Consuming and analyzing memory..."
echo "=========================================="
./bin/consumer \
    -batch-size=$((BATCH_SIZE * 1024 * 1024)) \
    -queue-size=${QUEUE_SIZE} \
    -max-batches=${MAX_BATCHES} \
    -scenario="${SCENARIO}" \
    -output=./results

echo ""
echo "=========================================="
echo "Test completed!"
echo "Results saved to: ./results/"
echo ""
echo "To analyze heap profile:"
echo "  go tool pprof -http=:8080 ./results/heap_${SCENARIO}.pprof"
echo "=========================================="
