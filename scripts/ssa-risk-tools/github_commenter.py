#!/usr/bin/env python3
"""
GitHub Comment Tool
用于从JSON格式的风险报告中提取问题代码信息并在GitHub上添加评论
支持从.github/github-commenter.yml配置文件读取设置
"""

import json
import requests
import argparse
import sys
import time
import re
import os
import yaml
from pathlib import Path
from typing import Dict, List, Optional


class GitHubCommenter:
    def __init__(self, token: str, repo: str, pr_number: int, config: Dict = None):
        self.token = token
        self.repo = repo
        self.pr_number = pr_number
        self.config = config or {}
        self.session = requests.Session()
        self.session.headers.update({
            'Authorization': f'token {token}',
            'Accept': 'application/vnd.github.v3+json',
            'User-Agent': 'GitHub-Comment-Tool'
        })
        self.base_url = f'https://api.github.com/repos/{repo}'
        
        # 设置API配置
        api_config = self.config.get('comment', {}).get('api', {})
        self.request_delay = api_config.get('request_delay', 1)
        self.max_retries = api_config.get('max_retries', 3)
        self.timeout = api_config.get('timeout', 30)

    def validate_token(self) -> bool:
        """验证GitHub Token"""
        try:
            response = self.session.get('https://api.github.com/user')
            if response.status_code == 200:
                user_data = response.json()
                print(f"✅ GitHub Token 有效，用户: {user_data['login']}")
                return True
            else:
                print(f"❌ GitHub Token 无效 (HTTP {response.status_code})")
                return False
        except Exception as e:
            print(f"❌ 验证Token时出错: {e}")
            return False

    def validate_pr(self) -> bool:
        """验证Pull Request"""
        try:
            response = self.session.get(f'{self.base_url}/pulls/{self.pr_number}')
            if response.status_code == 200:
                pr_data = response.json()
                print(f"✅ 找到Pull Request: {pr_data['title']} (状态: {pr_data['state']})")
                if pr_data['state'] != 'open':
                    print("⚠️  Pull Request 不是开放状态，可能无法添加评论")
                return True
            else:
                print(f"❌ 找不到Pull Request #{self.pr_number} (HTTP {response.status_code})")
                return False
        except Exception as e:
            print(f"❌ 验证PR时出错: {e}")
            return False

    def parse_json_file(self, json_file: str) -> List[Dict]:
        """解析JSON文件并提取风险信息"""
        try:
            with open(json_file, 'r', encoding='utf-8') as f:
                data = json.load(f)
            
            risks = []
            # 支持新的JSON格式 (Risks字段，大写R)
            risks_data = data.get('Risks', {})
            if not risks_data:
                # 兼容旧格式 (risks字段，小写r)
                risks_data = data.get('risks', {})
            
            for risk_id, risk_data in risks_data.items():
                if risk_data and isinstance(risk_data, dict):
                    # 提取文件路径，支持code_source_url字段
                    file_path = risk_data.get('code_source_url', '')
                    if not file_path:
                        file_path = risk_data.get('file_path', '')
                    
                    # 清理文件路径，移除前缀
                    if file_path.startswith('/'):
                        file_path = file_path[1:]
                    
                    risk_info = {
                        'id': risk_id,
                        'file_path': file_path,
                        'line': risk_data.get('line', 0),
                        'severity': risk_data.get('severity', 'unknown'),
                        'title': risk_data.get('title', ''),
                        'title_verbose': risk_data.get('title_verbose', ''),
                        'description': risk_data.get('description', ''),
                        'solution': risk_data.get('solution', ''),
                        'rule_name': risk_data.get('rule_name', ''),
                        'function_name': risk_data.get('function_name', ''),
                        'program_name': risk_data.get('program_name', ''),
                        'language': risk_data.get('language', ''),
                        'risk_type': risk_data.get('risk_type', ''),
                        'cve': risk_data.get('cve', ''),
                        'cwe': risk_data.get('cwe', []),
                        'time': risk_data.get('time', ''),
                        'latest_disposal_status': risk_data.get('latest_disposal_status', '')
                    }
                    
                    # 应用过滤器
                    if self._should_include_risk(risk_info):
                        risks.append(risk_info)
            
            print(f"✅ 找到 {len(risks)} 个风险")
            return risks
        except Exception as e:
            print(f"❌ 解析JSON文件时出错: {e}")
            return []
    
    def _should_include_risk(self, risk: Dict) -> bool:
        """检查风险是否应该被包含在评论中"""
        filters = self.config.get('comment', {}).get('filters', {})
        
        # 检查严重程度
        min_severity = filters.get('min_severity', 'info')
        severity_order = ['info', 'low', 'medium', 'middle', 'high', 'critical']
        risk_severity = risk.get('severity', 'unknown').lower()
        
        try:
            risk_level = severity_order.index(risk_severity)
            min_level = severity_order.index(min_severity.lower())
            if risk_level < min_level:
                return False
        except ValueError:
            # 如果严重程度不在列表中，默认包含
            pass
        
        # 检查文件路径过滤
        file_path = risk.get('file_path', '')
        exclude_files = filters.get('exclude_files', [])
        exclude_dirs = filters.get('exclude_dirs', [])
        
        # 检查文件模式
        for pattern in exclude_files:
            if self._match_pattern(file_path, pattern):
                return False
        
        # 检查目录模式
        for pattern in exclude_dirs:
            if self._match_pattern(file_path, pattern):
                return False
        
        return True
    
    def _match_pattern(self, file_path: str, pattern: str) -> bool:
        """简单的glob模式匹配"""
        import fnmatch
        return fnmatch.fnmatch(file_path, pattern)

    def create_comment_body(self, risk: Dict) -> str:
        """创建评论内容"""
        # 从配置中获取emoji映射
        template_config = self.config.get('comment', {}).get('template', {})
        severity_emojis = template_config.get('severity_emojis', {
            'critical': '🔴',
            'high': '🟠',
            'medium': '🟡',
            'middle': '🟡',
            'low': '🟢',
            'info': 'ℹ️',
            'unknown': '⚪'
        })
        
        severity_emoji = severity_emojis.get(risk['severity'].lower(), '⚪')
        
        # 使用title_verbose作为显示标题，如果没有则使用title
        display_title = risk.get('title_verbose', '') or risk.get('title', '')
        
        # 构建评论内容
        header_template = template_config.get('header', 
            "## {emoji} 代码安全问题检测\n\n**严重程度:** `{severity}`\n**问题:** {title}\n\n")
        
        description_template = template_config.get('description', 
            "**描述:**\n{description}\n\n")
        
        solution_template = template_config.get('solution', 
            "**建议解决方案:**\n{solution}\n\n")
        
        footer_template = template_config.get('footer', 
            "---\n*此评论由代码安全扫描工具自动生成*")
        
        # 替换模板变量
        body = header_template.format(
            emoji=severity_emoji,
            severity=risk['severity'],
            title=display_title
        )
        
        if risk.get('description', '').strip():
            body += description_template.format(description=risk['description'])
        
        if risk.get('solution', '').strip():
            body += solution_template.format(solution=risk['solution'])
        
        body += footer_template
        
        return body

    def create_comment(self, risk: Dict, dry_run: bool = False) -> bool:
        """创建GitHub评论"""
        if not risk['file_path'] or not risk['line']:
            print(f"⚠️  跳过风险 {risk['id']}: 缺少文件路径或行号信息")
            return False
        
        comment_data = {
            'body': self.create_comment_body(risk),
            'path': risk['file_path'],
            'line': int(risk['line']),
            'side': 'RIGHT'
        }
        
        if dry_run:
            print(f"🔍 DRY RUN - 将为以下位置创建评论:")
            print(f"  文件: {risk['file_path']}")
            print(f"  行号: {risk['line']}")
            print(f"  严重程度: {risk['severity']}")
            print(f"  标题: {risk['title']}")
            print()
            return True
        
        try:
            response = self.session.post(
                f'{self.base_url}/pulls/{self.pr_number}/comments',
                json=comment_data
            )
            
            if response.status_code == 201:
                print(f"✅ 成功为 {risk['file_path']}:{risk['line']} 创建评论")
                return True
            else:
                print(f"❌ 创建评论失败 (HTTP {response.status_code})")
                print(f"响应: {response.text}")
                return False
        except Exception as e:
            print(f"❌ 创建评论时出错: {e}")
            return False

    def process_risks(self, risks: List[Dict], dry_run: bool = False) -> None:
        """处理所有风险"""
        print(f"🚀 开始处理风险...")
        
        success_count = 0
        error_count = 0
        
        for i, risk in enumerate(risks, 1):
            print(f"处理风险 {i}/{len(risks)}: {risk.get('title', 'Unknown')}")
            
            if self.create_comment(risk, dry_run):
                success_count += 1
            else:
                error_count += 1
            
            # 添加延迟以避免API限制
            if not dry_run and i < len(risks):
                time.sleep(self.request_delay)
        
        print(f"📊 处理完成: 成功 {success_count}, 失败 {error_count}")


