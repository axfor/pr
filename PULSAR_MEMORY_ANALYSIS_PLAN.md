# Pulsar Client Go 内存优化分析方案

## 一、问题描述

**现象**: 使用 pulsar-client-go 实现批量消费（每批 50MB）时，发现客户端内存占用是业务数据量的 3-5 倍。

**目标**: 深度分析内存放大原因，并提供优化方案。

---

## 二、分析维度

### 2.1 内存放大的可能原因

1. **消息缓冲机制**
   - Consumer 预取缓冲区 (ReceiverQueueSize)
   - 内部 channel 缓冲
   - 批量消息解包后的内存复制

2. **消息结构开销**
   - `pulsar.Message` 接口实现的元数据
   - Properties map 分配
   - MessageID 对象
   - 消息头部信息

3. **序列化/反序列化**
   - Protocol Buffers 解码产生的临时对象
   - 字节切片的多次复制

4. **连接层开销**
   - TCP 缓冲区
   - TLS 缓冲区（如果启用）
   - 压缩/解压缩缓冲区

5. **Go Runtime 因素**
   - GC 延迟导致的内存堆积
   - 内存碎片化
   - goroutine 栈内存

---

## 三、测试方案

### 3.1 环境准备

```bash
# 1. 部署 Pulsar Standalone (Docker)
docker run -d --name pulsar-standalone \
  -p 6650:6650 \
  -p 8080:8080 \
  apache/pulsar:3.1.0 \
  bin/pulsar standalone

# 2. 创建测试 Topic
docker exec pulsar-standalone bin/pulsar-admin topics create persistent://public/default/memory-test
```

### 3.2 测试代码结构

```
pulsar-memory-test/
├── go.mod
├── cmd/
│   ├── producer/          # 生产测试数据
│   │   └── main.go
│   └── consumer/          # 消费并分析内存
│       └── main.go
├── pkg/
│   ├── metrics/           # 内存监控工具
│   │   └── memory.go
│   └── analysis/          # 分析报告生成
│       └── report.go
├── configs/
│   └── test_config.yaml   # 测试配置
└── results/               # 测试结果输出
    └── .gitkeep
```

### 3.3 测试场景矩阵

| 场景 | ReceiverQueueSize | 消息大小 | 批量大小 | 压缩 | 预期内存倍数 |
|------|-------------------|----------|----------|------|--------------|
| 基准 | 1000 (默认) | 1KB | 50MB | 无 | 测量 |
| 小队列 | 100 | 1KB | 50MB | 无 | 测量 |
| 最小队列 | 10 | 1KB | 50MB | 无 | 测量 |
| 大消息 | 100 | 100KB | 50MB | 无 | 测量 |
| 压缩启用 | 100 | 1KB | 50MB | LZ4 | 测量 |
| 压缩+大消息 | 100 | 100KB | 50MB | ZSTD | 测量 |

### 3.4 监控指标

1. **进程级内存**
   - RSS (Resident Set Size)
   - VMS (Virtual Memory Size)
   - HeapAlloc / HeapSys / HeapInuse

2. **对象级分析**
   - pprof heap profile
   - 对象分配热点
   - 内存逃逸分析

3. **时序数据**
   - 内存随消费进度的变化曲线
   - GC 暂停时间和频率

---

## 四、优化策略

### 4.1 客户端配置优化

```go
// 关键参数调优
client, _ := pulsar.NewClient(pulsar.ClientOptions{
    URL:                     "pulsar://localhost:6650",
    MemoryLimitBytes:        64 * 1024 * 1024,  // 限制客户端内存
    OperationTimeout:        30 * time.Second,
})

consumer, _ := client.Subscribe(pulsar.ConsumerOptions{
    Topic:             "memory-test",
    SubscriptionName:  "sub-1",
    Type:              pulsar.Shared,
    ReceiverQueueSize: 100,  // 降低预取队列大小（默认1000）
    EnableBatchIndexAcknowledgment: true,  // 批量ACK优化
})
```

### 4.2 消费模式优化

```go
// 方案A: 零拷贝消费（如果支持）
// 直接操作底层字节，避免复制

// 方案B: 流式处理
// 不累积到50MB再处理，改为流式 + 定时批量提交

// 方案C: 对象池复用
// 使用 sync.Pool 复用处理过程中的临时对象
```

### 4.3 GC 调优

```go
// 设置 GOGC 环境变量或代码中设置
debug.SetGCPercent(50)  // 更激进的GC

// 或使用 GOMEMLIMIT (Go 1.19+)
// GOMEMLIMIT=512MiB ./consumer
```

---

## 五、预期产出

1. **内存分析报告**
   - 各组件内存占用比例图
   - 内存放大因子的精确测量
   - 热点分配路径

2. **优化建议文档**
   - 参数配置最佳实践
   - 代码层面优化方案
   - 架构层面改进建议

3. **基准测试代码**
   - 可复用的测试框架
   - 自动化分析脚本

---

## 六、执行步骤

### Phase 1: 环境搭建 (Step 1-2)
1. Docker 部署 Pulsar Standalone
2. 初始化 Go 项目结构

### Phase 2: 基准测试 (Step 3-5)
3. 实现 Producer，写入测试数据
4. 实现 Consumer + 内存监控
5. 运行基准测试，收集数据

### Phase 3: 深度分析 (Step 6-7)
6. pprof 分析内存热点
7. 源码级别分析 pulsar-client-go 内存模型

### Phase 4: 优化验证 (Step 8-9)
8. 实施优化方案
9. 对比测试，验证效果

### Phase 5: 文档输出 (Step 10)
10. 生成最终报告和最佳实践文档

---

## 七、风险与注意事项

1. **测试环境隔离**: 确保测试不影响其他服务
2. **数据量控制**: 避免磁盘/内存溢出
3. **版本一致性**: 记录 pulsar-client-go 和 Pulsar 服务端版本
4. **可复现性**: 所有测试步骤可重复执行

---

## 八、参考资料

- [pulsar-client-go GitHub](https://github.com/apache/pulsar-client-go)
- [Pulsar Consumer Configuration](https://pulsar.apache.org/docs/client-libraries-go/)
- [Go pprof 使用指南](https://go.dev/blog/pprof)
- [Go 内存管理](https://go.dev/doc/gc-guide)
