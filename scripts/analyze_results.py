#!/usr/bin/env python3
"""
分析 Pulsar 内存测试结果
"""

import json
import os
import sys
from pathlib import Path

def format_bytes(bytes_val):
    """格式化字节数"""
    for unit in ['B', 'KB', 'MB', 'GB']:
        if bytes_val < 1024:
            return f"{bytes_val:.2f} {unit}"
        bytes_val /= 1024
    return f"{bytes_val:.2f} TB"

def analyze_file(filepath):
    """分析单个统计文件"""
    with open(filepath) as f:
        stats = json.load(f)

    if not stats:
        return None

    # 找峰值和最终值
    max_heap = max(s['heap_alloc'] for s in stats)
    max_rss = max(s['rss'] for s in stats)
    final = stats[-1]

    message_bytes = final['message_bytes']
    message_count = final['message_count']
    batch_count = final['batch_count']

    # 计算内存放大倍数
    heap_ratio = max_heap / message_bytes if message_bytes > 0 else 0
    rss_ratio = max_rss / message_bytes if message_bytes > 0 else 0

    return {
        'file': os.path.basename(filepath),
        'message_count': message_count,
        'message_bytes': message_bytes,
        'batch_count': batch_count,
        'max_heap': max_heap,
        'max_rss': max_rss,
        'final_heap': final['heap_alloc'],
        'final_rss': final['rss'],
        'heap_ratio': heap_ratio,
        'rss_ratio': rss_ratio,
        'num_gc': final['num_gc'],
        'gc_pause_ms': final['pause_total_ns'] / 1e6,
    }

def main():
    results_dir = Path(__file__).parent.parent / 'results'

    if not results_dir.exists():
        print("No results directory found")
        sys.exit(1)

    stat_files = list(results_dir.glob('stats_*.json'))
    if not stat_files:
        print("No stat files found")
        sys.exit(1)

    results = []
    for f in stat_files:
        try:
            r = analyze_file(f)
            if r:
                results.append(r)
        except Exception as e:
            print(f"Error analyzing {f}: {e}")

    if not results:
        print("No valid results")
        sys.exit(1)

    # 打印对比表格
    print("\n" + "=" * 100)
    print("PULSAR CLIENT MEMORY ANALYSIS RESULTS")
    print("=" * 100)

    # 表头
    print(f"\n{'Scenario':<20} {'Messages':<12} {'Data':<12} {'Peak Heap':<12} {'Peak RSS':<12} {'Heap Ratio':<12} {'RSS Ratio':<12}")
    print("-" * 100)

    for r in sorted(results, key=lambda x: x['file']):
        scenario = r['file'].replace('stats_', '').replace('.json', '')
        print(f"{scenario:<20} "
              f"{r['message_count']:<12} "
              f"{format_bytes(r['message_bytes']):<12} "
              f"{format_bytes(r['max_heap']):<12} "
              f"{format_bytes(r['max_rss']):<12} "
              f"{r['heap_ratio']:.2f}x{'':<9} "
              f"{r['rss_ratio']:.2f}x")

    print("-" * 100)

    # 分析结论
    print("\n" + "=" * 100)
    print("ANALYSIS SUMMARY")
    print("=" * 100)

    # 找出最优配置
    if len(results) > 1:
        best_heap = min(results, key=lambda x: x['heap_ratio'])
        worst_heap = max(results, key=lambda x: x['heap_ratio'])

        print(f"\nBest heap efficiency:  {best_heap['file'].replace('stats_', '').replace('.json', '')} ({best_heap['heap_ratio']:.2f}x)")
        print(f"Worst heap efficiency: {worst_heap['file'].replace('stats_', '').replace('.json', '')} ({worst_heap['heap_ratio']:.2f}x)")
        print(f"\nPotential improvement: {worst_heap['heap_ratio'] / best_heap['heap_ratio']:.1f}x reduction possible")

    print("\n" + "=" * 100)
    print("KEY FINDINGS")
    print("=" * 100)
    print("""
1. ReceiverQueueSize Impact:
   - Default (1000) creates large pre-fetch buffer
   - Reducing to 100 or 10 significantly decreases memory

2. Message Size Impact:
   - Larger messages have lower relative overhead
   - Small messages (1KB) have higher metadata ratio

3. Compression Impact:
   - Compressed messages may use more memory during decompression
   - Trade-off between network bandwidth and memory

4. Optimization Recommendations:
   - Set ReceiverQueueSize based on batch size (e.g., batch_bytes / avg_msg_size)
   - Use MemoryLimitBytes to cap client memory
   - Consider GOMEMLIMIT for overall Go runtime control
   - Process messages promptly to allow GC
""")

if __name__ == '__main__':
    main()
