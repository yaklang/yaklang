# prompts/loop

ReAct 主循环 prompt 的 5 段切片模板, 按"稳定性分层"渲染, 与 aicache
prefix-cache 边界对齐.

| 文件 | 段名 | 稳定性 | 缓存边界 |
| --- | --- | --- | --- |
| `high_static_section.txt` | high-static | 跨 caller / 跨 turn 字节恒定 | `<\|AI_CACHE_SYSTEM_high-static\|>` |
| `frozen_block_section.txt` | frozen-block | Tool/Forge inventory + Timeline frozen 前缀 | `<\|AI_CACHE_FROZEN_semi-dynamic\|>` |
| `semi_dynamic_section.txt` | semi-dynamic | Skills + Schema + OutputExample + Persistent + RecentToolsCache | `<\|AI_CACHE_SEMI_semi\|>` |
| `timeline_open_section.txt` | timeline-open | Timeline 末桶 + Workspace + UserHistory + PlanContext | 缓存边界外 (易变尾段) |
| `dynamic_section.txt` | dynamic | UserQuery + AutoContext + ExtraCapabilities + Reflection + InjectedMemory | 缓存边界外 (本轮独有) |

具体段间字节稳定性约束见
[`common/ai/aid/aicache/LESSONS_LEARNED.md`](../../../aicache/LESSONS_LEARNED.md).

## 修改 high-static 段时的硬约束

`high_static_section.txt` 是跨所有 caller 字节恒定的"系统级"段, 进入
`<|AI_CACHE_SYSTEM_high-static|>` 缓存边界, 也是 ReAct 主循环每一轮
prompt 的固定开头. 修改该文件时**必须**满足:

1. **不写入 caller-specific 字段**. 任何随 forge / loop / pe-task 切换的
   内容 (OutputExample / TaskInstruction / Schema / SkillsContext) 都
   不放进本段, 否则 `AI_CACHE_SYSTEM` hash 漂移, 上游 prefix cache 直接
   失效. 这类字段应放 `semi_dynamic_section.txt`.
2. **散文不出现 SCHEMA enum / JSON key 的字面 snake_case 形式**. 例如
   `directly_answer` / `require_tool` / `tool_compose` /
   `require_ai_blueprint` / `finish_exploration` /
   `request_plan_and_execution` / `output_facts` / `loading_skills` /
   `load_capability` / `enter_focus_mode` / `ask_for_clarification` /
   `answer_payload` / `tool_require_payload` / `@action` 等都禁止写在
   散文里. 测试基础设施 (`common/ai/aid/test/prompt_matchers_test.go`,
   `common/ai/aid/aireact/test_prompt_matchers_test.go` 等 30+ 处)
   用朴素 `strings.Contains` 子串匹配区分不同 prompt 类型, 这套契约
   假设这些字面量**只**出现在真正暴露该 enum 的 schema 块里. 一旦
   high-static 散文出现这些字面量, mock 全部错位, 表层症状是"原本无关的
   mock 测试集体失败 (`expected token X not found in ""`)".
   - **替代写法**: 散文统一用 kebab-case (`directly-answer` /
     `require-tool` / `tool-compose` / `finish-exploration` 等), 模板内
     "命名约定 (Action Identifier Convention)" 一节会显式告诉模型实际
     `@action` 字面以本轮 SCHEMA enum / const 为准, 不要把 kebab-case
     形式照抄进 JSON.
3. **不写入形如 `<|TAG_NAME_<NONCE>|>` 的 AITAG 占位字面量**.
   `aicommon.ExtractPromptNonce` 会把其中的 `<NONCE` 字面误识别为合法
   nonce, 让基于 nonce 的 retry / 解析链路串味. 描述 AITAG 形态时改用
   纯文字解释, 或者用 kebab-case 替代 (`<|CURRENT-TASK-...|>`).
4. **token 数 ≥ 1500**. 来自 dashscope / qwen 实测的"显式 prefix cache
   创建最小窗口". 低于 1500 tokens 上游放弃缓存, 即便 hash 稳定也无法
   转化为真实计费节省. 该约束在 `prompt_loop_materials_test.go` 有回归
   断言.

## 改完后的自检流程

```bash
# 1. 主路径单元测试
go test -count=1 -timeout=300s ./common/ai/aid/test/...
go test -count=1 -timeout=300s ./common/ai/aid/aireact/...
go test -count=1 -timeout=300s ./common/ai/aid/aireact/reactloops/reactloopstests/...

# 2. token 数 / hash 稳定性回归
go test -count=1 -run TestPromptManager -timeout=120s ./common/ai/aid/aireact/

# 3. (可选) cachebench 长跑, 验证 intelligent 路径 token_hit ≥ 50%, 见
#    common/ai/aid/aicache/LESSONS_LEARNED.md 第 2 节验收标准
```

如果第 1 步出现"原本无关的 mock 测试集体失败"且失败信息形如
`expected token X not found in ""` / `Should be true`, 90% 是上面第 2 条
"snake_case 字面量泄漏到散文"被违反.
