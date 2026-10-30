---
name: syntaxflow-rule
metadata:
  display_name_zh-CN: SyntaxFlow 规则编写
description: >
  SyntaxFlow 漏洞检测规则编写技能。覆盖规则骨架（双 desc 结构）、desc 元数据语法硬规则、
  正例嵌入 file://UNSAFE 格式、数据流语法约束与语法校验工具用法。用于在通用对话模式下
  轻量生成或修改 SyntaxFlow 规则；带强制正例自检闭环的完整规则开发请使用
  「编写 SyntaxFlow 规则」专注模式。
---

# SyntaxFlow 规则编写 (syntaxflow-rule)

编写正确、可编译的 SyntaxFlow 漏洞检测规则。语法错误（`missing ')'`、`mismatched input ','`、
`mismatched input ':' expecting <EOF>`）几乎都源于 desc 格式不合规，先遵守本文约定再动手。

## 1. 规则骨架（必须遵守）

规则文件以 `desc()` 开头，**禁止使用 `rule()` 包装**。标准布局为三段：

```
desc( ...元数据... )          ← 第一个 desc()：仅元数据
<规则体>                      ← include / 数据流 / alert
desc( ...样例... )            ← 第二个 desc()：正例（与可选反例）
```

最小完整示例（XXE 检测）：

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

**仅含 desc() 的规则是无效的**：desc 只提供元数据，每条规则必须在 desc() 之后包含检测逻辑——
至少一个数据流模式（含 `#->`）和至少一个 `alert` 语句。

完整范例参考（仓库内置规则，结构标杆）：
- `common/syntaxflow/sfbuildin/buildin/golang/cwe-1336-ssti/golang-template-ssti.sf`
- `common/syntaxflow/sfbuildin/buildin/golang/cwe-79-xss/golang-reflected-xss-gin-context.sf`

## 2. desc() 语法硬规则

- **字段格式**：`fieldName: value`，冒号不可省略。禁止 `title "xxx"` 或 `type vuln`（缺冒号）。
- **字段分隔**：多行 desc 中字段间用**换行**分隔，**禁止用逗号**作主分隔符（易触发 `mismatched input ','`）。
- **heredoc 结束符**：`desc: <<<TEXT ... TEXT` 的结束标识符（TEXT、DESC、SOLUTION、UNSAFE 等）
  必须单独占一行且行首无空格。前导空格会导致 heredoc 不闭合。

错误示例（缺冒号、滥用逗号）：

```
desc(
    title "检测漏洞",
    type vuln,
```

正确示例：

```
desc(
    title: "检测漏洞"
    type: audit
    level: high
)
```

## 3. 正例嵌入（file:// + UNSAFE）

用户提供漏洞代码样例时，样例即**正例**，必须嵌入到**规则末尾的第二个 desc() 块**：

```
desc(
    lang: golang
    alert_mid: 1
    'file://unsafe.go': <<<UNSAFE
<用户样例完整代码原样嵌入>
UNSAFE
    'safefile://safe.go': <<<SAFE
<可选的安全写法反例>
SAFE
)
```

- `lang` 按样例语言填（golang/java/python...）；`alert_high`/`alert_mid` 与规则 alert 的
  level 对应，数量为该级别预期命中数。
- 文件名后缀与语言一致（.go、.java）。
- 反例（`safefile://` + `SAFE`）可选，用于验证规则不误报。

## 4. 检测逻辑语法约束

- 数据流用 `#->`，**禁止 `-->`**。
- **禁止使用不存在的伪语法**：`pattern()`、`call()`、`with_param()`、`without()`、`testcase()`。
- 库引用：`<include('lib')>` **必须紧跟 `as $var`**，否则变量未定义。例如：
  `<include('golang-gin-context')> as $gin; $gin.Context as $context;`
- 常用 lib 选择：Gin → `golang-gin-context`；Spring → `java-spring-mvc-param`；
  Go 标准库输入 → `golang-user-input`。
- 方法/变量匹配用具体符号名或 glob（`.get*`、`*config*`），禁止在字符串里用 `|` 表示多选。

## 5. 编写思路

1. **从样例出发**：先读懂漏洞代码，确定 sink（如 exec、Query、Parse）与 source（如 c.Query）。
2. **先简单后复杂**：首版用最小模式（`.dangerousMethod(* #-> as $src); alert $src`），
   语法通过后再加 include 与过滤条件。
3. **变量链自底向上排查**：自检反馈为 0 的变量沿依赖链找根因——链首（include 或 source）
   未定义或匹配数为 0 是最常见根因。复合模式可拆成多步逐段验证。
4. **多次失败时回归参考**：禁止越改越复杂；换用结构最接近的内置规则骨架，仅调整方法名/参数。

## 6. 元数据字段规范

- `title`：英文，动词+语言+目的；`title_zh`：中文，必填。
- `desc`：中文 markdown，写清漏洞原理、触发场景、潜在影响（可含代码示例）。
- `solution`：中文 markdown 修复建议；非漏洞规则填 `none`。
- `reference`：CWE/OWASP 链接，无则 `none`。
- `alert` 块内可携带 `message`、`solution`、`desc` 等字段，同样用 heredoc，格式规则同上。

## 7. 工具与校验

- **语法校验**：`check_syntaxflow_syntax`（参数 `syntaxflow-code` 或 `path`）。该工具默认不在
  工具清单中展示，通过 `require_tool` 加载后调用。**禁止**用 yaklang 语法检查工具校验
  SyntaxFlow 规则。
- **读写 .sf 文件**：`read_file` / `write_file`。
- 生成的规则建议写入 `.sf` 文件再做校验，而不是只留在对话里。

## 8. 边界说明

本技能覆盖通用对话模式下的规则编写与语法自查。需要**强制正例自检闭环**（自动验证规则能命中
正例、失败自动迭代）与规则编辑器交付时，请切换到「编写 SyntaxFlow 规则」专注模式。
