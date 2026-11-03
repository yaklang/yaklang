---
name: syntaxflow-rule
metadata:
  display_name_zh-CN: SyntaxFlow 规则编写
description: >
  端到端编写 SyntaxFlow 漏洞检测规则：查 include 库结构、读写 .sf 文件、语法校验，
  以及用 sample_code 正例自检复刻专注模式的验收闭环。当用户要求生成/修改 SyntaxFlow
  规则、写 sf 检测规则、根据漏洞样例写规则时加载。无样例时语法通过即验收；有样例时
  必须 matched=true 才算完成。
---

# SyntaxFlow 规则编写 (syntaxflow-rule)

本技能提供与「编写 SyntaxFlow 规则」专注模式**等价的能力闭环**，在通用对话模式下按以下
工作流执行。核心差异只有一个：本技能靠你自己依序调用工具，专注模式由系统自动拦截校验。

## 0. 验收标准（先明确，避免自欺）

| 场景 | 完成条件 |
|---|---|
| 用户未提供漏洞样例 | `check-syntaxflow-syntax` 语法通过（`passed=true`）即可交付 |
| 用户提供了漏洞样例 | 语法通过 **且** 正例自检 `matched=true` |

**有样例却未 matched 时，禁止声称任务完成。** 未通过就继续迭代，不得跳过自检交付。

## 1. 工作流（按序执行，不要跳步）

```
1) 识别 sink / source
2) 用 syntaxflowdoc_search / syntaxflowdoc_builtin_lib_details 查库与语法
3) 写规则到 .sf 文件
4) 语法校验（check-syntaxflow-syntax）
5) 有样例 → 正例自检（同一工具，加 sample_code+language）
6) 失败 → 按诊断迭代；成功 → 交付
```

### 1.1 识别 sink 与 source

从用户样例/需求确认：sink 危险调用点（`exec`、`Query`、`Parse`、`Execute`...）、
source 用户输入来源（`c.Query`、`r.URL.Query`、`FormValue`...）。先在纸上明确这两端再写规则。

### 1.2 查 include 库与语法（使用全局文档工具）

`syntaxflowdoc_search` 和 `syntaxflowdoc_builtin_lib_details` 是全局 scenario 工具（默认清单不展示，
但可直接调用，参数见下），查询的是编译进引擎的 SyntaxFlowDoc 快照，**与专注模式看到的是同一份数据**。
不要再去磁盘搜索 `.sf` 库文件（内置库不落盘）。

**先搜索**（确认真实存在的 native call / include 库名 / 语法关键字，禁止臆造）：

```json
{"@action": "directly_call_tool", "directly_call_tool_name": "syntaxflowdoc_search", "directly_call_tool_params": {"query": "golang-gin-context", "category": "builtin_lib", "limit": 10}}
```

`category` 可选：`native_call` / `operator` / `opcode` / `desc_key` / `builtin_lib` / `syntax`。

**搜索技巧**：优先用**完整库名/符号名**（`golang-gin-context`、`QueryContext`），而不是
`gin`、`sql` 这类短词——短词会匹配到大量描述文本，把目标挤到 limit 之外。先按框架推测库名，
搜不到再放宽为短词并提高 `limit`（最大 64）。

**再查库详情**（拿到库名后确认语言与相对路径）：

```json
{"@action": "directly_call_tool", "directly_call_tool_name": "syntaxflowdoc_builtin_lib_details", "directly_call_tool_params": {"name": ["golang-gin-context"]}}
```

注意：库详情只返回库名/语言/路径/描述，**不含导出变量名**。需要变量名时：

- 若引擎由源码构建（工作目录下有 yaklang 仓库），可 `read_file` 读取
  `<repo>/common/syntaxflow/sfbuildin/buildin/<path>`（path 来自详情返回）确认变量；
- 否则**不要猜**，直接改用最小原生语法（见下）。

**找不到库时的兜底**：优先不写 include，用最小原生语法覆盖样例中的 sink：
`.Query(* #-> as $src);` / `.Exec(* #-> as $src);` / `$x.Params(* #-> as $p);`。
只有库名与导出变量都确认后才写 `<include('lib')> as $var`。

常用库对照（仅作参考）：Gin → `golang-gin-context`；Spring → `java-spring-mvc-param`；
Go 标准库输入 → `golang-user-input`。

### 1.3 写规则到 .sf 文件

用 `write_file` 把规则写入工作目录下的 `.sf` 文件（如 `rule.sf`）。写入后**必须**再读取确认
（`read_file`）内容完整，因为后续校验以文件为准。

### 1.4 语法校验

`check-syntaxflow-syntax` 是 scenario 工具（默认清单不展示），但它**可以直接调用**，参数本文已给出，
不需要 `require_tool` 预加载 Schema。**不要使用 `require_tool`**（弱模型容易把 payload 传成对象导致
`require_tool_payload must be a tool name or an array of tool names` 中断任务）。

```json
{"@action": "directly_call_tool", "directly_call_tool_name": "check-syntaxflow-syntax", "directly_call_tool_params": {"path": "<你的 .sf 绝对路径>"}}
```

返回 `passed=true` 表示语法通过；`passed=false` 时按 `errors` 逐行修复后重跑。

**调用纪律**：优先使用上面这种单次 `directly_call_tool` 标量形式，不要用
`directly_call_tool_params_group` 批量形式；参数类型必须与 Schema 一致——整数直接写数字
（`"limit": 20`，不要写成 `"20"`），布尔用 `true`/`false`，不要加引号。

