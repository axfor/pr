#!/usr/bin/env python3
"""分析外部 RSS 监控数据"""
import sys
import json

def analyze_rss(filename):
    """分析 RSS 数据文件，返回 min/max/avg"""
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

    min_rss = min(rss_values)
    max_rss = max(rss_values)
    avg_rss = sum(rss_values) / len(rss_values)

    return {
        'sample_count': len(rss_values),
        'min_rss_kb': min_rss,
        'max_rss_kb': max_rss,
        'avg_rss_kb': avg_rss,
        'min_rss_mb': min_rss / 1024,
        'max_rss_mb': max_rss / 1024,
        'avg_rss_mb': avg_rss / 1024,
    }

def main():
    if len(sys.argv) < 2:
        print("Usage: analyze-external-rss.py <rss_file> [json|text]", file=sys.stderr)
        sys.exit(1)

    filename = sys.argv[1]
    output_format = sys.argv[2] if len(sys.argv) > 2 else 'text'

    result = analyze_rss(filename)

    if result is None:
        print("No valid RSS data found", file=sys.stderr)
        sys.exit(1)

    if output_format == 'json':
        print(json.dumps(result, indent=2))
    else:
        print(f"Samples: {result['sample_count']}")
        print(f"Min RSS: {result['min_rss_mb']:.2f} MB")
        print(f"Max RSS: {result['max_rss_mb']:.2f} MB")
        print(f"Avg RSS: {result['avg_rss_mb']:.2f} MB")

if __name__ == '__main__':
    main()
