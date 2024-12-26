#!/bin/bash

# 设置颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 检查是否安装了 lsof
if ! command -v lsof &> /dev/null; then
    echo "请先安装 lsof"
    exit 1
fi

while true; do
    clear
    echo -e "${GREEN}=== YAK 进程 TCP 连接监控 ===${NC}"
    echo -e "${YELLOW}当前时间: $(date '+%Y-%m-%d %H:%M:%S')${NC}\n"

    # 获取 yak 进程的所有 TCP 连接
    ALL_CONNECTIONS=$(lsof -i TCP -a -p $(pgrep yak) 2>/dev/null)
    LISTEN_CONNECTIONS=$(echo "$ALL_CONNECTIONS" | grep "LISTEN")
    
    if [ -z "$ALL_CONNECTIONS" ]; then
        echo -e "${RED}未找到 yak 进程或没有 TCP 连接${NC}"
    else
        # 统计各种状态的连接数
        TOTAL=$(echo "$ALL_CONNECTIONS" | grep -c TCP)
        ESTABLISHED=$(echo "$ALL_CONNECTIONS" | grep -c ESTABLISHED)
        LISTEN=$(echo "$ALL_CONNECTIONS" | grep -c LISTEN)
        TIME_WAIT=$(echo "$ALL_CONNECTIONS" | grep -c TIME_WAIT)
        CLOSE_WAIT=$(echo "$ALL_CONNECTIONS" | grep -c CLOSE_WAIT)

        # 显示统计信息
        echo -e "${BLUE}=== 连接统计 ===${NC}"
        echo -e "总连接数: ${GREEN}$TOTAL${NC}"
        echo -e "ESTABLISHED: ${GREEN}$ESTABLISHED${NC}"
        echo -e "LISTEN: ${GREEN}$LISTEN${NC}"
        echo -e "TIME_WAIT: ${YELLOW}$TIME_WAIT${NC}"
        echo -e "CLOSE_WAIT: ${RED}$CLOSE_WAIT${NC}\n"

        # 只显示 LISTEN 状态的详细信息
        echo -e "${BLUE}=== 监听端口详细信息 ===${NC}"
        if [ -z "$LISTEN_CONNECTIONS" ]; then
            echo -e "${RED}当前没有监听的端口${NC}"
        else
            echo -e "${YELLOW}进程名称\t\tPID\t\t本地地址:端口\t\t状态${NC}"
            echo -e "----------------------------------------------------------------"
            echo "$LISTEN_CONNECTIONS" | awk '{
                split($9, addr, ":")
                port = addr[length(addr)]
                printf "%-20s %-15s %-25s %-15s\n", $1, $2, $9, $10
            }' | sort -k3 -n
        fi
    fi

    sleep 2
done

