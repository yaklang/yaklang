# GitHub 安全扫描和评论工具

这个工具集提供了多种方式来集成代码安全扫描和GitHub PR评论功能。

## 🚀 功能特性

- **自动安全扫描**: 使用SyntaxFlow进行Go代码安全扫描
- **智能评论**: 在PR中自动添加安全问题的详细评论
- **多种集成方式**: 支持自定义Action、社区Action和直接脚本调用
- **灵活配置**: 支持YAML配置文件自定义评论模板和过滤规则
- **条件评论**: 只在发现安全问题时才添加评论

## 📁 文件结构

```
scripts/ssa-risk-tools/
├── github_commenter.py          # 主要的Python评论脚本
├── extract-risks.awk           # AWK脚本用于提取风险信息
├── quick-comment.bat           # Windows批处理脚本
├── quick-comment.sh            # Linux/macOS shell脚本
├── example_usage.py            # 使用示例脚本
└── README.md                   # 本文档

.github/
├── github-commenter.yml        # 配置文件
└── workflows/
    ├── security-scan-comment.yml           # 自定义Action工作流
    ├── security-comment-simple.yml         # 简化版工作流
    ├── security-comment-community.yml      # 社区Action工作流
    └── diff-code-check.yml                 # 集成到现有工作流

.github/actions/
└── security-commenter/
    └── action.yml              # 自定义GitHub Action
```

## 🔧 配置选项

### 1. 配置文件 (.github/github-commenter.yml)

```yaml
# 默认仓库配置
default_repo: "yaklang/yaklang"

# 评论配置
comment:
  enabled: true
  
  # 评论模板配置
  template:
    severity_emojis:
      critical: "🔴"
      high: "🟠" 
      medium: "🟡"
      low: "🟢"
      info: "ℹ️"
    
  # 过滤配置
  filters:
    min_severity: "info"
    exclude_files:
      - "*.test.go"
      - "*/test/*"
    exclude_dirs:
      - "test"
      - "vendor"
```

## 🛠️ 使用方法

### 方法1: 使用自定义Action (推荐)

```yaml
# .github/workflows/security-scan-comment.yml
name: Security Scan and Comment
on:
  pull_request:
    branches: [ main ]

jobs:
  security-scan:
    runs-on: ubuntu-22.04
    steps:
      # ... 扫描步骤 ...
      
      - name: Comment on PR with security findings
        if: steps.scan.outputs.scan_result == 'failure'
        uses: ./.github/actions/security-commenter
        with:
          risk_json_path: risk.json
          github_token: ${{ secrets.GITHUB_TOKEN }}
          pr_number: ${{ github.event.pull_request.number }}
          repo: ${{ github.repository }}
```

### 方法2: 使用社区Action (最简单)

```yaml
# .github/workflows/security-comment-community.yml
name: Security Comment (Community Action)
on:
  pull_request:
    branches: [ main ]

jobs:
  security-scan-and-comment:
    runs-on: ubuntu-22.04
    steps:
      # ... 扫描步骤 ...
      
      - name: Comment PR with security findings
        if: steps.scan.outputs.scan_result == 'failure'
        uses: JoseThen/comment-pr@v1
        with:
          comment: ${{ steps.report.outputs.report_content }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### 方法3: 直接脚本调用

```bash
# 使用配置文件
python3 scripts/ssa-risk-tools/github_commenter.py -p 123

# 指定参数
python3 scripts/ssa-risk-tools/github_commenter.py \
  --token ghp_xxx \
  --repo owner/repo \
  --pr 123 \
  --json risk.json

# Dry-run模式
python3 scripts/ssa-risk-tools/github_commenter.py -p 123 --dry-run
```

### 方法4: 使用快速脚本

```bash
# Windows
quick-comment.bat -p 123

# Linux/macOS
./quick-comment.sh -p 123
```

## 📋 工作流选项对比

| 方法 | 复杂度 | 灵活性 | 维护性 | 推荐场景 |
|------|--------|--------|--------|----------|
| 自定义Action | 高 | 最高 | 高 | 复杂项目，需要高度定制 |
| 社区Action | 低 | 中 | 中 | 快速集成，简单需求 |
| 简化工作流 | 中 | 中 | 中 | 中等复杂度项目 |
| 直接脚本 | 低 | 高 | 低 | 本地测试，一次性使用 |

## 🔍 评论示例

### 成功扫描
```
## ✅ 代码安全检查通过

🎉 代码安全扫描未发现任何问题。

**扫描统计:**
- 扫描文件数: 15
- 发现风险数: 0

---
*此评论由代码安全检查工具自动生成*
```

### 发现安全问题
```
## 🔍 代码安全扫描报告

**扫描时间:** 2025-01-23T10:30:00Z
**程序名称:** yaklang
**编程语言:** golang
**扫描文件数:** 15
**代码行数:** 5076
**发现风险数:** 2

### 🚨 风险详情

#### 审计Golang中Init函数内的数据库操作

**严重程度:** `high`
**位置:** common/yak/init.go:25
**规则:** golang-database-init.sf
**函数:** init

**描述:**
该规则用于审计Golang代码中在`init`函数内执行数据库操作的情况...

**建议解决方案:**
使用延迟初始化钩子，通过注册回调函数在数据库初始化完成后执行操作...

**问题代码:**
```go
func init() {
    db := consts.GetGormProfileDatabase()
    autoAutomigrateVectorStoreDocument(db)
}
```

---
*此报告由代码安全扫描工具自动生成*
```

## 🚨 故障排除

### 常见问题

1. **配置文件未找到**
   - 确保 `.github/github-commenter.yml` 存在
   - 检查文件路径和权限

2. **GitHub Token无效**
   - 检查Token权限是否包含 `repo` 和 `pull_requests`
   - 确认Token未过期

3. **风险报告格式不匹配**
   - 确保 `risk.json` 包含 `Risks` 字段
   - 检查JSON格式是否正确

4. **评论未显示**
   - 检查PR是否处于开放状态
   - 确认GitHub API限制未超限

### 调试模式

```bash
# 启用详细日志
export GITHUB_COMMENTER_DEBUG=1
python3 scripts/ssa-risk-tools/github_commenter.py -p 123 --dry-run
```

## 📚 相关文档

- [GitHub Actions 文档](https://docs.github.com/en/actions)
- [SyntaxFlow 文档](https://github.com/yaklang/syntaxflow)
- [GitHub API 文档](https://docs.github.com/en/rest)

## 🤝 贡献

欢迎提交Issue和Pull Request来改进这个工具！

## 📄 许可证

本项目采用与主项目相同的许可证。