#!/bin/bash
# GitHub Comment Tool 快速评论脚本
# 用于在Linux/macOS环境下快速执行GitHub评论

set -e

echo "🚀 GitHub Comment Tool - 快速评论"
echo "====================================="

# 检查Python是否可用
if ! command -v python3 &> /dev/null; then
    echo "❌ 错误: 未找到Python3，请确保Python3已安装并在PATH中"
    exit 1
fi

# 检查配置文件
if [ ! -f ".github/github-commenter.yml" ]; then
    echo "❌ 错误: 未找到配置文件 .github/github-commenter.yml"
    echo "请确保配置文件存在"
    exit 1
fi

# 检查风险报告文件
RISK_FILE=""
if [ -f "risk.json" ]; then
    RISK_FILE="risk.json"
elif [ -f "scripts/ssa-risk-tools/risk.json" ]; then
    RISK_FILE="scripts/ssa-risk-tools/risk.json"
else
    echo "❌ 错误: 未找到风险报告文件"
    echo "请确保以下文件之一存在:"
    echo "  - risk.json"
    echo "  - scripts/ssa-risk-tools/risk.json"
    exit 1
fi

echo "✅ 找到风险报告文件: $RISK_FILE"

# 检查GitHub Token
if [ -z "$GITHUB_TOKEN" ]; then
    echo "⚠️  警告: 未设置 GITHUB_TOKEN 环境变量"
    echo "请设置环境变量或使用 -t 参数提供Token"
    echo ""
    echo "设置方法:"
    echo "  export GITHUB_TOKEN=your_token_here"
    echo ""
    echo "或者使用 -t 参数:"
    echo "  ./quick-comment.sh -t your_token_here -p PR_NUMBER"
    echo ""
fi

# 解析命令行参数
TOKEN=""
PR_NUMBER=""
DRY_RUN=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--token)
            TOKEN="$2"
            shift 2
            ;;
        -p|--pr)
            PR_NUMBER="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN="--dry-run"
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo "❌ 未知参数: $1"
            show_help
            exit 1
            ;;
    esac
done

# 检查必要参数
if [ -z "$PR_NUMBER" ]; then
    show_help
    exit 1
fi

# 构建命令
CMD="python3 scripts/ssa-risk-tools/github_commenter.py -p $PR_NUMBER"
if [ -n "$TOKEN" ]; then
    CMD="$CMD -t $TOKEN"
fi
if [ -n "$DRY_RUN" ]; then
    CMD="$CMD $DRY_RUN"
fi

echo ""
echo "🔧 执行命令: $CMD"
echo ""

# 执行命令
eval $CMD

echo ""
echo "🎉 操作完成"

show_help() {
    echo ""
    echo "用法: $0 [选项] -p PR_NUMBER"
    echo ""
    echo "选项:"
    echo "  -t, --token TOKEN    GitHub Personal Access Token"
    echo "  -p, --pr PR_NUMBER   Pull Request编号 (必需)"
    echo "  --dry-run            干运行模式，只显示将要创建的评论"
    echo "  -h, --help           显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 -p 123"
    echo "  $0 -t ghp_xxx -p 123"
    echo "  $0 -p 123 --dry-run"
    echo ""
    echo "环境变量:"
    echo "  GITHUB_TOKEN         GitHub Personal Access Token"
    echo ""
}
