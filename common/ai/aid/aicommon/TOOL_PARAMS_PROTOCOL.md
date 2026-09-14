# 固定工具的参数生成协议

适用于 `ToolCaller.generateParams`（R2），包括文本和原生 FunctionCall 模式。进入此阶段时，运行时已经选定工具。R1 的动作选择、直接调用参数及工具执行后的失败处理继续走各自的流程。

## 标准输出与兼容形式

标准输出仍使用一份完整外壳：

```json
{"@action":"call-tool","tool":"write_file","identifier":"write_example","params":{"file":"/tmp/example.txt","content":"example","force":true}}
```

| 响应 | R2 处理 |
| --- | --- |
| 标准外壳或 `action` 别名 | 接受；`@action` 优先，空值或错误值不能被别名覆盖 |
| `{"tool":"write_file","params":{...}}` | 补齐 `@action` |
| `{"@action":"call-tool","params":{...}}`、`{"params":{...}}` | 使用运行时已经选定的工具，补齐缺少的标识 |
| `{"file":"...","content":"...","force":"true"}` | 仅当所有顶层键均为工具声明的参数且不含协议保留键时，作为裸参数对象包装 |
| 空对象 `{}` | 作为真实的空参数对象，仍需通过工具 schema；不能从空响应推断 |

保留键为 `@action`、`action`、`tool`、`params`、`identifier`、`call_expectations`。如果工具业务参数使用这些名字，必须放在标准外壳的 `params` 内；不会从嵌套业务字段推断动作或工具。业务参数本身名为 `params` 时，外壳必须显式提供动作或工具身份，避免误把业务对象解包。

只处理一个完整 JSON 对象，可使用 Markdown JSON 围栏。多个候选对象、顶层数组、截断 JSON、重复外壳键、已声明参数混放及未知裸参数字段均报错。外壳中与工具参数不重名的未知诊断字段保留在原始记录中，不传入工具。显式工具名必须严格匹配固定工具；原生 FunctionCall 的名称、调用 ID 和索引也必须属于同一次工具调用。

## 内容与类型

- 只解析 JSON 对象之后、针对已声明参数且首尾匹配的完整 AITAG。JSON 字符串中的标签按原文保留。
- 当前 nonce 的 AITAG 优先于同名 JSON 值；明确提供的空块是空字符串，未提供的块不等于空字符串。同一参数的多个块报错。
- 保留原有单个错误 nonce 的有限恢复：只有一个完整非空块、没有当前 nonce 块、且 JSON 中该字段没有非空值时，才可补充该字段。无法恢复的缺失字段仍由 schema 拒绝。
- 仅当顶层参数 schema 明确为 `boolean` 时，将精确的字符串 `"true"` / `"false"` 转成布尔值。不会猜测 `yes`、数字、联合类型或字符串字段的类型。
- 归一化后调用工具自身的 `ValidateParams`，沿用其默认值、必填项、类型和取值约束。不会凭历史输出或报错补造内容。

## 重试与执行边界

完整读取响应及内容块后，依次检查固定工具身份、归一化参数、合并内容块、校验 schema。任何错误返回 `CallAITransaction` 的现有有界重试流程，下一轮 prompt 带上具体错误并要求重新提供完整参数。取消请求直接终止。

每个尝试独占参数、identifier、expectations 和原始响应；只有成功的尝试能提交结果。不同尝试不会合并，批量调用的不同成员也不会共享候选参数。校验成功之后才进入工具审批和调用；原有调用前校验继续作为最终防线。

本次日志中的第一轮有 `content/file/force`，但缺少协议外壳；第二轮原始响应已经缺少 `content`，工具报告也确认回调未执行。前者可在固定工具范围内恢复，后者必须在参数生成阶段退回重试。

回归入口：`toolcall_fixed_params_test.go`、`toolcall_action_tolerance_test.go`、`toolcall_nonce_test.go`。测试使用普通文本和惰性回调，覆盖两种模式、内容保真、失败拒绝、重试隔离及真实 ToolCaller 调用边界。
