#!/bin/bash
LOG_FILE="build/test-nuclear-clear.log"

while true; do
    count=$(grep "unit_batch_" "$LOG_FILE" 2>/dev/null | wc -l)
    last=$(grep "unit_batch_" "$LOG_FILE" 2>/dev/null | tail -1 | grep -oP 'HeapInuse=\s*\K[0-9.]+MB' || echo "N/A")

    echo "[$(date +%H:%M:%S)] Batches: $count/24, Last heap: $last"

    if [ "$count" -ge 24 ]; then
        echo "✓ All 24 batches completed!"
        grep "编译完成" "$LOG_FILE" && echo "✓ Compilation finished!"
        exit 0
    fi

    if grep -q "编译完成" "$LOG_FILE" 2>/dev/null; then
        echo "✓ Compilation completed!"
        exit 0
    fi

    if ! pgrep -f "test-nuclear-clear" > /dev/null; then
        echo "✗ Process not running!"
        dmesg | grep -i "killed process" | tail -1
        exit 1
    fi

    sleep 10
done
