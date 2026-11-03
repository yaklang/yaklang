---
name: sf-rule-completion
metadata:
  display_name_zh-CN: SyntaxFlow 规则美化
description: >
  端到端补全 SyntaxFlow 规则的 desc/alert 元数据（规则美化）：调用 syntaxflow_rule_completion
  工具读规则、AI 补全、合并并落盘，再用 check-syntaxflow-syntax 校验合并结果。当用户要求
  美化规则、补全 desc/alert、给规则加描述信息时加载。只改元数据，不改检测逻辑。
---

# SyntaxFlow 规则美化 (sf-rule-completion)

把一条只有检测逻辑的 SyntaxFlow 规则，补全为元数据完整、可直接入库/展示的规则。
本技能在通用对话模式下复刻 `sf_rule_completion` 蓝图的作业流程。

## 0. 验收标准

1. **元数据完整**：第一个 desc 块与每个 alert 都补齐必填字段（见第 3 节）。
2. **结构不变**：alert 条数与 name 与原文逐一对应；level 保持原文。
3. **语法通过**：合并结果经 `check-syntaxflow-syntax` 校验 `passed=true`。
4. **落盘成功**：工具返回 `written=true` 且 `written_path` 可读。

四项全满足才算完成。缺任何一项都必须继续，不得提前宣称完成。

## 1. 铁律

- **只改元数据**：禁止修改 include、数据流、alert 触发条件。
- **alert 数量不变**，**alert name 不变**（`$` 前缀保持原样）。
- **level 保留原文**，不要自行输出或调整。
- **不编造 rule_id**：由工具/合并层生成。
- **中文为主**：`title_zh`、`desc`、`solution`、`message`、`risk` 必须中文；仅 `title` 用英文。

## 2. 工作流

### 2.1 定位输入规则

三种来源，至少确定其一：
- 规则库中的规则名 → `rule_name`
- 本地 `.sf` 文件 → `file_path`（先用 `read_file` 确认文件存在且内容完整）
- 规则正文 → `file_content`

### 2.2 调用 `syntaxflow_rule_completion`（核心步骤）

该工具在默认工具清单中可见，直接执行，**不需要** require_tool：

```json
{
  "@action": "directly_call_tool",
  "directly_call_tool_name": "syntaxflow_rule_completion",
  "directly_call_tool_params": {
    "file_path": "<规则文件绝对路径>",
    "write_output": true
  }
}
```

可选参数：`rule_name`、`file_content`、`query`（说明）、`source_name`（影响输出文件名）。

工具内部完成：读规则 → AI 补全 desc/alert → 内存合并 → 写盘。
默认输出到 `~/yakit-projects/code/<基名>-completed.sf`。

### 2.3 核对返回

从返回 JSON 确认：
- `written=true`，记录 `written_path`
- `rule_content` 长度合理（不应明显短于原文）
- `rule_name`、`rule_id` 已生成

### 2.4 语法校验（必做）

`check-syntaxflow-syntax` 是 scenario 工具（默认清单不展示），按**两个独立 action** 调用，
禁止把 `require_tool` 和 `directly_call_tool` 字段混在同一个 action 里：

```json
{"@action": "require_tool", "require_tool_payload": "check-syntaxflow-syntax", "human_readable_thought": "加载 SyntaxFlow 语法检查工具"}
```

```json
{"@action": "directly_call_tool", "directly_call_tool_name": "check-syntaxflow-syntax", "directly_call_tool_params": {"path": "<written_path>"}}
```

`passed=false` 时按 `errors` 修复——但注意：**只能修元数据部分**，如果错误出在规则体，
说明补全被写坏了，应回到 2.2 重新补全或报告问题，不要手改检测逻辑。

### 2.5 交付

向用户交付：`written_path` + 关键改动摘要（补了哪些字段、多少条 alert）。
仅当用户要求覆盖原文件时，才用 `write_file` 把 `rule_content` 写回源路径。

## 3. 元数据标准

| 字段 | 标准 |
|---|---|
| `title` | 英文，动词+语言+目的，如 `Check Python XXE in Standard Library` |
| `title_zh` | 中文必填，如 `检测 Python 标准库 XXE 漏洞` |
| `cwe` | CWE 编号纯数字；非漏洞规则可无 |
| `desc` | ≥100 字，中文 markdown，写清漏洞原理、触发场景、潜在影响 |
| `solution` | ≥100 字，中文 markdown 修复建议；非漏洞规则填 `none` |
| `reference` | 参考资料；无则 `none` |

每个 alert 补齐：`name`、`title`、`title_zh`、`desc`、`message`、`risk`、`solution`
（**不含 level**）。

## 4. risk 预设值（从中选最贴切项）

SQL注入 / 命令注入 / 远程代码执行（RCE） / 服务器端模板注入（SSTI） / 路径遍历 /
任意文件上传 / XML外部实体（XXE） / 跨站脚本（XSS） / 服务端请求伪造（SSRF） /
敏感信息泄露 / 不安全反序列化 / 软件成分分析（SCA） / 不安全加密 / 硬编码凭据 /
资源泄漏 / 缓冲区溢出 / 拒绝服务（DoS） / 信息（仅用于库/辅助规则）

同义写法（「路径穿越」「目录遍历」）可接受；**禁止** `file-upload`、`xxe`、`sca`
等纯英文 slug。

## 5. 迭代与失败处理

- 工具返回 `written=false` 或报错：检查 `file_path` 是否可读、规则是否为空。
- 语法校验失败且错误在元数据：重新调用工具（工具会重新补全），不要手工拼接。
- 语法校验失败且错误在规则体：说明原文本身有问题，向用户报告，不要擅自修改检测逻辑。
- **禁止绕过工具手写合并**：不要自己 read_file 后拼 desc/alert 再写入，这最容易破坏
  alert 对应关系；工具内置了本技能的全部标准与合并逻辑。

## 6. 与专注模式/蓝图的差异

| 能力 | 本技能（通用模式） | `sf_rule_completion` 蓝图 |
|---|---|---|
| 补全执行 | 你直接调用叶子工具 | 蓝图协调器编排子任务后调用同一工具 |
| 语法校验 | 你显式调用 `check-syntaxflow-syntax` | 由流程保证 |
| 结构化结果 Action | 无（直接读工具返回 JSON） | Result 阶段产出 `sf_rule_completion_result` |

两者共用同一叶子工具与同一套标准，产出物一致；差异仅在编排层。
