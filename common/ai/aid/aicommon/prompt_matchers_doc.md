# Prompt Matcher 使用指南

## 背景

多角色 AI Agent 系统在测试时，mock AI 回调需要根据收到的 prompt 判断当前是哪个角色（R1 主循环决策 / R2 参数生成 / R3 工具参数重生成 / R5 蓝图参数生成 / R6 切换蓝图 / 意图识别 / 满意度审查 等），然后返回对应的 canned response。

之前各测试包（`common/ai/aid/test`、`common/ai/aid/aireact`、`reactloopstests`）各自维护一套 `MatchAllOfSubString` 关键词匹配，存在两个核心问题：

1. **匹配误命中**：R1 instruction 散文里会出现 R2 的标记（如 `# Tool Context`、`<|TOOL_SCHEMA_...|>` 在反引号散文中），导致 R1 prompt 被误判为 R2。
2. **维护分散**：同样的判定逻辑在三个包里各写一遍，prompt 改动后容易遗漏同步。

## 解决方案：共享 prompt_matchers

所有判定函数已统一迁移到 `common/ai/aid/aicommon/prompt_matchers.go`（非 test 文件，导出函数），各测试包通过薄包装调用。

### 核心函数

| 函数 | 用途 | 判定依据 |
|------|------|----------|
| `IsPrimaryDecisionPrompt(prompt)` | R1 主循环决策 | `<\|AI_CACHE_SYSTEM_high-static\|>` + `<\|PROMPT_SECTION_dynamic_` + `"require_tool"` + `"tool_require_payload"`，且排除 R2 |
| `IsToolParamGenPrompt(prompt)` | R2/R3/R5 参数生成（通用） | `# Parameter Generation Task` 标题（动态段独有）或旧路径 fallback |
| `IsToolParamGenPromptForTool(prompt, toolName)` | R2 工具参数生成 | 先过 `IsToolParamGenPrompt`，再排除蓝图（无 `Blueprint Description:`） |
| `IsToolParamGenPromptForBlueprint(prompt, forgeName)` | R5 蓝图参数生成 | `You need to generate parameters for the AI Blueprint` 或旧路径 `Blueprint Schema:` + `Blueprint Description:` |
| `IsToolParamGenPromptWithOldParams(prompt)` | R3/R5 参数重生成 | `<\|PARAM_REGENERATION_CONTEXT\|>` 标签（动态段独有） |
| `IsChangeBlueprintPrompt(prompt)` | R6 切换蓝图 | `# Change AI Blueprint Task` 标题或 `change-ai-blueprint` action |
| `IsIntentEnrichmentPrompt(prompt)` | 意图识别循环 | `意图识别与上下文增强系统` 或 `Intent Recognition` + `Context Enrichment`（**不**匹配 `finalize_enrichment` 等可泄漏到主循环的关键词） |
| `IsVerifySatisfactionPrompt(prompt)` | 满意度审查 | `verify-satisfaction` + `user_satisfied` 或 `任务策略师` |
| `IsDirectAnswerPrompt(prompt)` | 直接回答 | `FINAL_ANSWER` 或 `directly_answer` + `answer_payload` |
| `IsToolCallReasonLiteForgePrompt(prompt)` | LiteForge tool-call-reason | `"tool-call-reason"` |

### 各包使用方式

**`common/ai/aid/aireact` 包内测试**：通过 `test_prompt_matchers_test.go` 中的薄包装函数（小写开头），如 `isPrimaryDecisionPrompt`、`isToolParamGenPromptForTool` 等，直接调用。

**`common/ai/aid/test` 包内测试**：通过 `prompt_matchers_test.go` 中的薄包装函数，如 `isNextActionDecisionPrompt`、`isToolParamGenerationPrompt` 等。

**`reactloopstests` 包内测试**：直接调用 `aicommon.IsPrimaryDecisionPrompt`、`aicommon.IsToolParamGenPromptForTool` 等导出函数。

## 改进测试的步骤

### 1. 用共享函数替换内联 MatchAllOfSubString

**之前**（容易误命中）：
```go
if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
    // 返回主循环响应
}
if utils.MatchAllOfSubString(prompt, "# Tool Context") {
    // 返回参数生成响应
}
```

**之后**（精确匹配）：
```go
if isPrimaryDecisionPrompt(prompt) {
    // 返回主循环响应
}
if isToolParamGenPromptForTool(prompt, "") {
    // 返回参数生成响应
}
```

### 2. 注意 mock 回调中的判定顺序

由于 R2 复用 R1 instruction，R1 prompt 里会包含 R2 的标记散文。判定顺序应遵循：

1. **最特化的角色优先**：LiteForge、意图识别、切换蓝图等有唯一标记的角色先判。
2. **R2/R3/R5 参数生成**：在 R1 之前判，因为 `IsPrimaryDecisionPrompt` 内部已排除 R2。
3. **R1 主循环决策**：兜底。
4. **directly_answer / summary**：最后。

典型顺序：
```
1. intent-finalize-summary (LiteForge)
2. isIntentEnrichmentPrompt
3. isChangeBlueprintPrompt
4. isToolParamGenPromptForTool / isToolParamGenPromptForBlueprint
5. isPrimaryDecisionPrompt
6. isVerifySatisfactionPrompt
7. isDirectAnswerPrompt
8. summary
9. fallback
```

### 3. 添加新的 prompt 标记时同步更新 matcher

当在动态段模板中添加新的标志性标题或标签时：

1. 在动态段模板中添加**唯一标记**（如 `# Parameter Generation Task`、`<|PARAM_REGENERATION_CONTEXT|>`），确保该标记不出现在 instruction 散文中。
2. 在 `aicommon/prompt_matchers.go` 的对应函数中添加对新标记的匹配。
3. 在 `aireact/test_prompt_matchers_test.go` 和 `test/prompt_matchers_test.go` 中添加薄包装（如需）。
4. 更新测试文件中的内联匹配为共享函数调用。

## 散文污染教训

**禁止在静态 prompt 散文中出现具体 action 字面量**，无论 snake_case 还是 kebab-case。原因：

- `directly_answer`、`require_tool` 等关键词会出现在 R1 schema 块中，如果散文里也写，`MatchAllOfSubString` 无法区分。
- R1 instruction 中提到 `# Tool Context`（反引号包裹）来指导 R2 识别，但动态段中 `# Tool Context` 是 markdown 标题（无反引号），二者字节不同但子串匹配会混淆。
- **解决方案**：用动态段独有的标题/标签（如 `# Parameter Generation Task`）做判定，而不是用散文中也会出现的标记。

## 文件清单

| 文件 | 作用 |
|------|------|
| `common/ai/aid/aicommon/prompt_matchers.go` | 共享判定函数（导出） |
| `common/ai/aid/aireact/test_prompt_matchers_test.go` | aireact 包薄包装 |
| `common/ai/aid/test/prompt_matchers_test.go` | test 包薄包装 |
| `common/ai/aid/aireact/prompts/tool-params/dynamic.txt` | R2 动态段（含 `# Parameter Generation Task` 和 `<\|PARAM_REGENERATION_CONTEXT\|>` 标记） |
| `common/ai/aid/aireact/prompts/change-blueprint/instruction.txt` | R6 instruction（含 `# Change AI Blueprint Task` 标题） |
