#!/usr/bin/env python3
"""对比分析内存测试结果"""
import sys
import json
import os

def load_stats(filename):
    """加载统计数据"""
    if not os.path.exists(filename):
        return None
    with open(filename, 'r') as f:
        return json.load(f)

def load_external_rss(filename):
    """加载外部 RSS 数据"""
    if not os.path.exists(filename):
        return None

    rss_values = []
    with open(filename, 'r') as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith('timestamp'):
                continue
            parts = line.split(',')
            if len(parts) >= 3:
                try:
                    rss_kb = int(parts[2])
                    rss_values.append(rss_kb)
                except ValueError:
                    continue

    if not rss_values:
        return None

    return {
        'min': min(rss_values) / 1024,
        'max': max(rss_values) / 1024,
        'avg': sum(rss_values) / len(rss_values) / 1024,
    }

def format_mb(value):
    """格式化为 MB"""
    return f"{value:.2f}"

def print_comparison(no_release, with_release, ext_no_release, ext_with_release):
    """打印对比结果"""

    print("")
    print("=" * 70)
    print("                    MEMORY COMPARISON REPORT")
    print("=" * 70)

    # 数据量信息
    if no_release:
        data_mb = no_release['summary']['message_bytes'] / 1024 / 1024
        msg_count = no_release['summary']['message_count']
        print(f"  Test Data: {data_mb:.2f} MB ({msg_count:,} messages)")
    print("")

    # HeapAlloc 对比表格
    print("-" * 70)
    print("  HeapAlloc (Go Runtime) - Unit: MB")
    print("-" * 70)
    print(f"  {'Mode':<20} {'Min':>10} {'Max':>10} {'Avg':>10} {'Final':>10}")
    print(f"  {'-'*20} {'-'*10} {'-'*10} {'-'*10} {'-'*10}")

    if no_release:
        s = no_release['summary']
        print(f"  {'WITHOUT ReleasePayload':<20} {format_mb(s['min_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['max_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['avg_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['final_heap_alloc']/1024/1024):>10}")

    if with_release:
        s = with_release['summary']
        print(f"  {'WITH ReleasePayload':<20} {format_mb(s['min_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['max_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['avg_heap_alloc']/1024/1024):>10} "
              f"{format_mb(s['final_heap_alloc']/1024/1024):>10}")

    print("")

    # RSS 对比表格 (内部)
    print("-" * 70)
    print("  RSS (Internal gopsutil) - Unit: MB")
    print("-" * 70)
    print(f"  {'Mode':<20} {'Min':>10} {'Max':>10} {'Avg':>10} {'Final':>10}")
    print(f"  {'-'*20} {'-'*10} {'-'*10} {'-'*10} {'-'*10}")

    if no_release:
        s = no_release['summary']
        print(f"  {'WITHOUT ReleasePayload':<20} {format_mb(s['min_rss']/1024/1024):>10} "
              f"{format_mb(s['max_rss']/1024/1024):>10} "
              f"{format_mb(s['avg_rss']/1024/1024):>10} "
              f"{format_mb(s['final_rss']/1024/1024):>10}")

    if with_release:
        s = with_release['summary']
        print(f"  {'WITH ReleasePayload':<20} {format_mb(s['min_rss']/1024/1024):>10} "
              f"{format_mb(s['max_rss']/1024/1024):>10} "
              f"{format_mb(s['avg_rss']/1024/1024):>10} "
              f"{format_mb(s['final_rss']/1024/1024):>10}")

    print("")

    # 外部 RSS 对比表格
    if ext_no_release or ext_with_release:
        print("-" * 70)
        print("  RSS (External ps aux) - Unit: MB")
        print("-" * 70)
        print(f"  {'Mode':<20} {'Min':>10} {'Max':>10} {'Avg':>10}")
        print(f"  {'-'*20} {'-'*10} {'-'*10} {'-'*10}")

        if ext_no_release:
            print(f"  {'WITHOUT ReleasePayload':<20} {format_mb(ext_no_release['min']):>10} "
                  f"{format_mb(ext_no_release['max']):>10} "
                  f"{format_mb(ext_no_release['avg']):>10}")

        if ext_with_release:
            print(f"  {'WITH ReleasePayload':<20} {format_mb(ext_with_release['min']):>10} "
                  f"{format_mb(ext_with_release['max']):>10} "
                  f"{format_mb(ext_with_release['avg']):>10}")

        print("")

    # 内存节省分析
    if no_release and with_release:
        print("=" * 70)
        print("                      MEMORY SAVINGS ANALYSIS")
        print("=" * 70)

        no_s = no_release['summary']
        with_s = with_release['summary']

        # HeapAlloc Max 节省
        no_max_heap = no_s['max_heap_alloc'] / 1024 / 1024
        with_max_heap = with_s['max_heap_alloc'] / 1024 / 1024
        heap_saved = no_max_heap - with_max_heap
        heap_pct = (heap_saved / no_max_heap * 100) if no_max_heap > 0 else 0

        print(f"  Max HeapAlloc:")
        print(f"    WITHOUT: {no_max_heap:>10.2f} MB")
        print(f"    WITH:    {with_max_heap:>10.2f} MB")
        print(f"    SAVED:   {heap_saved:>10.2f} MB  ({heap_pct:.1f}%)")
        print("")

        # HeapAlloc Avg 节省
        no_avg_heap = no_s['avg_heap_alloc'] / 1024 / 1024
        with_avg_heap = with_s['avg_heap_alloc'] / 1024 / 1024
        heap_avg_saved = no_avg_heap - with_avg_heap
        heap_avg_pct = (heap_avg_saved / no_avg_heap * 100) if no_avg_heap > 0 else 0

        print(f"  Avg HeapAlloc:")
        print(f"    WITHOUT: {no_avg_heap:>10.2f} MB")
        print(f"    WITH:    {with_avg_heap:>10.2f} MB")
        print(f"    SAVED:   {heap_avg_saved:>10.2f} MB  ({heap_avg_pct:.1f}%)")
        print("")

        # RSS Max 节省
        no_max_rss = no_s['max_rss'] / 1024 / 1024
        with_max_rss = with_s['max_rss'] / 1024 / 1024
        rss_saved = no_max_rss - with_max_rss
        rss_pct = (rss_saved / no_max_rss * 100) if no_max_rss > 0 else 0

        print(f"  Max RSS:")
        print(f"    WITHOUT: {no_max_rss:>10.2f} MB")
        print(f"    WITH:    {with_max_rss:>10.2f} MB")
        print(f"    SAVED:   {rss_saved:>10.2f} MB  ({rss_pct:.1f}%)")
        print("")

        # 外部 RSS 节省
        if ext_no_release and ext_with_release:
            ext_no_max = ext_no_release['max']
            ext_with_max = ext_with_release['max']
            ext_saved = ext_no_max - ext_with_max
            ext_pct = (ext_saved / ext_no_max * 100) if ext_no_max > 0 else 0

            print(f"  Max RSS (External):")
            print(f"    WITHOUT: {ext_no_max:>10.2f} MB")
            print(f"    WITH:    {ext_with_max:>10.2f} MB")
            print(f"    SAVED:   {ext_saved:>10.2f} MB  ({ext_pct:.1f}%)")
            print("")

        # 内存放大倍数对比
        print("-" * 70)
        print("  Memory Amplification (Max Memory / Data Size)")
        print("-" * 70)
        data_bytes = no_s['message_bytes']
        if data_bytes > 0:
            no_heap_ratio = no_s['max_heap_alloc'] / data_bytes
            with_heap_ratio = with_s['max_heap_alloc'] / data_bytes
            no_rss_ratio = no_s['max_rss'] / data_bytes
            with_rss_ratio = with_s['max_rss'] / data_bytes

            print(f"  {'Metric':<25} {'WITHOUT':>12} {'WITH':>12} {'Reduction':>12}")
            print(f"  {'-'*25} {'-'*12} {'-'*12} {'-'*12}")
            print(f"  {'HeapAlloc/DataSize':<25} {no_heap_ratio:>11.2f}x {with_heap_ratio:>11.2f}x {no_heap_ratio-with_heap_ratio:>11.2f}x")
            print(f"  {'RSS/DataSize':<25} {no_rss_ratio:>11.2f}x {with_rss_ratio:>11.2f}x {no_rss_ratio-with_rss_ratio:>11.2f}x")

        print("")

    print("=" * 70)

def main():
    results_dir = sys.argv[1] if len(sys.argv) > 1 else './results'

    no_release = load_stats(os.path.join(results_dir, 'stats_no-release.json'))
    with_release = load_stats(os.path.join(results_dir, 'stats_with-release.json'))
    ext_no_release = load_external_rss(os.path.join(results_dir, 'external_rss_no-release.txt'))
    ext_with_release = load_external_rss(os.path.join(results_dir, 'external_rss_with-release.txt'))

    if not no_release and not with_release:
        print("No stats files found in", results_dir)
        sys.exit(1)

    print_comparison(no_release, with_release, ext_no_release, ext_with_release)

if __name__ == '__main__':
    main()
