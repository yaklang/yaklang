# Prompt Matcher 使用指南

测试 mock 使用 `prompt_matchers.go` 区分主决策、Forge 参数生成、验证、答复与其它辅助请求。应匹配当前动态上下文中的目标，避免工具列表或历史内容造成误命中。

工具自动构参和工具重选择辅助链路已删除：主循环或子循环直接生成参数，审核反馈通过 Timeline 返回所属循环。工具相关 mock 不再提供旧 `call-tool` 构参响应；遇到这类请求应失败。

| Matcher | 用途 |
| --- | --- |
| `IsPrimaryDecisionPrompt` | 主循环提示词；支持统一 `require_tool_payload` 和旧单项字段兼容 |
| `IsToolParamGenPromptForBlueprint` | Forge 启动参数与参数修改，模板在 `forge-params/` |
| `IsToolParamGenerationPrompt` / `IsToolParamGenPromptForTool` | 保留用于 legacy fixture 及检测已淘汰工具构参请求，不表示现代运行时支持此流程 |
| `IsVerifySatisfactionPrompt` | 满意度验证 |

先识别专属辅助角色，再匹配主循环。修改提示词时，不要在通用静态说明中加入其它角色的专属协议标题。