def load_config(config_path: str = None) -> Dict:
    """加载配置文件"""
    if config_path is None:
        # 尝试从多个位置查找配置文件
        possible_paths = [
            '.github/github-commenter.yml',
            'github-commenter.yml',
            'scripts/ssa-risk-tools/github-commenter.yml'
        ]
        
        for path in possible_paths:
            if os.path.exists(path):
                config_path = path
                break
    
    if config_path and os.path.exists(config_path):
        try:
            with open(config_path, 'r', encoding='utf-8') as f:
                config = yaml.safe_load(f)
            print(f"✅ 加载配置文件: {config_path}")
            return config
        except Exception as e:
            print(f"⚠️  加载配置文件失败: {e}")
            return {}
    else:
        print("ℹ️  未找到配置文件，使用默认设置")
        return {}


def main():
    parser = argparse.ArgumentParser(
        description='GitHub Comment Tool - 在GitHub上为问题代码添加评论',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  python3 github_commenter.py -t ghp_xxx -r owner/repo -p 123 -j risk.json
  python3 github_commenter.py --token ghp_xxx --repo owner/repo --pr 123 --json risk.json --dry-run
  python3 github_commenter.py --config .github/github-commenter.yml --token ghp_xxx --repo owner/repo --pr 123

JSON文件格式要求:
  - 包含 'Risks' 或 'risks' 字段，每个风险包含:
    - 'code_source_url' 或 'file_path': 文件路径
    - 'line': 行号
    - 'severity': 严重程度
    - 'title': 问题标题
    - 'title_verbose': 中文标题
    - 'description': 问题描述
    - 'solution': 解决方案
        """
    )
    
    parser.add_argument('-t', '--token', help='GitHub Personal Access Token')
    parser.add_argument('-r', '--repo', help='GitHub仓库 (格式: owner/repo)')
    parser.add_argument('-p', '--pr', type=int, help='Pull Request编号')
    parser.add_argument('-j', '--json', help='JSON格式的风险报告文件')
    parser.add_argument('-c', '--config', help='配置文件路径 (默认: .github/github-commenter.yml)')
    parser.add_argument('-d', '--dry-run', action='store_true', help='dry run模式，只显示将要创建的评论，不实际创建')
    
    args = parser.parse_args()
    
    # 加载配置文件
    config = load_config(args.config)
    
    # 从配置文件或命令行参数获取必要信息
    token = args.token or os.getenv('GITHUB_TOKEN')
    repo = args.repo or config.get('default_repo')
    pr_number = args.pr
    json_file = args.json or config.get('risk_report', {}).get('json_file', 'risk.json')
    
    # 检查必要参数
    if not token:
        print("❌ 错误: 需要提供GitHub Token (通过 -t 参数或 GITHUB_TOKEN 环境变量)")
        sys.exit(1)
    
    if not repo:
        print("❌ 错误: 需要提供GitHub仓库 (通过 -r 参数或配置文件)")
        sys.exit(1)
    
    if not pr_number:
        print("❌ 错误: 需要提供Pull Request编号 (通过 -p 参数)")
        sys.exit(1)
    
    # 检查是否启用评论功能
    if not config.get('comment', {}).get('enabled', True):
        print("ℹ️  评论功能已在配置中禁用")
        sys.exit(0)
    
    # 创建GitHub评论器
    commenter = GitHubCommenter(token, repo, pr_number, config)
    
    # 验证Token (dry-run模式下跳过)
    if not args.dry_run:
        if not commenter.validate_token():
            sys.exit(1)
    else:
        print("🔍 Dry-run模式，跳过Token验证")
    
    # 验证PR (dry-run模式下跳过)
    if not args.dry_run:
        if not commenter.validate_pr():
            sys.exit(1)
    else:
        print("🔍 Dry-run模式，跳过PR验证")
    
    # 解析JSON文件
    risks = commenter.parse_json_file(json_file)
    if not risks:
        print("⚠️  没有找到风险信息")
        sys.exit(0)
    
    # 处理风险
    commenter.process_risks(risks, args.dry_run)
    
    print("🎉 所有操作完成")


if __name__ == '__main__':
    main()
