# aireact 角色 Prompt 缓存优化方案

## 1. 角色清单（R1~R10）

| 编号 | 角色 | 生成方法 | 用途 | 走共享 prefix? |
|------|------|---------|------|:---:|
| R1 | ReAct 主循环 | `AssembleLoopPrompt` | 核心决策循环：选工具/直接回答/请求蓝图/规划/验证 | ✅ |
| R2 | 工具参数生成 | `GenerateToolParamsPromptWithMeta` | 为选定的工具生成调用参数 | ✅ |
| R3 | 工具参数重生成 | `GenerateReGenerateToolParamsPromptWithMeta` | 参数失败后带着旧参数重新生成 | ✅ |
| R4 | 工具重选择 | `GenerateToolReSelectPrompt` | 旧工具不合适时重新选工具 | ✅ |
| R5 | 蓝图参数生成 | `GenerateAIBlueprintForgeParamsPromptEx` | 为 AI Blueprint 生成启动参数 | ✅ |
| R6 | 蓝图切换 | `GenerateChangeAIBlueprintPrompt` | 当前蓝图不合适时重新选蓝图 | ✅ |
| R7 | 验证/满意度判定 | `GenerateVerificationPrompt` | 评估子任务是否达成，产出 user_satisfied + evidence | ✅ |
| R8 | 直接回答 | `GenerateDirectlyAnswerPrompt` | 无需工具时直接给出最终回答 | ✅ |
| R9 | 工具间隔审查 | `GenerateIntervalReviewPromptWithContext` | 长时间工具调用的进度审查（continue/cancel） | ❌ |
| R10 | 自我反思 | `buildReflectionPrompt` | SPIN 死循环检测与打破建议 | ❌ |

> 另有 R11 对话标题生成（`GenerateRequireConversationTitlePrompt`），频率极低，不走 prefix，不参与缓存优化。

## 2. 真实运行调用次数

以下数据来自一次渗透测试 Agent 运行的 23 条 intelligent 模型 prompt dump
（`/Users/rookie/yakit-projects/developing/`）：

| 编号 | 角色 | 调用次数 | 占比 | 说明 |
|------|------|:---:|:---:|------|
| R1 | ReAct 主循环 | 13 | 56.5% | 最高频，每轮决策都调用 |
| R7 | 验证/满意度判定 | 5 | 21.7% | 每步工具调用后验证 |
| R2 | 工具参数生成 | 4 | 17.4% | 每次工具调用生成参数 |
| R8 | 直接回答 | 1 | 4.3% | 任务结束时直接回答 |
| R3 | 工具参数重生成 | 0 | 0% | 本次运行未触发参数失败 |
| R4 | 工具重选择 | 0 | 0% | 本次运行未触发工具切换 |
| R5 | 蓝图参数生成 | 0 | 0% | 本次运行未使用蓝图 |
| R6 | 蓝图切换 | 0 | 0% | 同上 |
| R9 | 工具间隔审查 | 0 | 0% | 走 lightweight 模型，不在 dump 中 |
| R10 | 自我反思 | 0 | 0% | 走 lightweight 模型，不在 dump 中 |
| **合计** | | **23** | 100% | |

**关键观察**：
- R1↔R7↔R2 是最高频的交替组合（13+5+4 = 22/23 = 95.7%）
- 实际切换序列：`R1 → R2 → R1 → R2 → R1 → R7 → R1 → R2 → R7 → R1 → R1 → R7 → R1 → R7 → R1 → ... → R8`

## 3. 已完成的变更

### 3.1 semi-dynamic-1 段统一 SkillsContext

5 个子角色（R4/R5/R6/R7/R8）的 `SkillsContext` 从 `""` 改为 `pm.renderSkillsContextForPrompt()`，R7 删除了 `PromotedSemiDynamic1 = ""` 和 `PromotedTimelineOpen = ""`。所有 R1~R8 的 semi-1 段渲染产物一致。

### 3.2 semi-dynamic-2 段 Schema 移到末尾

`semi_dynamic_2_section.txt` 模板内字段顺序从 `TaskInstruction → Schema → OutputExample → AutoLoadedSkills` 改为 `TaskInstruction → OutputExample → AutoLoadedSkills → Schema`。

