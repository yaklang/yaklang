#!/bin/bash

# 检查risk.json是否存在且非空
if [ -s "risk.json" ]; then
    # 尝试格式化JSON文件
    if jq . risk.json > formatted.json 2>/dev/null; then
        mv formatted.json risk.json
    else
        # 如果格式化失败（例如无效的JSON），删除临时文件
        rm -f formatted.json
    fi
    # 输出错误信息到标准错误并退出，退出码为-1（实际为255）
    echo "risk.json has been formatted" >&2
    exit -1
fi