#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "=========================================="
echo "Running All Test Scenarios"
echo "=========================================="

# 确保 Pulsar 运行
if ! docker exec pulsar-standalone bin/pulsar-admin brokers healthcheck > /dev/null 2>&1; then
    echo "Pulsar is not running. Starting..."
    ./scripts/start-pulsar.sh
fi

# 场景1: 默认配置 (队列大小 1000)
echo ""
echo "=========================================="
echo "Scenario 1: Default (queue=1000)"
echo "=========================================="
SCENARIO="queue_1000" QUEUE_SIZE=1000 MESSAGE_SIZE=1024 ./scripts/run-test.sh

sleep 3

# 场景2: 小队列 (队列大小 100)
echo ""
echo "=========================================="
echo "Scenario 2: Small queue (queue=100)"
echo "=========================================="
SCENARIO="queue_100" QUEUE_SIZE=100 MESSAGE_SIZE=1024 ./scripts/run-test.sh

sleep 3

# 场景3: 最小队列 (队列大小 10)
echo ""
echo "=========================================="
echo "Scenario 3: Minimal queue (queue=10)"
echo "=========================================="
SCENARIO="queue_10" QUEUE_SIZE=10 MESSAGE_SIZE=1024 ./scripts/run-test.sh

sleep 3

# 场景4: 大消息 (100KB 消息)
echo ""
echo "=========================================="
echo "Scenario 4: Large messages (100KB)"
echo "=========================================="
SCENARIO="large_msg" QUEUE_SIZE=100 MESSAGE_SIZE=102400 ./scripts/run-test.sh

sleep 3

# 场景5: LZ4 压缩
echo ""
echo "=========================================="
echo "Scenario 5: LZ4 compression"
echo "=========================================="
SCENARIO="lz4" QUEUE_SIZE=100 MESSAGE_SIZE=1024 COMPRESSION=lz4 ./scripts/run-test.sh

echo ""
echo "=========================================="
echo "All scenarios completed!"
echo "=========================================="
echo ""
echo "Results summary:"
ls -la results/

echo ""
echo "To compare results:"
echo "  python3 scripts/analyze_results.py"
