#!/bin/bash
# 外部 RSS 内存监控脚本
# 用法: ./monitor-rss.sh <process_name_pattern> <output_file> <interval_sec>
# 例如: ./monitor-rss.sh "consumer" results/external_rss.txt 1

PATTERN="${1:-consumer}"
OUTPUT="${2:-/tmp/rss_monitor.txt}"
INTERVAL="${3:-1}"

# 清空输出文件
> "$OUTPUT"

echo "timestamp,pid,rss_kb,rss_mb" >> "$OUTPUT"

while true; do
    # 查找匹配的进程
    # macOS 使用 ps aux，Linux 使用 ps aux
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS: RSS 在第 6 列 (KB)
        PS_OUTPUT=$(ps aux | grep "$PATTERN" | grep -v grep | grep -v monitor-rss)
    else
        # Linux: RSS 在第 6 列 (KB)
        PS_OUTPUT=$(ps aux | grep "$PATTERN" | grep -v grep | grep -v monitor-rss)
    fi

    if [ -n "$PS_OUTPUT" ]; then
        while IFS= read -r line; do
            PID=$(echo "$line" | awk '{print $2}')
            RSS_KB=$(echo "$line" | awk '{print $6}')
            RSS_MB=$(echo "scale=2; $RSS_KB / 1024" | bc)
            TIMESTAMP=$(date +%s.%N 2>/dev/null || date +%s)
            echo "$TIMESTAMP,$PID,$RSS_KB,$RSS_MB" >> "$OUTPUT"
        done <<< "$PS_OUTPUT"
    fi

    sleep "$INTERVAL"
done
