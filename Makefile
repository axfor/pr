.PHONY: all build clean clean-results start-pulsar stop-pulsar produce consume test test-all analyze help
.PHONY: test-memory test-memory-stress test-memory-compare test-pprof-collect generate-flamegraphs open-flamegraphs

# Go 编译参数
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# 测试参数 (可通过环境变量覆盖)
TOTAL_SIZE ?= 200
MESSAGE_SIZE ?= 1024
QUEUE_SIZE ?= 1000
BATCH_SIZE ?= 10
MAX_BATCHES ?= 4
SCENARIO ?= default
COMPRESSION ?= none
PPROF_PORT ?= 6060
STRESS_DURATION ?= 120

# 压测参数 (默认 500MB 数据，约1-2分钟完成)
STRESS_TOTAL_SIZE ?= 500
STRESS_MAX_BATCHES ?= 50

help:
	@echo "Pulsar Memory Test Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build              - Build producer and consumer"
	@echo "  make start-pulsar       - Start Pulsar with Docker"
	@echo "  make stop-pulsar        - Stop Pulsar"
	@echo "  make produce            - Produce test messages"
	@echo "  make consume            - Consume messages and analyze memory"
	@echo "  make test               - Run memory comparison test (with/without ReleasePayload)"
	@echo "  make test-memory        - Run quick memory test"
	@echo "  make test-memory-stress - Run long-duration stress test with pprof"
	@echo "  make test-memory-compare- Compare memory usage with/without ReleasePayload"
	@echo "  make test-all           - Run all test scenarios"
	@echo "  make analyze            - Analyze test results"
	@echo "  make clean              - Clean build artifacts"
	@echo ""
	@echo "Environment variables:"
	@echo "  TOTAL_SIZE       - Total data to produce in MB (default: 200)"
	@echo "  MESSAGE_SIZE     - Message size in bytes (default: 1024)"
	@echo "  QUEUE_SIZE       - Consumer receiver queue size (default: 1000)"
	@echo "  BATCH_SIZE       - Batch size in MB (default: 10)"
	@echo "  MAX_BATCHES      - Max batches to process (default: 4)"
	@echo "  SCENARIO         - Test scenario name (default: default)"
	@echo "  COMPRESSION      - Compression type: none, lz4, zlib, zstd (default: none)"
	@echo "  STRESS_DURATION  - Stress test duration in seconds (default: 120)"
	@echo "  PPROF_PORT       - pprof HTTP server port (default: 6060)"
	@echo ""
	@echo "Examples:"
	@echo "  make test                              # Run full memory comparison test"
	@echo "  make test-memory-stress STRESS_DURATION=600  # 10-minute stress test"
	@echo "  make test QUEUE_SIZE=100 SCENARIO=small_queue"

all: build

build:
	@echo "Building..."
	@mkdir -p bin
	go build -o bin/producer ./cmd/producer
	go build -o bin/consumer ./cmd/consumer
	@echo "Build complete: bin/producer, bin/consumer"

