# ReAct 提示词职责

主循环负责工具选择和显式参数生成。工具的 Schema 通过 `require_tool` 或工具能力加载进入缓存，加载本身不执行工具。单次与参数组调用均使用 `directly_call_tool`。

审核的 `wrong_tool` / `wrong_params` 结束当前提案，反馈和 Schema 进入 Timeline，由所属主循环或子循环重新决策；不再发起工具参数生成或工具重选择辅助请求。

| 职责 | 入口 | 模板 |
| --- | --- | --- |
| 主循环决策 | `AssembleLoopPrompt` | 当前 loop 模板 |
| Forge 参数生成及修改 | `GenerateAIBlueprintForgeParamsPromptEx` | `forge-params/` |
| Forge 切换 | `GenerateChangeAIBlueprintPrompt` | `blueprint/` |
| 验证 | `GenerateVerificationPrompt` | verification 模板 |
| 最终答复 | `GenerateDirectlyAnswerPrompt` | directly-answer 模板 |
| 运行中工具审查 | `GenerateIntervalReviewPromptWithContext` | interval-review 模板 |

工具参数生成的独立文本流和 Function Call 协议已删除。Forge 参数生成仍然保留，使用独立模板；共享 Timeline、evidence、用户输入及缓存分层由现有 prompt assembly 提供。