Schema（随工具/角色变）被后置到 semi-2 段末尾。Schema 之前的 `TaskInstruction` + `OutputExample` + `AutoLoadedSkills` 部分如果跨角色一致则命中 prefix cache，只有 Schema 部分变化导致 miss。

### 3.3 P0-A：合并参数生成类角色（R2 + R3 + R5 → 统一参数生成）

R2/R3/R5 的 instruction 都讲"为 X 生成参数"，逻辑 80~90% 重叠。合并为一份 instruction + 一份 output_example + 一份 dynamic 模板。

**变更内容**：

| 文件 | 变更 |
|------|------|
| `tool-params/instruction.txt` | 泛化标题为 "Parameter Generation"，覆盖工具和蓝图；新增重生成条件规则；合并 3 种 Response Pattern（Simple JSON / AITAG Hybrid / AI Blueprint） |
| `tool-params/output_example.txt` | 新增，合并 R3 和 R5 的 example（3 个 pattern） |
| `tool-params/dynamic.txt` | 统一模板，字段沿用 ToolName/ToolDescription/ToolUsage；条件分支 `{{ if .IsBlueprint }}` 区分蓝图上下文；支持 `{{ if .OldParams }}` 旧参数块和 `{{ if .ExtraPrompt }}` 额外指令 |
| `prompts.go` R2 | OutputExample 从 `""` 改为 `toolParamsOutputExampleText` |
| `prompts.go` R3 | TaskInstruction/OutputExample/dynamic 全部改用 toolParams 系列 |
| `prompts.go` R5 | TaskInstruction/OutputExample/dynamic 全部改用 toolParams 系列；字段映射 `ToolName=ins.ForgeName`、`ToolDescription=ins.Description`；新增 `IsBlueprint=true` |
| 删除 6 个文件 | `tool/wrong-params_*` (R3) + `tool-params/blueprint-params_*` (R5) |
| 删除 6 个 embed | 对应的 `//go:embed` 声明和变量 |

**semi-2 hash 变体**：3 → 1

**命中提升**：
- R2 调不同工具间：instruction + example 一致 → 命中到 example 末尾（只有 schema miss）
- R2→R3 切换（同工具）：instruction + example + schema 全部一致 → 整个 semi-2 命中
- R2→R5 切换（工具↔蓝图）：instruction + example 一致 → 命中到 example 末尾

## 4. 待实施的方案

### 4.1 P0-B：合并重选择类角色（R4 + R6）

R4（工具重选择）和 R6（蓝图切换）逻辑 85% 重叠（"旧的不合适 → 重新选择"）。合并后 R4↔R6 切换时整个 semi-2 命中。

**计划**：
- 新建 `prompts/capability-reselect/`（统一 instruction + example + dynamic）
- 合并 schema：`getCapabilityReSelectSchema()`，enum 含 `require-tool` + `change-ai-blueprint` + `abandon`
- R4/R6 改用统一模板
- 删除 6 个文件 + 6 个 embed 声明

### 4.2 P1：R9/R10 改走共享 prefix

R9/R10 完全脱离共享 prefix，与主循环切换时 0 命中。改走 `preparePromptPrefixMaterials` 路径后复用 high-static + frozen + semi-1，消除 0 命中和 prefix_misalign 度量干扰。

## 5. 合并后的角色全景

| 编号 | 角色 | semi-2 内容 | semi-2 命中条件 |
|------|------|------------|----------------|
| R1 | 主循环 | ReAct instruction + example + schema(全 action) | 同 R1 连续调用 |
| R2 | 参数生成（含 R3/R5） | 统一 instruction + 统一 example + schema(工具/蓝图) | 同 instruction+example → 命中到 example；同工具 → 全命中 |
| R3 | 能力重选择（含 R4/R6） | 统一 instruction + 统一 example + 统一 schema | R4↔R6 全命中 |
| R4 | 验证/满意度 | 验证 instruction + example + schema | 同 R4 连续调用 |
| R5 | 直接回答 | 直答 instruction + example + schema | 同 R5 连续调用 |
| R6 | 间隔审查 | 审查 instruction + example + schema | 改走共享 prefix 后命中 HS+FB+S1 |
| R7 | 自我反思 | 反思 instruction + example + schema | 改走共享 prefix 后命中 HS+FB+S1 |
