package metrics

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// MemoryStats 内存统计数据
type MemoryStats struct {
	Timestamp   time.Time `json:"timestamp"`

	// Go runtime 内存统计
	HeapAlloc    uint64 `json:"heap_alloc"`     // 堆上已分配的字节数
	HeapSys      uint64 `json:"heap_sys"`       // 从OS获取的堆内存
	HeapInuse    uint64 `json:"heap_inuse"`     // 正在使用的堆内存
	HeapIdle     uint64 `json:"heap_idle"`      // 空闲的堆内存
	HeapReleased uint64 `json:"heap_released"`  // 释放回OS的内存
	HeapObjects  uint64 `json:"heap_objects"`   // 堆上对象数量

	StackInuse   uint64 `json:"stack_inuse"`    // 栈使用内存
	StackSys     uint64 `json:"stack_sys"`      // 栈系统内存

	MSpanInuse   uint64 `json:"mspan_inuse"`
	MCacheInuse  uint64 `json:"mcache_inuse"`

	Sys          uint64 `json:"sys"`            // 从OS获取的总内存
	TotalAlloc   uint64 `json:"total_alloc"`    // 累计分配的字节数

	NumGC        uint32 `json:"num_gc"`         // GC次数
	PauseTotalNs uint64 `json:"pause_total_ns"` // GC总暂停时间

	// 进程级内存统计
	RSS          uint64 `json:"rss"`            // 驻留内存
	VMS          uint64 `json:"vms"`            // 虚拟内存

	// 业务统计
	MessageCount    int64  `json:"message_count"`    // 已处理消息数
	MessageBytes    int64  `json:"message_bytes"`    // 已处理消息字节数
	BatchCount      int64  `json:"batch_count"`      // 批次数
}

