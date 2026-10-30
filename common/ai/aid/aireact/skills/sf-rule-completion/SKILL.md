---
name: sf-rule-completion
metadata:
  display_name_zh-CN: SyntaxFlow 规则美化
description: >
  SyntaxFlow 规则元数据补全（规则美化）技能：只补全 desc/alert 元数据，不修改规则体
  语法与检测逻辑。主路径调用 syntaxflow_rule_completion 工具完成读取、AI 补全、合并与
  落盘，遵循中文为主的元数据标准与 risk 中文预设枚举。
---

# SyntaxFlow 规则美化 (sf-rule-completion)

为 SyntaxFlow 规则补全 desc / alert 元数据（title、desc、solution、message、risk 等），
让规则具备完整、规范的描述信息。

## 1. 铁律（先读）

- **只改元数据**：不修改规则体的 SyntaxFlow 语法与检测逻辑（include / 数据流 / alert 触发条件）。
- **alert 数量不变**：输出 alert 数组条数必须与原文一致，逐条对应。
- **alert name 不变**：与原文变量名一致（含 `$` 前缀与否都保持原样）。
- **level 保留原文**：不要自行输出或调整 level。
- **不编造 rule_id**：rule_id 由工具/合并层生成。

## 2. 标准流程（工具优先，勿套娃）

1. **调用 `syntaxflow_rule_completion` 工具**（一步完成读规则 + AI 补全 + 内存合并 + 落盘）。
   参数 `rule_name`（按名称查规则库）/ `file_path`（本地 .sf 路径）/ `file_content`（规则正文）
   **至少提供其一**；可选 `query`（补全说明）、`source_name`（影响输出文件名）、
   `write_output`（默认 true）。
2. 从返回 JSON 确认 `written=true` 与 `written_path`。工具默认写入
   `~/yakit-projects/code/<基名>-completed.sf`。
3. 仅当用户明确要求**覆盖源路径**时，再用 `write_file` 把 `rule_content` 写回原文件。
4. 收尾可用 `check_syntaxflow_syntax`（经 `require_tool` 加载）对合并结果做语法校验。

禁止绕过工具自行拆步骤：不要在外层先 `read_file` 再手工拼补全结果——工具内部已内置
本技能描述的元数据标准与合并逻辑，手拼容易破坏 alert 对应关系。

## 3. 元数据字段标准

| 字段 | 标准 |
|---|---|
| `title` | 英文，动词+语言+目的，如 `Check Python XXE in Standard Library` |
| `title_zh` | 中文，必填，如 `检测 Python 标准库 XXE 漏洞` |
| `cwe` | CWE 编号纯数字；非漏洞规则可无 |
| `desc` | 不少于 100 字，中文 markdown，写清漏洞原理、触发场景、潜在影响 |
| `solution` | 不少于 100 字，中文 markdown 修复建议；非漏洞规则填 `none` |
| `reference` | 参考资料；无则 `none` |

每个 alert 补全：`name`、`title`、`title_zh`、`desc`、`message`、`risk`、`solution`
（不输出 level，由原文保留）。

## 4. 语言要求（强制）

- `title_zh`、`desc`、`solution`、`message`、`risk` **必须中文**（可保留 XML、XXE、API 等术语）。
- **禁止**整段英文填 desc/solution/risk（不要照抄原文英文描述）。
- 仅 `title` 字段用英文。

## 5. risk 预设值（必须从中选最贴切项）

SQL注入 / 命令注入 / 远程代码执行（RCE） / 服务器端模板注入（SSTI） / 路径遍历 /
任意文件上传 / XML外部实体（XXE） / 跨站脚本（XSS） / 服务端请求伪造（SSRF） /
敏感信息泄露 / 不安全反序列化 / 软件成分分析（SCA） / 不安全加密 / 硬编码凭据 /
资源泄漏 / 缓冲区溢出 / 拒绝服务（DoS） / 信息（仅用于库/辅助规则）

同义写法（如「路径穿越」「目录遍历」）可接受，但**禁止** `file-upload`、`xxe`、`sca`
等纯英文 slug。

## 6. 边界说明

本技能与蓝图 `sf_rule_completion`（SyntaxFlow 规则补全）共用同一套标准与叶子工具：
在通用对话模式下直接按本文流程调用工具即可；经蓝图入口（`require_ai_blueprint`）调用时
由蓝图协调器负责子任务编排，无需重复套用本文流程。
