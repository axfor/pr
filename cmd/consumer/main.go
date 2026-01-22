package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"pulsar-memory-test/pkg/metrics"
)

var (
	pulsarURL         = flag.String("url", "pulsar://localhost:6650", "Pulsar broker URL")
	topic             = flag.String("topic", "persistent://public/default/memory-test", "Topic name")
	subscription      = flag.String("sub", "memory-test-sub", "Subscription name")
	batchSize         = flag.Int64("batch-size", 50*1024*1024, "Batch size in bytes before processing")
	receiverQueueSize = flag.Int("queue-size", 1000, "Consumer receiver queue size")
	memoryLimit       = flag.Int64("memory-limit", 0, "Client memory limit in bytes (0 = no limit)")
	gcPercent         = flag.Int("gc-percent", 100, "GOGC value")
	pprofPort         = flag.Int("pprof-port", 6060, "pprof HTTP server port")
	outputDir         = flag.String("output", "./results", "Output directory for results")
	processDelay      = flag.Duration("process-delay", 0, "Simulated processing delay per batch")
	maxBatches        = flag.Int("max-batches", 0, "Maximum number of batches to process (0 = unlimited)")
	scenario          = flag.String("scenario", "default", "Test scenario name for output files")
	releasePayload    = flag.Bool("release-payload", false, "Release payload after business processing to save memory")
)

// BatchProcessor 模拟批量处理
type BatchProcessor struct {
	messages       []pulsar.Message
	currentBytes   int64
	batchSize      int64
	batchCount     int
	processDelay   time.Duration
	consumer       pulsar.Consumer
	monitor        *metrics.MemoryMonitor
	releasePayload bool
}

func NewBatchProcessor(batchSize int64, processDelay time.Duration, consumer pulsar.Consumer, monitor *metrics.MemoryMonitor, releasePayload bool) *BatchProcessor {
	return &BatchProcessor{
		messages:       make([]pulsar.Message, 0, 10000),
		batchSize:      batchSize,
		processDelay:   processDelay,
		consumer:       consumer,
		monitor:        monitor,
		releasePayload: releasePayload,
	}
}

func (bp *BatchProcessor) Add(msg pulsar.Message) (shouldProcess bool) {
	msgSize := int64(len(msg.Payload()))

	// 模拟业务处理：读取 payload 数据
	// 实际业务中这里会解析消息内容进行处理
	_ = msg.Payload()

	// 如果启用了 releasePayload，处理完后立即释放 payload 内存
	// 只保留 MessageID 用于后续 ACK
	if bp.releasePayload {
		msg.ReleasePayload()
	}

	bp.messages = append(bp.messages, msg)
	bp.currentBytes += msgSize
	bp.monitor.RecordMessage(msgSize)

	return bp.currentBytes >= bp.batchSize
}

func (bp *BatchProcessor) Process(ctx context.Context) error {
	if len(bp.messages) == 0 {
		return nil
	}

	bp.batchCount++
	log.Printf("Processing batch #%d: %d messages, %.2f MB",
		bp.batchCount, len(bp.messages), float64(bp.currentBytes)/1024/1024)

	// 记录处理前的内存状态
	beforeStats := bp.monitor.Collect()
	log.Printf("  Before processing - HeapAlloc: %.2f MB, RSS: %.2f MB",
		float64(beforeStats.HeapAlloc)/1024/1024, float64(beforeStats.RSS)/1024/1024)

	// 模拟业务处理
	if bp.processDelay > 0 {
		time.Sleep(bp.processDelay)
	}

	// 逐个确认消息
	for _, msg := range bp.messages {
		bp.consumer.Ack(msg)
	}

	bp.monitor.RecordBatch()

	// 清空批次
	bp.messages = bp.messages[:0]
	bp.currentBytes = 0

	// 处理完成后强制 GC，观察内存释放情况
	runtime.GC()

	afterStats := bp.monitor.Collect()
	log.Printf("  After processing+GC - HeapAlloc: %.2f MB, RSS: %.2f MB",
		float64(afterStats.HeapAlloc)/1024/1024, float64(afterStats.RSS)/1024/1024)

	return nil
}

const logPrefix = "[CONSUMER] "

