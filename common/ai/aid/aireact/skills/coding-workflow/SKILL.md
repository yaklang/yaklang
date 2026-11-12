---
name: coding-workflow
description: >
  Repository coding workflow for implementation, bug fixing, refactoring, code
  review, tests, builds, and debugging. Use the existing file, shell, Git,
  search, MCP, and skill capabilities while keeping changes minimal and
  verified.
metadata:
  display_name_zh-CN: 编程工作流
---

# 编程工作流

本技能只描述软件工程任务的执行顺序，不限制可用工具。MCP、用户 Skill、代码搜索、Shell、Git、文件工具和动态能力发现仍然可用。

## 开始前

1. 明确用户要求的结果、验收条件和明确不做的内容。
2. 阅读仓库级约定：`AGENTS.md`、`CLAUDE.md`、`CONTRIBUTING`、README、构建清单和相邻实现。
3. 定位入口、调用链、测试、配置和生成代码边界。
4. 先运行与目标最接近的基线测试或构建，记录真实退出码；无法运行时说明原因。

## 实施

1. 非平凡任务先建立 TODO，一个当前项只对应一个可验证目标。
2. 修改前读取文件当前内容。已有文件优先增量修改，新文件才使用整文件写入。
3. 每次修改保持最小范围，复用现有抽象、命名和错误语义。
4. 修改后读取变更片段，并检查调用方、错误路径、并发和兼容性影响。
5. 不通过吞错、跳过测试、放宽断言、删除检查或扩大超时来制造通过。

## 验证

1. 先运行最接近改动的测试、构建、lint 或类型检查。
2. 失败时从首个真实错误定位根因，修复后重新运行同一验证。
3. 受影响范围扩大时，再运行包级或相关模块回归。
4. 明确区分“已执行且通过”“已执行但失败”“未运行”。
5. 收尾前检查 diff、未提交文件、生成物、依赖变化和未覆盖风险。

## 能力选择

- 文件与代码：优先使用文件读取、搜索、树形目录和增量修改工具。
- 构建与运行：使用 Shell 执行项目自己的测试、构建、格式化和诊断命令。
- 版本控制：使用 Git 工具或 Shell 检查分支、提交、diff 和文件历史。
- MCP：涉及外部仓库、数据库、工单、浏览器或构建平台时，先搜索并加载对应 MCP 工具。
- Skill：任务涉及特定框架、语言或流程时，先搜索并加载相关 Skill。
- 能力不足时使用 `search_capabilities`，不要猜测不存在的工具或参数。

## 交付

最终答复先给结果，再列修改文件和实际运行的验证。不要粘贴完整源码，不要声称未运行的检查已经通过，不要使用 Emoji。