> 同理，`grep` / `read_file` / `write_file` 都在默认工具清单中可见，直接调用即可，
> 不要用 `require_tool` 加载它们。
>
> **本技能不需要 `bash`**：语法校验/正例自检由 `check-syntaxflow-syntax` 完成，
> 库与语法由 `syntaxflowdoc_*` 查询。校验通过后直接交付，不要再用 `bash`
> 探测安装目录或做额外验证（既低效又容易触发参数类型错误）。

### 1.5 正例自检（有样例时必做）

同一个工具，补上样例参数直接调用（无需预加载）：

```json
{
  "@action": "directly_call_tool",
  "directly_call_tool_name": "check-syntaxflow-syntax",
  "directly_call_tool_params": {
    "path": "<.sf 绝对路径>",
    "sample_code": "<用户提供的漏洞样例完整源码>",
    "filename": "vuln.go",
    "language": "golang"
  }
}
```

`language` 取值：`golang`、`java`、`php`、`c`、`javascript`、`yak`、`python`。
`filename` 必须与规则第二个 desc 块里 `'file://vuln.go'` 的文件名一致。

返回 `matched=true` 即验收通过。

### 1.6 迭代策略（失败时）

- **看 `errors` / 诊断信息定位断点**：变量匹配为 0 时，沿依赖链自底向上找根因
  （链首的 include 或 source 未定义/未匹配最常见）。
- **禁止越改越复杂**：连续两轮失败就回到最小模式 —— 先用一行验证 sink：
  `.dangerousMethod(* #-> as $src); alert $src`，通过后再加 source 链路。
- **拆分验证**：复合模式拆成多步（先 `$gin.Context as $context`，再
  `$context.Query(* #-> as $param)`），逐段确认哪一段为 0。
- **`<include('lib')>` 必须紧跟 `as $var`**，否则变量未定义 —— 这是最高频的失败原因。

## 2. 规则骨架（三段式，必须遵守）

```
desc( ...元数据... )          ← 第一个 desc()：仅元数据
<规则体>                      ← include / 数据流 / alert
desc( ...样例... )            ← 第二个 desc()：正例（可选反例）
```

以 `desc()` 开头，**禁止 `rule()` 包装**。最小完整示例（XXE）：

```
desc(
    title: "Check XXE in DocumentBuilderFactory"
    title_zh: "检测 DocumentBuilderFactory 中的 XXE 漏洞"
    type: vuln
    level: high
)
.DocumentBuilderFactory.newDocumentBuilder()(* #-> as $src);
alert $src for { message: "XXE in DocumentBuilderFactory", level: high };
```

**仅含 desc() 的规则无效**：必须有至少一个 `#->` 数据流与至少一个 `alert`。

### desc() 语法硬规则

- 字段格式 `fieldName: value`，**冒号不可省略**（禁止 `title "xxx"`）。
- 字段间用**换行**分隔，**禁止用逗号**作主分隔符（触发 `mismatched input ','`）。
- heredoc 结束符（`DESC`、`SOLUTION`、`UNSAFE`...）必须**单独占一行且行首无空格**。

### 数据流语法约束

- 数据流用 `#->`，**禁止 `-->`**。
- **禁止伪语法**：`pattern()`、`call()`、`with_param()`、`without()`、`testcase()`。
- 方法/变量匹配用具体符号名或 glob（`.get*`、`*config*`），禁止用 `|` 表示多选。

## 3. 正例嵌入（第二个 desc 块）

```
desc(
    lang: golang
    alert_mid: 1
    'file://vuln.go': <<<UNSAFE
<用户样例完整代码原样嵌入>
UNSAFE
    'safefile://safe.go': <<<SAFE
<可选的安全写法>
SAFE
)
```

- `lang` 与 `language` 参数一致；`alert_high`/`alert_mid` 对应规则 alert 的 level。
- `file://` 后的文件名与自检参数 `filename` 一致。

## 4. 元数据规范

- `title` 英文（动词+语言+目的）；`title_zh` 中文必填。
- `desc` 中文 markdown：漏洞原理、触发场景、潜在影响。
- `solution` 中文 markdown 修复建议，非漏洞规则填 `none`；`reference` 无则 `none`。
- `alert` 块内可带 `message` / `solution` / `desc`，heredoc 规则同上。

## 5. 交付

语法（及样例自检）通过后，向用户交付：规则文件路径 + 1~3 句说明（检测目标、数据流要点）
+ 规则正文。**不要**在未通过校验时把规则描述成“已完成”。

## 6. 与专注模式的差异

| 能力 | 本技能（通用模式） | 专注模式 |
|---|---|---|
| 语法校验 | 你显式调用 `check-syntaxflow-syntax` | 写文件后系统自动 lint |
| 正例自检 | 你显式传 `sample_code` 自检 | 语法通过后系统自动重检至 matched |
| 文档/库检索 | `syntaxflowdoc_search` / `syntaxflowdoc_builtin_lib_details`（同一份内嵌快照） | `syntaxflowdoc_*` loop action（同数据） |
| 规则样例 RAG 搜索 | 不可用（改用 grep 搜 .sf 样例） | 内置 Grep + 语义检索 |
| 编辑器交付 | 写入普通文件 | 交付到前端「规则编写」编辑器 |

强制拦截能力无法在通用模式复刻，**所以本技能的验收闸门靠你自己严格执行第 0 节标准**。
