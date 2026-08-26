# AI Status 事件规范

Status 用来告诉用户助手正在做什么，不承担调试日志职责。界面默认展示中文；英文和更丰富的展示信息作为可选字段提供。

## 兼容协议

旧接口保持原样：

```go
emitter.EmitStatus(key, value)
```

新代码优先使用：

```go
emitter.EmitStatusI18n(
    key,
    "正在准备调用 2 个工具：读取文件、搜索代码",
    "Preparing 2 tools: Read File and Search Code",
    aicommon.WithStatusCode("tool.preparing"),
    aicommon.WithStatusProgress(0, 2, "tool"),
    aicommon.WithStatusTools(tools...),
)
```

对应事件是旧结构的严格超集：

```json
{
  "key": "react",
  "value": "正在准备调用 2 个工具：读取文件、搜索代码",
  "value_i18n": {
    "zh": "正在准备调用 2 个工具：读取文件、搜索代码",
    "en": "Preparing 2 tools: Read File and Search Code"
  },
  "code": "tool.preparing",
  "state": "running",
  "detail": "先了解相关实现，再继续处理",
  "detail_i18n": {
    "zh": "先了解相关实现，再继续处理",
    "en": "Reviewing the relevant implementation before continuing"
  },
  "progress": { "current": 0, "total": 2, "unit": "tool" },
  "tools": [
    { "name": "read_file", "display_name": "读取文件", "state": "running" },
    { "name": "grep", "display_name": "搜索代码", "state": "running" }
  ]
}
```

| 组合 | 行为 |
| --- | --- |
| 旧后端 + 旧前端 | 继续读取 `key`、`value` |
| 新后端 + 旧前端 | `value` 仍是字符串，默认中文 |
| 旧后端 + 新前端 | 缺少扩展字段时退回 `value` |
| 新后端 + 新前端 | 按语言展示文案，并可展示详情、进度和工具 |

`state` 可选值为 `running`、`waiting`、`recovering`、`success`、`warning`、`error`。它描述当前提示的呈现状态，不替代任务自身的生命周期状态。

## 文案迁移

文案描述用户能理解的意图和结果；解析器、流、字段名、事务、DAG 等实现细节保留在日志中。

| 原表达 | 优化后 | 说明 |
| --- | --- | --- |
| 解析 AI 响应中 | 正在梳理思路 | 隐去协议解析过程 |
| 等待网络响应 | 正在查找最新资料 | 表达用户可感知的目的 |
| 初始化 ReAct 循环 | 正在准备这次任务 | 隐去运行框架 |
| Verifier 验证中 | 正在确认关键细节 | 使用自然语言 |
| 事务执行失败 | 暂时没能完成这一步 | 原始错误进入日志或时间线 |
| Action 解析失败，重试中 | 刚才的信息不够完整，正在重新整理 | 表达可恢复状态 |
| 正在执行工具 | 正在准备使用「读取文件」 | 展示真实工具名 |
| 批量工具调用中 | 正在准备调用 3 个工具：读取文件、搜索代码、查看目录 | 展示数量、名称和进度 |
| 批量工具调用完成 | 3 个工具中有 2 个已完成，正在整理可用结果 | 展示实际成功数及逐工具状态 |
| Task 2 执行中 | 正在处理「分析登录逻辑」 | 展示任务名称，不展示内部序号 |
| Phase 2 扫描中 | 正在排查潜在风险 | 隐去内部阶段编号 |
| 正在解析图片附件 | 正在理解第 2/5 张图片 | 提供真实进度 |
| 正在生成最终结果 | 正在组织回答 | 更接近助手语气 |
| 处理中，请稍候 | 这一步比预期久一些，仍在继续 | 对等待给出明确反馈 |

## 编写约定

- `value` 优先使用简短、可独立展示的中文，不拼接双语。
- `value_i18n` 承载语言版本；中文缺失时才回退英文。
- `code` 使用稳定的机器标识，前端不要依赖文案判断状态。
- 只有可计算时才提供 `progress.total`，不要用虚构百分比安抚用户。
- 工具状态展示真实名称，标题最多列出三个，其余用数量概括。
- 批量工具只在准备和批次返回时更新状态，避免为每个子调用重复刷屏。
- Status 不携带工具参数、提示词、原始响应和敏感错误信息。
- `detail` 只补充“为什么做”或“接下来做什么”，不要复制标题。
