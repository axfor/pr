# Pulsar Client Go Memory Optimization Test

测试 `ReleasePayload()` 方法对 Pulsar 消费者内存使用的优化效果。

## Quick Start

```bash
# 启动 Pulsar
make start-pulsar

# 运行内存对比测试
make test

# 停止 Pulsar
make stop-pulsar
```

## Memory Comparison Report

**Test Data:** 16.21 MB (16,600 messages)

### HeapAlloc (Go Runtime)

| Mode | Min | Max | Avg | Final |
|------|----:|----:|----:|------:|
| Payload 保留 | 1.67 MB | 32.02 MB | 17.38 MB | 23.03 MB |
| Payload 释放 | 1.69 MB | 12.58 MB | 6.96 MB | 6.81 MB |

### RSS (Internal gopsutil)

| Mode | Min | Max | Avg | Final |
|------|----:|----:|----:|------:|
| Payload 保留 | 18.53 MB | 68.80 MB | 44.64 MB | 68.80 MB |
| Payload 释放 | 18.77 MB | 36.83 MB | 30.69 MB | 36.83 MB |

### RSS (External ps aux)

| Mode | Min | Max | Avg |
|------|----:|----:|----:|
| Payload 保留 | 0.03 MB | 65.72 MB | 27.35 MB |
| Payload 释放 | 0.11 MB | 36.41 MB | 25.98 MB |

## Memory Savings Analysis

| Metric | Payload 保留 | Payload 释放 | Saved | Reduction |
|--------|-------------:|-------------:|------:|----------:|
| Max HeapAlloc | 32.02 MB | 12.58 MB | 19.44 MB | **60.7%** |
| Avg HeapAlloc | 17.38 MB | 6.96 MB | 10.41 MB | **59.9%** |
| Max RSS | 68.80 MB | 36.83 MB | 31.97 MB | **46.5%** |
| Max RSS (External) | 65.72 MB | 36.41 MB | 29.31 MB | **44.6%** |

## Memory Amplification

| Metric | Payload 保留 | Payload 释放 | Reduction |
|--------|-------------:|-------------:|----------:|
| HeapAlloc / DataSize | 1.98x | 0.78x | 1.20x |
| RSS / DataSize | 4.24x | 2.27x | 1.97x |

## Conclusion

使用 `ReleasePayload()` 后：
- **HeapAlloc 峰值降低 60.7%**
- **RSS 峰值降低 46.5%**
- 内存放大倍数从 1.98x 降至 0.78x
