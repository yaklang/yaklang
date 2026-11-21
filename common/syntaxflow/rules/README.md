# Condition rules

Reusable SyntaxFlow rules for "which conditions compare a value with a
constant". They are inventory queries, not findings: the result is a list of
comparison sites, with no severity, CWE or message.

| 文件 | 找什么 |
| --- | --- |
| `condition-const-compare.sf` | `If`/`Loop` 条件里，二元运算的一侧是常量 |
| `condition-call-strcmp-literal.sf` | `If`/`Loop` 条件里，`strcmp` 一类比较调用的实参是字符串字面量 |

两条规则互补。C 没有字符串 `==`，所以真正的字符串比较都在调用里；第一条规则
找到的是字符字面量（`c == 's'`）和整数标志，第二条找到的是
`strcmp(x, "https") == 0` 这类调用。在 php-src 上第一条命中 39,431 条，其中
字符串常量 0 条，第二条补了 407 条。

## 运行

```sh
yak ssa-query -p <program> --database <db> --syntaxflow common/syntaxflow/rules/condition-const-compare.sf
```

`yak ssa-query` 直接打印各个符号的值。要把结果落成 JSON，用
`sf_report`（本仓库外的审计工具，见下方「导出」）。

## 输出符号

`condition-const-compare.sf`

| 符号 | 含义 |
| --- | --- |
| `$stmt` | 判断语句（`If` 或 `Loop`） |
| `$cond` | 该语句实际测试的条件值 |
| `$bin` | 条件里的二元比较（`$binDirect` 直接子节点 + `$binNested` 经 `#>` 可达） |
| `$const` | 二元比较的常量操作数 |

`condition-call-strcmp-literal.sf`

| 符号 | 含义 |
| --- | --- |
| `$stmt` / `$cond` | 同上 |
| `$calls` | 条件里可达的全部调用 |
| `$call` | 命中比较函数名的调用 |
| `$arg` | 调用的常量实参（含 `strncmp` 的长度、Zend 宏的 `sizeof()`） |

## 写法要点

- `opcode:` 只接受标识符形式的名字（写 `gteq`，不能写符号 `>=`）。写错名字会在
  编译期报错并列出可用的名字，不再静默返回空。
- 通配过滤器 `?{*a*}` 允许 `-`，因为 SSA 把调用打印成 `Function-strcmp(...)`。
  但模式必须以 `*` 开头：裸标识符不能含 `-`，否则会把 `->`、`-{` 的 `-` 吃掉。
- 多个名字用 `||` 连接即可，`?{*a* || *b*}` 正常工作。每个名字一个变量再 `+`
  合并也等价，实测快约 25%（php-src 全量库：58 秒 vs 74 秒），名字很多时可以
  选这种写法。
- `have: 'x'` 是「值文本包含 x」，`opcode:` 是「指令类型」，两者可以混在 `||` 里。

## 导出

`sf_report` 是审计时用的导出器（不在本仓库内）：它执行规则、把 `$bin` / `$call`
渲染成 JSON 行，并做行号回映射（预处理行号 → 源文件行号，三态
`exact` / `literal` / `estimated`）。每行的字段：

```json
{
  "file": "ext/soap/php_sdl.c",
  "line": 304,
  "line_source": "exact",
  "function": "sdl_set_uri_credentials",
  "form": "binop",
  "operator": "eq",
  "condition": "ctx->sdl->source[4] == 's'",
  "constants": [{"value": "s", "type": "char", "length": 1}],
  "short_string": false,
  "single_char": true,
  "semantic_hint": "single-character discriminator",
  "ssa_value": "eq(ParameterMember-parameter[0].4, 115)"
}
```

`form=call` 的行多一个 `callee` 字段，常量来自实参。`redundant_wrapper=true`
表示这条二元行只是把比较调用的结果和 0 比（`strcmp(...) == 0`），字面量由同一
位置的 `call` 行给出，统计时不计入。
