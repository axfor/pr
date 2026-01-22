#!/usr/bin/env python3
"""对比分析 ReceiverQueueSize 对内存的影响"""
import sys
import json
import os

def load_stats(filename):
    """加载统计数据"""
    if not os.path.exists(filename):
        return None
    with open(filename, 'r') as f:
        return json.load(f)

def format_mb(value):
    """格式化为 MB"""
    return f"{value:.2f}"

def print_comparison(queue_1000, queue_100):
    """打印对比结果"""

    print("")
    print("=" * 70)
    print("              QUEUE SIZE COMPARISON REPORT")
    print("              (Both use ReleasePayload)")
    print("=" * 70)

    # 数据量信息
    if queue_1000:
        data_mb = queue_1000['summary']['message_bytes'] / 1024 / 1024
        msg_count = queue_1000['summary']['message_count']
        print(f"  Test Data: {data_mb:.2f} MB ({msg_count:,} messages)")
    print("")

    # HeapAlloc 对比表格
    print("-" * 70)
    print("  HeapAlloc (Go Runtime) - Unit: MB")
    print("-" * 70)
    print(f"  {'QueueSize':<15} {'Min':>10} {'Max':>10} {'Avg':>10} {'Final':>10}")
    print(f"  {'-'*15} {'-'*10} {'-'*10} {'-'*10} {'-'*10}")

    if queue_1000:
        s = queue_1000['summary']
        print(f"  {'1000 (default)':<15} {format_mb(s['min_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['max_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['avg_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['final_heap_alloc']/1024/1024):>10}")

    if queue_100:
        s = queue_100['summary']
        print(f"  {'100':<15} {format_mb(s['min_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['max_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['avg_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['final_heap_alloc']/1024/1024):>10}")

    print("")

    # RSS 对比表格
    print("-" * 70)
    print("  RSS (Resident Set Size) - Unit: MB")
    print("-" * 70)
    print(f"  {'QueueSize':<15} {'Min':>10} {'Max':>10} {'Avg':>10} {'Final':>10}")
    print(f"  {'-'*15} {'-'*10} {'-'*10} {'-'*10} {'-'*10}")

    if queue_1000:
        s = queue_1000['summary']
        print(f"  {'1000 (default)':<15} {format_mb(s['min_rss']/1024/1024):>10} "
              f"{format_mb(s['max_rss']/1024/1024):>10} "
              f"{format_mb(s['avg_rss']/1024/1024):>10} "
              f"{format_mb(s['final_rss']/1024/1024):>10}")

    if queue_100:
        s = queue_100['summary']
        print(f"  {'100':<15} {format_mb(s['min_rss']/1024/1024):>10} "
              f"{format_mb(s['max_rss']/1024/1024):>10} "
              f"{format_mb(s['avg_rss']/1024/1024):>10} "
              f"{format_mb(s['final_rss']/1024/1024):>10}")

    print("")

    # 内存节省分析
    if queue_1000 and queue_100:
        print("=" * 70)
        print("              MEMORY SAVINGS ANALYSIS")
        print("              (queue-size=100 vs queue-size=1000)")
        print("=" * 70)

        s1000 = queue_1000['summary']
        s100 = queue_100['summary']

        # HeapAlloc Max 节省
        heap_1000 = s1000['max_heap_alloc'] / 1024 / 1024
        heap_100 = s100['max_heap_alloc'] / 1024 / 1024
        heap_saved = heap_1000 - heap_100
        heap_pct = (heap_saved / heap_1000 * 100) if heap_1000 > 0 else 0

        print(f"  Max HeapAlloc:")
        print(f"    queue=1000: {heap_1000:>10.2f} MB")
        print(f"    queue=100:  {heap_100:>10.2f} MB")
        print(f"    SAVED:      {heap_saved:>10.2f} MB  ({heap_pct:.1f}%)")
        print("")

        # HeapAlloc Avg 节省
        heap_avg_1000 = s1000['avg_heap_alloc'] / 1024 / 1024
        heap_avg_100 = s100['avg_heap_alloc'] / 1024 / 1024
        heap_avg_saved = heap_avg_1000 - heap_avg_100
        heap_avg_pct = (heap_avg_saved / heap_avg_1000 * 100) if heap_avg_1000 > 0 else 0

        print(f"  Avg HeapAlloc:")
        print(f"    queue=1000: {heap_avg_1000:>10.2f} MB")
        print(f"    queue=100:  {heap_avg_100:>10.2f} MB")
        print(f"    SAVED:      {heap_avg_saved:>10.2f} MB  ({heap_avg_pct:.1f}%)")
        print("")

        # RSS Max 节省
        rss_1000 = s1000['max_rss'] / 1024 / 1024
        rss_100 = s100['max_rss'] / 1024 / 1024
        rss_saved = rss_1000 - rss_100
        rss_pct = (rss_saved / rss_1000 * 100) if rss_1000 > 0 else 0

        print(f"  Max RSS:")
        print(f"    queue=1000: {rss_1000:>10.2f} MB")
        print(f"    queue=100:  {rss_100:>10.2f} MB")
        print(f"    SAVED:      {rss_saved:>10.2f} MB  ({rss_pct:.1f}%)")
        print("")

        # 内存放大倍数对比
        print("-" * 70)
        print("  Memory Amplification (Max Memory / Data Size)")
        print("-" * 70)
        data_bytes = s1000['message_bytes']
        if data_bytes > 0:
            heap_ratio_1000 = s1000['max_heap_alloc'] / data_bytes
            heap_ratio_100 = s100['max_heap_alloc'] / data_bytes
            rss_ratio_1000 = s1000['max_rss'] / data_bytes
            rss_ratio_100 = s100['max_rss'] / data_bytes

            print(f"  {'Metric':<20} {'queue=1000':>12} {'queue=100':>12} {'Reduction':>12}")
            print(f"  {'-'*20} {'-'*12} {'-'*12} {'-'*12}")
            print(f"  {'HeapAlloc/DataSize':<20} {heap_ratio_1000:>11.2f}x {heap_ratio_100:>11.2f}x {heap_ratio_1000-heap_ratio_100:>11.2f}x")
            print(f"  {'RSS/DataSize':<20} {rss_ratio_1000:>11.2f}x {rss_ratio_100:>11.2f}x {rss_ratio_1000-rss_ratio_100:>11.2f}x")

        print("")

    print("=" * 70)
    print("  Note: Smaller queue-size reduces prefetch memory but may")
    print("        affect throughput. Choose based on your memory constraints.")
    print("=" * 70)

def main():
    results_dir = sys.argv[1] if len(sys.argv) > 1 else './results'

    queue_1000 = load_stats(os.path.join(results_dir, 'stats_queue-1000.json'))
    queue_100 = load_stats(os.path.join(results_dir, 'stats_queue-100.json'))

    if not queue_1000 and not queue_100:
        print("No queue comparison stats found in", results_dir)
        sys.exit(1)

    print_comparison(queue_1000, queue_100)

if __name__ == '__main__':
    main()
