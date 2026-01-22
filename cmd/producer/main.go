package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
)

var (
	pulsarURL    = flag.String("url", "pulsar://localhost:6650", "Pulsar broker URL")
	topic        = flag.String("topic", "persistent://public/default/memory-test", "Topic name")
	messageSize  = flag.Int("size", 1024, "Message size in bytes")
	totalSize    = flag.Int64("total", 200*1024*1024, "Total data size to produce in bytes")
	concurrency  = flag.Int("concurrency", 10, "Number of concurrent producers")
	batchingTime = flag.Duration("batching-time", 10*time.Millisecond, "Batching max publish delay")
	compression  = flag.String("compression", "none", "Compression type: none, lz4, zlib, zstd")
	pprofPort    = flag.Int("pprof-port", 6070, "pprof HTTP server port")
)

const logPrefix = "[PRODUCER] "

func main() {
	flag.Parse()

	// 设置日志前缀
	log.SetPrefix(logPrefix)

	// 启动 pprof 服务
	go func() {
		addr := fmt.Sprintf("localhost:%d", *pprofPort)
		log.Printf("Starting pprof server at http://%s/debug/pprof/", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Printf("pprof server error: %v", err)
		}
	}()

	log.Println("========== Producer Config ==========")
	log.Printf("  URL: %s", *pulsarURL)
	log.Printf("  Topic: %s", *topic)
	log.Printf("  Message size: %d bytes", *messageSize)
	log.Printf("  Total size: %.2f MB", float64(*totalSize)/1024/1024)
	log.Printf("  Concurrency: %d", *concurrency)
	log.Printf("  Compression: %s", *compression)
	log.Printf("  pprof: http://localhost:%d/debug/pprof/", *pprofPort)
	log.Println("======================================")

	// 创建客户端
	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL:               *pulsarURL,
		OperationTimeout:  30 * time.Second,
		ConnectionTimeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// 确定压缩类型
	var compressionType pulsar.CompressionType
	switch *compression {
	case "lz4":
		compressionType = pulsar.LZ4
	case "zlib":
		compressionType = pulsar.ZLib
	case "zstd":
		compressionType = pulsar.ZSTD
	default:
		compressionType = pulsar.NoCompression
	}

	// 创建 producer
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic:                   *topic,
		CompressionType:         compressionType,
		BatchingMaxPublishDelay: *batchingTime,
		BatchingMaxMessages:     1000,
	})
	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}
	defer producer.Close()

	// 生成消息模板
	messagePayload := make([]byte, *messageSize)
	rand.Read(messagePayload)

	// 处理信号
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received signal, stopping...")
		cancel()
	}()

	// 统计
	var sentBytes int64
	var sentCount int64
	var errorCount int64

	startTime := time.Now()

	// 进度报告
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sent := atomic.LoadInt64(&sentBytes)
				count := atomic.LoadInt64(&sentCount)
				errors := atomic.LoadInt64(&errorCount)
				progress := float64(sent) / float64(*totalSize) * 100
				rate := float64(sent) / time.Since(startTime).Seconds() / 1024 / 1024
				log.Printf("Progress: %.1f%% | Sent: %.2f MB | Messages: %d | Errors: %d | Rate: %.2f MB/s",
					progress, float64(sent)/1024/1024, count, errors, rate)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 并发发送
	var wg sync.WaitGroup
	messagesPerWorker := int(*totalSize) / *messageSize / *concurrency

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < messagesPerWorker; j++ {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// 每条消息稍微变化一下，避免压缩效果太好
				payload := make([]byte, *messageSize)
				copy(payload, messagePayload)
				payload[0] = byte(workerID)
				payload[1] = byte(j % 256)
				payload[2] = byte((j / 256) % 256)

				_, err := producer.Send(ctx, &pulsar.ProducerMessage{
					Payload: payload,
					Properties: map[string]string{
						"worker":    fmt.Sprintf("%d", workerID),
						"sequence":  fmt.Sprintf("%d", j),
						"timestamp": fmt.Sprintf("%d", time.Now().UnixNano()),
					},
				})

				if err != nil {
					if ctx.Err() != nil {
						return
					}
					atomic.AddInt64(&errorCount, 1)
					log.Printf("Worker %d: Send error: %v", workerID, err)
					continue
				}

				atomic.AddInt64(&sentBytes, int64(*messageSize))
				atomic.AddInt64(&sentCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 确保所有消息都发送完成
	producer.Flush()

	elapsed := time.Since(startTime)
	finalSent := atomic.LoadInt64(&sentBytes)
	finalCount := atomic.LoadInt64(&sentCount)
	finalErrors := atomic.LoadInt64(&errorCount)

	log.Println("")
	log.Println("========== Producer Summary ==========")
	log.Printf("  Duration:     %v", elapsed.Round(time.Millisecond))
	log.Printf("  Messages:     %d", finalCount)
	log.Printf("  Data size:    %.2f MB", float64(finalSent)/1024/1024)
	log.Printf("  Errors:       %d", finalErrors)
	log.Printf("  Throughput:   %.2f MB/s", float64(finalSent)/elapsed.Seconds()/1024/1024)
	log.Printf("  TPS:          %.0f msg/s", float64(finalCount)/elapsed.Seconds())
	log.Println("=======================================")
}