func main() {
	flag.Parse()

	// 设置日志前缀
	log.SetPrefix(logPrefix)

	// 设置 GOGC
	oldGC := debug.SetGCPercent(*gcPercent)
	log.Printf("GOGC: %d -> %d", oldGC, *gcPercent)

	// 启动 pprof 服务
	go func() {
		addr := fmt.Sprintf("localhost:%d", *pprofPort)
		log.Printf("Starting pprof server at http://%s/debug/pprof/", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Printf("pprof server error: %v", err)
		}
	}()

	// 创建输出目录
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	log.Println("========== Consumer Config ==========")
	log.Printf("  URL: %s", *pulsarURL)
	log.Printf("  Topic: %s", *topic)
	log.Printf("  Subscription: %s", *subscription)
	log.Printf("  Batch size: %.2f MB", float64(*batchSize)/1024/1024)
	log.Printf("  ReceiverQueueSize: %d", *receiverQueueSize)
	log.Printf("  Memory limit: %d bytes", *memoryLimit)
	log.Printf("  GOGC: %d", *gcPercent)
	log.Printf("  Max batches: %d (0=unlimited)", *maxBatches)
	log.Printf("  Scenario: %s", *scenario)
	log.Printf("  Release payload: %v", *releasePayload)
	log.Println("======================================")

	// 创建内存监控器
	monitor, err := metrics.NewMemoryMonitor()
	if err != nil {
		log.Fatalf("Failed to create memory monitor: %v", err)
	}

	// 开始内存采集 (每秒一次)
	monitor.Start(time.Second)

	// 记录初始内存状态
	initialStats := monitor.Collect()
	log.Printf("Initial memory - HeapAlloc: %.2f MB, RSS: %.2f MB",
		float64(initialStats.HeapAlloc)/1024/1024, float64(initialStats.RSS)/1024/1024)

	// 创建 Pulsar 客户端
	clientOptions := pulsar.ClientOptions{
		URL:               *pulsarURL,
		OperationTimeout:  30 * time.Second,
		ConnectionTimeout: 30 * time.Second,
	}
	if *memoryLimit > 0 {
		clientOptions.MemoryLimitBytes = *memoryLimit
	}

	client, err := pulsar.NewClient(clientOptions)
	if err != nil {
		log.Fatalf("Failed to create Pulsar client: %v", err)
	}
	defer client.Close()

	// 记录客户端创建后的内存
	postClientStats := monitor.Collect()
	log.Printf("After client creation - HeapAlloc: %.2f MB, RSS: %.2f MB (delta: +%.2f MB)",
		float64(postClientStats.HeapAlloc)/1024/1024,
		float64(postClientStats.RSS)/1024/1024,
		float64(postClientStats.HeapAlloc-initialStats.HeapAlloc)/1024/1024)

	// 创建消费者
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:                       *topic,
		SubscriptionName:            *subscription,
		Type:                        pulsar.Shared,
		SubscriptionInitialPosition: pulsar.SubscriptionPositionEarliest,
		ReceiverQueueSize:           *receiverQueueSize,
		EnableBatchIndexAcknowledgment: true,
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}
	defer consumer.Close()

	// 记录消费者创建后的内存
	postConsumerStats := monitor.Collect()
	log.Printf("After consumer creation - HeapAlloc: %.2f MB, RSS: %.2f MB (delta: +%.2f MB)",
		float64(postConsumerStats.HeapAlloc)/1024/1024,
		float64(postConsumerStats.RSS)/1024/1024,
		float64(postConsumerStats.HeapAlloc-postClientStats.HeapAlloc)/1024/1024)

	// 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 创建批处理器
	batchProcessor := NewBatchProcessor(*batchSize, *processDelay, consumer, monitor, *releasePayload)

	// 消费消息
	log.Println("Starting to consume messages...")
	startTime := time.Now()

	// 进度报告
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				msgCount, msgBytes, batchCount := monitor.GetCurrentStats()
				currentStats := monitor.Collect()
				log.Printf("Progress: %d messages (%.2f MB), %d batches | Heap: %.2f MB | RSS: %.2f MB | Ratio: %.2fx",
					msgCount,
					float64(msgBytes)/1024/1024,
					batchCount,
					float64(currentStats.HeapAlloc)/1024/1024,
					float64(currentStats.RSS)/1024/1024,
					float64(currentStats.HeapAlloc)/float64(msgBytes+1))
			case <-ctx.Done():
				return
			}
		}
	}()

	// 主消费循环
consumeLoop:
	for {
		select {
		case <-sigCh:
			log.Println("Received signal, stopping...")
			cancel()
			break consumeLoop
		case <-ctx.Done():
			break consumeLoop
		default:
		}

		// 带超时的接收
		recvCtx, recvCancel := context.WithTimeout(ctx, 100*time.Millisecond)
		msg, err := consumer.Receive(recvCtx)
		recvCancel()

		if err != nil {
			if ctx.Err() != nil {
				break consumeLoop
			}
			// 超时，检查是否还有更多消息
			if batchProcessor.currentBytes > 0 && batchProcessor.batchCount > 0 {
				// 没有更多消息且已经有数据，处理最后一批
				log.Println("No more messages, processing remaining batch...")
				batchProcessor.Process(ctx)
				break consumeLoop
			}
			continue
		}

		// 添加到批次
		if batchProcessor.Add(msg) {
			batchProcessor.Process(ctx)

			// 检查是否达到最大批次数
			if *maxBatches > 0 && batchProcessor.batchCount >= *maxBatches {
				log.Printf("Reached max batches (%d), stopping...", *maxBatches)
				break consumeLoop
			}
		}
	}

	// 处理剩余消息
	if batchProcessor.currentBytes > 0 {
		batchProcessor.Process(ctx)
	}

	elapsed := time.Since(startTime)
	monitor.Stop()

	// 写入堆 profile
	heapProfilePath := filepath.Join(*outputDir, fmt.Sprintf("heap_%s.pprof", *scenario))
	if err := metrics.WriteHeapProfile(heapProfilePath); err != nil {
		log.Printf("Failed to write heap profile: %v", err)
	} else {
		log.Printf("Heap profile saved to: %s", heapProfilePath)
	}

	// 保存统计数据
	statsPath := filepath.Join(*outputDir, fmt.Sprintf("stats_%s.json", *scenario))
	if err := monitor.SaveToFile(statsPath); err != nil {
		log.Printf("Failed to save stats: %v", err)
	} else {
		log.Printf("Stats saved to: %s", statsPath)
	}

	// 打印摘要
	monitor.PrintSummary()

	log.Println("")
	log.Printf("Duration: %v", elapsed.Round(time.Millisecond))
	log.Printf("pprof command: go tool pprof -http=:8080 %s", heapProfilePath)
}