clean:
	rm -rf bin/
	rm -rf results/*.json results/*.pprof results/*.svg results/*.txt

start-pulsar:
	./scripts/start-pulsar.sh

stop-pulsar:
	./scripts/stop-pulsar.sh

deps:
	go mod tidy
	go mod download

produce: build
	./bin/producer \
		-total=$$(($(TOTAL_SIZE) * 1024 * 1024)) \
		-size=$(MESSAGE_SIZE) \
		-compression=$(COMPRESSION)

consume: build
	@mkdir -p results
	./bin/consumer \
		-batch-size=$$(($(BATCH_SIZE) * 1024 * 1024)) \
		-queue-size=$(QUEUE_SIZE) \
		-max-batches=$(MAX_BATCHES) \
		-scenario=$(SCENARIO) \
		-output=./results

test: build clean-results test-memory-compare
	@echo ""
	@echo "=========================================="
	@echo "Memory test completed!"
	@echo "Results saved in ./results/"
	@echo "=========================================="

# 清理测试结果
clean-results:
	@echo "Cleaning previous test results..."
	@rm -rf results/*.json results/*.pprof results/*.svg results/*.txt results/external_rss_*.txt
	@mkdir -p results

test-all: build
	./scripts/run-all-scenarios.sh

analyze:
	python3 ./scripts/analyze_results.py

# ============================================================
# 内存测试目标
# ============================================================

# 快速内存测试 (单次运行)
test-memory: build
	@echo "Running quick memory test..."
	@mkdir -p results
	@TOPIC="persistent://public/default/memory-test-$$(date +%s)"; \
	SUB="test-sub-$$(date +%s)"; \
	echo "Creating test topic: $$TOPIC"; \
	./bin/producer -topic=$$TOPIC -total=$$(($(TOTAL_SIZE) * 1024 * 1024)) -size=$(MESSAGE_SIZE); \
	echo "Running consumer..."; \
	./bin/consumer \
		-topic=$$TOPIC \
		-sub=$$SUB \
		-batch-size=$$(($(BATCH_SIZE) * 1024 * 1024)) \
		-queue-size=$(QUEUE_SIZE) \
		-max-batches=$(MAX_BATCHES) \
		-scenario=$(SCENARIO) \
		-pprof-port=$(PPROF_PORT) \
		-output=./results

# 内存对比测试 (with/without ReleasePayload)
PRODUCER_PPROF_PORT ?= 6070
test-memory-compare: build
	@echo "============================================================"
	@echo "Memory Comparison Test: ReleasePayload ON vs OFF"
	@echo "============================================================"
	@echo ""
	@echo "pprof endpoints (available during test):"
	@echo "  Producer:                http://localhost:$(PRODUCER_PPROF_PORT)/debug/pprof/"
	@echo "  Consumer (no-release):   http://localhost:$(PPROF_PORT)/debug/pprof/"
	@echo "  Consumer (with-release): http://localhost:$$(($(PPROF_PORT) + 1))/debug/pprof/"
	@echo ""
	@mkdir -p results
	@TOPIC="persistent://public/default/memory-compare-$$(date +%s)"; \
	echo ""; \
	echo "[Step 1/3] Producing $(STRESS_TOTAL_SIZE) MB test data..."; \
	echo "  Producer pprof: http://localhost:$(PRODUCER_PPROF_PORT)/debug/pprof/"; \
	./bin/producer -topic=$$TOPIC -total=$$(($(STRESS_TOTAL_SIZE) * 1024 * 1024)) -size=$(MESSAGE_SIZE) -pprof-port=$(PRODUCER_PPROF_PORT); \
	echo ""; \
	echo "[Step 2/3] Test 1: WITHOUT ReleasePayload"; \
	echo "------------------------------------------------------------"; \
	echo "  Consumer pprof: http://localhost:$(PPROF_PORT)/debug/pprof/"; \
	SUB1="no-release-$$(date +%s)"; \
	./scripts/monitor-rss.sh "bin/consumer" results/external_rss_no-release.txt 1 & \
	MONITOR_PID1=$$!; \
	./bin/consumer \
		-topic=$$TOPIC \
		-sub=$$SUB1 \
		-batch-size=$$(($(BATCH_SIZE) * 1024 * 1024)) \
		-queue-size=$(QUEUE_SIZE) \
		-max-batches=$(STRESS_MAX_BATCHES) \
		-scenario=no-release \
		-pprof-port=$(PPROF_PORT) \
		-output=./results; \
	kill $$MONITOR_PID1 2>/dev/null || true; \
	echo ""; \
	echo "[Step 3/3] Test 2: WITH ReleasePayload"; \
	echo "------------------------------------------------------------"; \
	echo "  Consumer pprof: http://localhost:$$(($(PPROF_PORT) + 1))/debug/pprof/"; \
	SUB2="with-release-$$(date +%s)"; \
	./scripts/monitor-rss.sh "bin/consumer" results/external_rss_with-release.txt 1 & \
	MONITOR_PID2=$$!; \
	./bin/consumer \
		-topic=$$TOPIC \
		-sub=$$SUB2 \
		-batch-size=$$(($(BATCH_SIZE) * 1024 * 1024)) \
		-queue-size=$(QUEUE_SIZE) \
		-max-batches=$(STRESS_MAX_BATCHES) \
		-scenario=with-release \
		-release-payload \
		-pprof-port=$$(($(PPROF_PORT) + 1)) \
		-output=./results; \
	kill $$MONITOR_PID2 2>/dev/null || true; \
	echo ""; \
	echo "[Step 4/4] Generating flame graphs..."; \
	echo "------------------------------------------------------------"; \
	if [ -f results/heap_no-release.pprof ]; then \
		go tool pprof -svg results/heap_no-release.pprof > results/flamegraph_no-release.svg 2>/dev/null && \
		echo "  Generated: results/flamegraph_no-release.svg" || \
		echo "  Warning: Failed to generate no-release flamegraph (graphviz may not be installed)"; \
	fi; \
	if [ -f results/heap_with-release.pprof ]; then \
		go tool pprof -svg results/heap_with-release.pprof > results/flamegraph_with-release.svg 2>/dev/null && \
		echo "  Generated: results/flamegraph_with-release.svg" || \
		echo "  Warning: Failed to generate with-release flamegraph (graphviz may not be installed)"; \
	fi; \
	python3 ./scripts/compare-results.py ./results; \
	echo ""; \
	echo "Output Files:"; \
	echo "  Heap profiles:  results/heap_*.pprof"; \
	echo "  Flame graphs:   results/flamegraph_*.svg"; \
	echo "  Stats (JSON):   results/stats_*.json"; \
	echo ""; \
	echo "Commands:"; \
	echo "  open results/flamegraph_no-release.svg"; \
	echo "  open results/flamegraph_with-release.svg"; \
	echo "  go tool pprof -http=:8080 results/heap_no-release.pprof"; \
	echo "  go tool pprof -http=:8081 results/heap_with-release.pprof"

# 长时间压力测试 (带 pprof 收集)
test-memory-stress: build
	@echo "============================================================"
	@echo "Long-duration Stress Test with pprof Collection"
	@echo "Duration: $(STRESS_DURATION) seconds"
	@echo "============================================================"
	@mkdir -p results
	@TOPIC="persistent://public/default/stress-test-$$(date +%s)"; \
	TOTAL_BYTES=$$(($(STRESS_TOTAL_SIZE) * 1024 * 1024 * 5)); \
	echo ""; \
	echo "[Step 1] Producing large test dataset..."; \
	./bin/producer -topic=$$TOPIC -total=$$TOTAL_BYTES -size=$(MESSAGE_SIZE); \
	echo ""; \
	echo "[Step 2] Starting stress test ($(STRESS_DURATION)s)..."; \
	echo "pprof available at: http://localhost:$(PPROF_PORT)/debug/pprof/"; \
	echo ""; \
	SUB="stress-$$(date +%s)"; \
	./bin/consumer \
		-topic=$$TOPIC \
		-sub=$$SUB \
		-batch-size=$$(($(BATCH_SIZE) * 1024 * 1024)) \
		-queue-size=$(QUEUE_SIZE) \
		-max-batches=0 \
		-scenario=stress-test \
		-release-payload \
		-pprof-port=$(PPROF_PORT) \
		-output=./results & \
	CONSUMER_PID=$$!; \
	echo "Consumer PID: $$CONSUMER_PID"; \
	echo ""; \
	echo "Collecting heap profiles every 25 seconds..."; \
	for i in 1 2 3 4 5; do \
		sleep 25; \
		echo "Checkpoint $$i ($$(( $$i * 25 ))s): Collecting heap profile..."; \
		curl -s "http://localhost:$(PPROF_PORT)/debug/pprof/heap" > "results/heap_stress_checkpoint_$$i.pprof" 2>/dev/null || true; \
		curl -s "http://localhost:$(PPROF_PORT)/debug/pprof/heap?debug=1" 2>/dev/null | head -20 || true; \
	done; \
	echo ""; \
	echo "Waiting for consumer to complete or timeout..."; \
	sleep 5; \
	kill $$CONSUMER_PID 2>/dev/null || true; \
	echo ""; \
	echo "Stress test completed."; \
	echo "Heap profiles saved to results/heap_stress_checkpoint_*.pprof"

# 收集 pprof 数据 (用于正在运行的进程)
test-pprof-collect:
	@echo "Collecting pprof data from http://localhost:$(PPROF_PORT)/debug/pprof/"
	@mkdir -p results
	@echo "Heap profile..."
	@curl -s "http://localhost:$(PPROF_PORT)/debug/pprof/heap" > results/heap_live.pprof
	@echo "Heap profile (text)..."
	@curl -s "http://localhost:$(PPROF_PORT)/debug/pprof/heap?debug=1" > results/heap_live.txt
	@echo "Goroutine profile..."
	@curl -s "http://localhost:$(PPROF_PORT)/debug/pprof/goroutine" > results/goroutine_live.pprof
	@echo "Allocs profile..."
	@curl -s "http://localhost:$(PPROF_PORT)/debug/pprof/allocs" > results/allocs_live.pprof
	@echo ""
	@echo "Profiles saved to:"
	@echo "  - results/heap_live.pprof"
	@echo "  - results/heap_live.txt"
	@echo "  - results/goroutine_live.pprof"
	@echo "  - results/allocs_live.pprof"
	@echo ""
	@echo "To view: go tool pprof -http=:8080 results/heap_live.pprof"

# ============================================================
# 便捷测试目标
# ============================================================
test-queue-1000: build
	SCENARIO=queue_1000 QUEUE_SIZE=1000 ./scripts/run-test.sh

test-queue-100: build
	SCENARIO=queue_100 QUEUE_SIZE=100 ./scripts/run-test.sh

test-queue-10: build
	SCENARIO=queue_10 QUEUE_SIZE=10 ./scripts/run-test.sh

test-large-msg: build
	SCENARIO=large_msg QUEUE_SIZE=100 MESSAGE_SIZE=102400 ./scripts/run-test.sh

test-lz4: build
	SCENARIO=lz4 QUEUE_SIZE=100 COMPRESSION=lz4 ./scripts/run-test.sh

# ============================================================
# pprof 分析
# ============================================================
pprof-heap:
	@if [ -f results/heap_$(SCENARIO).pprof ]; then \
		go tool pprof -http=:8080 results/heap_$(SCENARIO).pprof; \
	else \
		echo "No heap profile found for scenario: $(SCENARIO)"; \
		echo "Available profiles:"; \
		ls results/*.pprof 2>/dev/null || echo "  (none)"; \
	fi

pprof-compare:
	@echo "Opening pprof comparison in browser..."
	@echo "Left: without ReleasePayload, Right: with ReleasePayload"
	@if [ -f results/heap_no-release.pprof ] && [ -f results/heap_with-release.pprof ]; then \
		go tool pprof -http=:8080 -diff_base=results/heap_no-release.pprof results/heap_with-release.pprof; \
	else \
		echo "Missing profile files. Run 'make test-memory-compare' first."; \
	fi

pprof-flamegraph:
	@echo "Generating flame graph..."
	@if [ -f results/heap_$(SCENARIO).pprof ]; then \
		go tool pprof -http=:8080 results/heap_$(SCENARIO).pprof; \
	else \
		echo "No heap profile found. Run 'make test' first."; \
	fi

# 生成所有火焰图 SVG
generate-flamegraphs:
	@echo "Generating flame graph SVGs..."
	@mkdir -p results
	@for pprof in results/*.pprof; do \
		if [ -f "$$pprof" ]; then \
			svg="$${pprof%.pprof}.svg"; \
			echo "  $$pprof -> $$svg"; \
			go tool pprof -svg "$$pprof" > "$$svg" 2>/dev/null || \
			echo "    Warning: Failed (graphviz may not be installed)"; \
		fi; \
	done
	@echo ""
	@echo "To view SVGs:"
	@echo "  open results/*.svg"

# 打开火焰图 (macOS)
open-flamegraphs:
	@if ls results/*.svg 1>/dev/null 2>&1; then \
		open results/*.svg; \
	else \
		echo "No SVG files found. Run 'make test' or 'make generate-flamegraphs' first."; \
	fi
