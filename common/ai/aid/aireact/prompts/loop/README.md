# prompts/loop

本目录仅保留 **仍由 `aireact` 包 `//go:embed` 引用** 的模板：

| 文件 | 用途 |
| --- | --- |
| `timeline_section.txt` | 兼容路径：合并 frozen + open 的 legacy timeline 段渲染（`PromptPrefixAssemblyResult.Timeline`） |
| `dynamic_section.txt` | ReAct 主循环 **dynamic** 段（UserQuery、AutoContext、ExtraCapabilities 等） |

**high-static / frozen-block / semi-dynamic-1 / semi-dynamic-2 / timeline-open** 五段中与 prefix 缓存对齐的正文已统一放在  
`common/ai/aid/aicommon/prompts/shared/`（`high_static_section.txt`、`frozen_block_section.txt`、`semi_dynamic_1_section.txt`、`semi_dynamic_2_section.txt`、`timeline_open_section.txt`），由 `aicommon.PromptPrefixBuilder` 嵌入，勿在本目录重复维护副本。

段间字节稳定性与 aicache 边界约定见  
[`common/ai/aid/aicache/LESSONS_LEARNED.md`](../../../aicache/LESSONS_LEARNED.md)。

## 修改 high-static 时的硬约束

编辑 **`aicommon/prompts/shared/high_static_section.txt`** 时须遵守与原 loop 版相同的约束（caller 无关、散文不写 schema enum 字面、不写 `<|TAG_<NONCE>|>` 形态、token 窗口等）。改完后可用：

```bash
grep -nE "require[-_]tool|directly[-_]answer|tool[-_]compose|finish[-_]exploration|enter[-_]focus[-_]mode|load[-_]capability|loading[-_]skills|load[-_]skill[-_]resources|change[-_]skill[-_]view[-_]offset|ask[-_]for[-_]clarification|abandon|@action|answer_payload|tool_require_payload" common/ai/aid/aicommon/prompts/shared/high_static_section.txt
```

正常应 **无输出**。

## 自检

```bash
go test -count=1 -timeout=300s ./common/ai/aid/aicommon/...
go test -count=1 -timeout=300s ./common/ai/aid/aireact/...
```
