#!/usr/bin/env python3
"""
GitHub Comment Tool 使用示例
演示如何使用配置文件进行快速评论
"""

import os
import sys
import subprocess
from pathlib import Path

def main():
    """演示如何使用GitHub评论工具"""
    
    print("🚀 GitHub Comment Tool 使用示例")
    print("=" * 50)
    
    # 检查配置文件是否存在
    config_file = ".github/github-commenter.yml"
    if not os.path.exists(config_file):
        print(f"❌ 配置文件不存在: {config_file}")
        print("请确保配置文件存在并包含正确的设置")
        return
    
    print(f"✅ 找到配置文件: {config_file}")
    
    # 检查风险报告文件
    risk_files = ["risk.json", "scripts/ssa-risk-tools/risk.json"]
    risk_file = None
    
    for file_path in risk_files:
        if os.path.exists(file_path):
            risk_file = file_path
            break
    
    if not risk_file:
        print("❌ 未找到风险报告文件")
        print("请确保以下文件之一存在:")
        for file_path in risk_files:
            print(f"  - {file_path}")
        return
    
    print(f"✅ 找到风险报告文件: {risk_file}")
    
    # 检查环境变量
    github_token = os.getenv('GITHUB_TOKEN')
    if not github_token:
        print("⚠️  未设置 GITHUB_TOKEN 环境变量")
        print("请设置环境变量或使用 -t 参数提供Token")
        print("export GITHUB_TOKEN=your_token_here")
        return
    
    print("✅ 找到GitHub Token")
    
    # 显示使用示例
    print("\n📖 使用示例:")
    print("-" * 30)
    
    # 示例1: 使用配置文件进行dry-run
    print("1. Dry-run模式 (推荐先运行):")
    print(f"   python3 scripts/ssa-risk-tools/github_commenter.py -p <PR_NUMBER> --dry-run")
    
    # 示例2: 使用配置文件进行实际评论
    print("\n2. 实际评论模式:")
    print(f"   python3 scripts/ssa-risk-tools/github_commenter.py -p <PR_NUMBER>")
    
    # 示例3: 指定自定义JSON文件
    print("\n3. 指定自定义风险报告文件:")
    print(f"   python3 scripts/ssa-risk-tools/github_commenter.py -p <PR_NUMBER> -j {risk_file}")
    
    # 示例4: 覆盖配置中的仓库设置
    print("\n4. 覆盖仓库设置:")
    print(f"   python3 scripts/ssa-risk-tools/github_commenter.py -r owner/repo -p <PR_NUMBER>")
    
    print("\n💡 提示:")
    print("- 配置文件会自动从 .github/github-commenter.yml 加载")
    print("- 可以通过环境变量 GITHUB_TOKEN 设置Token")
    print("- 建议先使用 --dry-run 模式测试")
    print("- 配置文件支持自定义评论模板、过滤规则等")
    
    print("\n🔧 配置文件位置:")
    print(f"  - 主配置: {config_file}")
    print(f"  - 风险报告: {risk_file}")
    
    print("\n✨ 准备就绪！请替换 <PR_NUMBER> 为实际的Pull Request编号")

if __name__ == '__main__':
    main()
