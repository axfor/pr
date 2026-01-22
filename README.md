# Pulsar Client Go Memory Optimization Test

测试 `ReleasePayload()` 方法对 Pulsar 消费者内存使用的优化效果。

## pulsar-client-go 改动点

### 新增 API

在 `Message` 接口中新增 `ReleasePayload()` 方法：

```go
// pulsar/message.go
type Message interface {
    // ... 其他方法 ...

    // ReleasePayload 释放消息的 payload 内存
    // 调用后 Payload() 将返回 nil，但 MessageID 等元数据仍可用于 ACK
    // 适用于攒批场景：处理完 payload 后立即释放，减少内存占用
    ReleasePayload()
}
```

### 实现位置

```go
// pulsar/impl_message.go
func (msg *message) ReleasePayload() {
    // 释放 payload 内存，只保留元数据用于 ACK
    msg.payLoad = nil
    // 同时释放 properties 以最大化内存节省
    msg.properties = nil
}
```

### 未压缩消息优化

对于未压缩消息（`CompressionType=NONE`），优化 `Decompress` 方法避免不必要的内存复制：

```go
// pulsar/internal/compression/noop.go
func (noopProvider) Decompress(dst, src []byte, _ int) ([]byte, error) {
    // 优化：对于无压缩数据，直接返回原切片，避免内存复制
    // 这可以显著减少内存占用（约 50%）
    if dst == nil {
        return src, nil
    }
    // ...
}
```

**优化原理**：原实现会为每条消息分配新内存并复制数据，优化后直接返回原始切片，减少约 50% 的内存占用。

### 使用场景

**攒批消费模式**：Consumer 接收消息后先处理业务逻辑，累积到一定数量再批量 ACK。

```go
// 传统方式 - payload 一直占用内存直到 GC
for _, msg := range messages {
    processPayload(msg.Payload())  // 处理数据
    // payload 仍在内存中
}
consumer.AckID(messages[len(messages)-1].ID())  // 批量确认

// 优化方式 - 处理后立即释放 payload
for _, msg := range messages {
    processPayload(msg.Payload())  // 处理数据
    msg.ReleasePayload()           // 立即释放 payload 内存
    // 只保留 MessageID 用于后续 ACK
}
consumer.AckID(messages[len(messages)-1].ID())  // 批量确认
```

### 内存结构对比

```
消息对象内存布局:

未调用 ReleasePayload():
┌─────────────────────────────────────┐
│ Message                             │
├─────────────────────────────────────┤
│ MessageID     │ 约 50-100 bytes     │ ← 保留
│ Properties    │ 可变                │ ← 保留
│ Topic         │ 字符串引用          │ ← 保留
│ Payload       │ 实际数据 (1KB-1MB+) │ ← 主要内存占用
└─────────────────────────────────────┘

调用 ReleasePayload() 后:
┌─────────────────────────────────────┐
│ Message                             │
├─────────────────────────────────────┤
│ MessageID     │ 约 50-100 bytes     │ ← 保留 (用于 ACK)
│ Properties    │ nil                 │ ← 已释放
│ Topic         │ 字符串引用          │ ← 保留
│ Payload       │ nil                 │ ← 已释放
└─────────────────────────────────────┘
```

## Quick Start

```bash
# 启动 Pulsar
make start-pulsar

# 运行内存对比测试
make test

# 运行 queue-size 对比测试
make test-queue-compare

# 停止 Pulsar
make stop-pulsar
```

## Memory Comparison Report

**Test Data:** 500 MB (512,000 messages)

### HeapAlloc (Go Runtime)

| Mode | Min | Max | Avg | Final |
|------|----:|----:|----:|------:|
| Payload 保留 | 1.67 MB | 32.02 MB | 17.38 MB | 23.03 MB |
| Payload 释放 | 1.69 MB | 12.58 MB | 6.96 MB | 6.81 MB |

### RSS (Resident Set Size)

| Mode | Min | Max | Avg | Final |
|------|----:|----:|----:|------:|
| Payload 保留 | 18.53 MB | 68.80 MB | 44.64 MB | 68.80 MB |
| Payload 释放 | 18.77 MB | 36.83 MB | 30.69 MB | 36.83 MB |

## Memory Savings Analysis

| Metric | Payload 保留 | Payload 释放 | Saved | Reduction |
|--------|-------------:|-------------:|------:|----------:|
| Max HeapAlloc | 32.02 MB | 12.58 MB | 19.44 MB | **60.7%** |
| Avg HeapAlloc | 17.38 MB | 6.96 MB | 10.41 MB | **59.9%** |
| Max RSS | 68.80 MB | 36.83 MB | 31.97 MB | **46.5%** |

## Queue Size 对比测试

测试 `ReceiverQueueSize` 对内存的影响（均使用 ReleasePayload）：

| QueueSize | Max HeapAlloc | Saved |
|-----------|-------------:|------:|
| 1000 (default) | 12.31 MB | - |
| 100 | 12.11 MB | **1.6%** |

**结论**：在已使用 `ReleasePayload()` 的情况下，减小 `queue-size` 对内存影响很小（仅 1.6%），因为主要内存占用来自 Payload。

## Conclusion

使用 `ReleasePayload()` 后：
- **HeapAlloc 峰值降低 60.7%**
- **RSS 峰值降低 46.5%**
- 内存放大倍数从 1.98x 降至 0.78x

这是**最有效的内存优化手段**，适用于攒批消费场景。