// MemoryMonitor 内存监控器
type MemoryMonitor struct {
	mu            sync.RWMutex
	stats         []MemoryStats
	messageCount  int64
	messageBytes  int64
	batchCount    int64
	startTime     time.Time
	pid           int32
	proc          *process.Process
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewMemoryMonitor 创建内存监控器
func NewMemoryMonitor() (*MemoryMonitor, error) {
	pid := int32(os.Getpid())
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	return &MemoryMonitor{
		stats:     make([]MemoryStats, 0, 1000),
		startTime: time.Now(),
		pid:       pid,
		proc:      proc,
		stopCh:    make(chan struct{}),
	}, nil
}

// Start 开始定期采集内存数据
func (m *MemoryMonitor) Start(interval time.Duration) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// 立即采集一次
		m.Collect()

		for {
			select {
			case <-ticker.C:
				m.Collect()
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop 停止采集
func (m *MemoryMonitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// Collect 采集一次内存数据
func (m *MemoryMonitor) Collect() MemoryStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var rss, vms uint64
	if memInfo, err := m.proc.MemoryInfo(); err == nil {
		rss = memInfo.RSS
		vms = memInfo.VMS
	}

	m.mu.RLock()
	msgCount := m.messageCount
	msgBytes := m.messageBytes
	batchCount := m.batchCount
	m.mu.RUnlock()

	stats := MemoryStats{
		Timestamp:    time.Now(),
		HeapAlloc:    ms.HeapAlloc,
		HeapSys:      ms.HeapSys,
		HeapInuse:    ms.HeapInuse,
		HeapIdle:     ms.HeapIdle,
		HeapReleased: ms.HeapReleased,
		HeapObjects:  ms.HeapObjects,
		StackInuse:   ms.StackInuse,
		StackSys:     ms.StackSys,
		MSpanInuse:   ms.MSpanInuse,
		MCacheInuse:  ms.MCacheInuse,
		Sys:          ms.Sys,
		TotalAlloc:   ms.TotalAlloc,
		NumGC:        ms.NumGC,
		PauseTotalNs: ms.PauseTotalNs,
		RSS:          rss,
		VMS:          vms,
		MessageCount: msgCount,
		MessageBytes: msgBytes,
		BatchCount:   batchCount,
	}

	m.mu.Lock()
	m.stats = append(m.stats, stats)
	m.mu.Unlock()

	return stats
}

// RecordMessage 记录消息处理
func (m *MemoryMonitor) RecordMessage(bytes int64) {
	m.mu.Lock()
	m.messageCount++
	m.messageBytes += bytes
	m.mu.Unlock()
}

// RecordBatch 记录批次完成
func (m *MemoryMonitor) RecordBatch() {
	m.mu.Lock()
	m.batchCount++
	m.mu.Unlock()
}

// GetStats 获取所有统计数据
func (m *MemoryMonitor) GetStats() []MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MemoryStats, len(m.stats))
	copy(result, m.stats)
	return result
}

// GetCurrentStats 获取当前统计
func (m *MemoryMonitor) GetCurrentStats() (msgCount, msgBytes, batchCount int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.messageCount, m.messageBytes, m.batchCount
}

// MemorySummary 内存统计摘要
type MemorySummary struct {
	Duration     time.Duration `json:"duration"`
	MessageCount int64         `json:"message_count"`
	MessageBytes int64         `json:"message_bytes"`
	BatchCount   int64         `json:"batch_count"`
	SampleCount  int           `json:"sample_count"`

	// HeapAlloc 统计 (字节)
	MinHeapAlloc uint64  `json:"min_heap_alloc"`
	MaxHeapAlloc uint64  `json:"max_heap_alloc"`
	AvgHeapAlloc float64 `json:"avg_heap_alloc"`
	FinalHeapAlloc uint64 `json:"final_heap_alloc"`

	// RSS 统计 (字节)
	MinRSS uint64  `json:"min_rss"`
	MaxRSS uint64  `json:"max_rss"`
	AvgRSS float64 `json:"avg_rss"`
	FinalRSS uint64 `json:"final_rss"`

	// HeapInuse 统计 (字节)
	MinHeapInuse uint64  `json:"min_heap_inuse"`
	MaxHeapInuse uint64  `json:"max_heap_inuse"`
	AvgHeapInuse float64 `json:"avg_heap_inuse"`

	// GC 统计
	NumGC        uint32 `json:"num_gc"`
	PauseTotalMs float64 `json:"pause_total_ms"`

	// 内存放大倍数
	HeapRatio float64 `json:"heap_ratio"` // MaxHeapAlloc / MessageBytes
	RSSRatio  float64 `json:"rss_ratio"`  // MaxRSS / MessageBytes
}

// GetSummary 计算内存统计摘要
func (m *MemoryMonitor) GetSummary() MemorySummary {
	stats := m.GetStats()
	summary := MemorySummary{
		Duration:    time.Since(m.startTime),
		SampleCount: len(stats),
	}

	if len(stats) == 0 {
		return summary
	}

	// 初始化最小值为第一个样本
	first := stats[0]
	summary.MinHeapAlloc = first.HeapAlloc
	summary.MinRSS = first.RSS
	summary.MinHeapInuse = first.HeapInuse

	// 计算总和用于平均值
	var totalHeap, totalRSS, totalHeapInuse uint64

	for _, s := range stats {
		// HeapAlloc
		if s.HeapAlloc < summary.MinHeapAlloc {
			summary.MinHeapAlloc = s.HeapAlloc
		}
		if s.HeapAlloc > summary.MaxHeapAlloc {
			summary.MaxHeapAlloc = s.HeapAlloc
		}
		totalHeap += s.HeapAlloc

		// RSS
		if s.RSS < summary.MinRSS && s.RSS > 0 {
			summary.MinRSS = s.RSS
		}
		if s.RSS > summary.MaxRSS {
			summary.MaxRSS = s.RSS
		}
		totalRSS += s.RSS

		// HeapInuse
		if s.HeapInuse < summary.MinHeapInuse {
			summary.MinHeapInuse = s.HeapInuse
		}
		if s.HeapInuse > summary.MaxHeapInuse {
			summary.MaxHeapInuse = s.HeapInuse
		}
		totalHeapInuse += s.HeapInuse
	}

	// 计算平均值
	n := uint64(len(stats))
	summary.AvgHeapAlloc = float64(totalHeap) / float64(n)
	summary.AvgRSS = float64(totalRSS) / float64(n)
	summary.AvgHeapInuse = float64(totalHeapInuse) / float64(n)

	// 最后一个样本的数据
	last := stats[len(stats)-1]
	summary.MessageCount = last.MessageCount
	summary.MessageBytes = last.MessageBytes
	summary.BatchCount = last.BatchCount
	summary.FinalHeapAlloc = last.HeapAlloc
	summary.FinalRSS = last.RSS
	summary.NumGC = last.NumGC
	summary.PauseTotalMs = float64(last.PauseTotalNs) / 1e6

	// 计算内存放大倍数
	if last.MessageBytes > 0 {
		summary.HeapRatio = float64(summary.MaxHeapAlloc) / float64(last.MessageBytes)
		summary.RSSRatio = float64(summary.MaxRSS) / float64(last.MessageBytes)
	}

	return summary
}

// PrintSummary 打印摘要信息
func (m *MemoryMonitor) PrintSummary() {
	summary := m.GetSummary()
	if summary.SampleCount == 0 {
		log.Println("No stats collected")
		return
	}

	log.Println("")
	log.Println("========== Memory Summary ==========")
	log.Printf("  Duration:      %v", summary.Duration.Round(time.Second))
	log.Printf("  Samples:       %d", summary.SampleCount)
	log.Printf("  Messages:      %d", summary.MessageCount)
	log.Printf("  Data size:     %.2f MB", float64(summary.MessageBytes)/1024/1024)
	log.Printf("  Batches:       %d", summary.BatchCount)
	log.Println("")
	log.Println("  --- HeapAlloc (MB) ---")
	log.Printf("    Min: %.2f | Max: %.2f | Avg: %.2f | Final: %.2f",
		float64(summary.MinHeapAlloc)/1024/1024,
		float64(summary.MaxHeapAlloc)/1024/1024,
		summary.AvgHeapAlloc/1024/1024,
		float64(summary.FinalHeapAlloc)/1024/1024)
	log.Println("")
	log.Println("  --- RSS (MB) ---")
	log.Printf("    Min: %.2f | Max: %.2f | Avg: %.2f | Final: %.2f",
		float64(summary.MinRSS)/1024/1024,
		float64(summary.MaxRSS)/1024/1024,
		summary.AvgRSS/1024/1024,
		float64(summary.FinalRSS)/1024/1024)
	log.Println("")
	log.Println("  --- HeapInuse (MB) ---")
	log.Printf("    Min: %.2f | Max: %.2f | Avg: %.2f",
		float64(summary.MinHeapInuse)/1024/1024,
		float64(summary.MaxHeapInuse)/1024/1024,
		summary.AvgHeapInuse/1024/1024)
	log.Println("")
	log.Printf("  --- GC ---")
	log.Printf("    Count: %d | Total pause: %.2f ms", summary.NumGC, summary.PauseTotalMs)

	// 计算内存放大倍数
	if summary.MessageBytes > 0 {
		log.Println("")
		log.Println("  --- Memory Amplification ---")
		log.Printf("    MaxHeapAlloc/DataSize: %.2fx", summary.HeapRatio)
		log.Printf("    MaxRSS/DataSize:       %.2fx", summary.RSSRatio)
	}
	log.Println("====================================")
}

// StatsOutput 保存到文件的输出格式
type StatsOutput struct {
	Summary MemorySummary `json:"summary"`
	Samples []MemoryStats `json:"samples,omitempty"`
}

// SaveToFile 保存统计数据到文件
func (m *MemoryMonitor) SaveToFile(filename string) error {
	output := StatsOutput{
		Summary: m.GetSummary(),
		Samples: m.GetStats(),
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// SaveSummaryToFile 仅保存摘要到文件
func (m *MemoryMonitor) SaveSummaryToFile(filename string) error {
	summary := m.GetSummary()

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

// WriteHeapProfile 写入堆内存 profile
func WriteHeapProfile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	runtime.GC() // 先触发 GC 获取更准确的数据
	return pprof.WriteHeapProfile(f)
}

// ForceGC 强制执行 GC
func ForceGC() {
	runtime.GC()
}

// FormatBytes 格式化字节数
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
