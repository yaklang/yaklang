# LiteForge 单次请求底层

`common/ai/aid/liteforge` 是独立的单次结构化请求执行器。公开 `aiforge.LiteForge`、Yak `liteforge.Execute`、`ai.FunctionCall`（含分级模型入口）和基于 `aicommon` 的 typed helper 共用它。执行器使用 `aicommon.Config` 的模型选择、重试和用量通道，以及 `aiprojection` 的请求投影；不依赖 coordinator、coordinator_legacy、ReAct 或 aiforge，也不创建 PLAN、任务循环或旧事件循环。Forge 的多步执行暂不迁移。

`Request → 构建稳定提示词与 schema → 调用模型 → 流式字段回调 → 完整结果校验 → Action`。

## 两种协议

默认开启 function call，包括自定义模型 callback 的独立调用。调用方用 `aicommon.WithEnableFunctionCallMode(false)` 选择旧文本协议，或用 `true` 显式开启。ReAct 和 Config 的辅助调用继承父配置，也允许一次调用显式覆盖；选项按传入顺序生效，后面的协议选项优先。

Yak 的两个公开入口都接受 `ai.withFunctionCallMode(true/false)`。`liteforge.Execute` 完整传递 `ai.*` 的 provider/model/APIKey/BaseURL 等配置；`ai.onStream` 在文本模式接收 JSON 文本，在原生模式接收增量 arguments，同时保持内部解析流和字段流独立。`ai.FunctionCall` 接受字段描述 map、字段 schema map 或完整 object schema，返回业务 map，移除内部 `@action` 标记；描述字段保持任意 JSON 类型，显式字段 schema 则检查最低约束，扩展字段保留。

Gateway 的显式、按层级和按策略选择只负责选模型，结果均进入此执行器，不再调用 provider `ExtractData`，也不回退旧抽取实现。ReAct 的重复 LiteForge 构造分支已删除。provider 的底层 `ExtractData` 接口仍有 CVE、chaosmaker 的直接调用，不属于上述两个入口。

为避免 `ai → liteforge → aicommon → ai` 的依赖环，gateway 通过 `aispec` 中的执行器注册桥接调用。Yak/aid 的正常初始化已加载本包；只导入 `common/ai` 的独立 Go 程序需要额外导入 `_ "github.com/yaklang/yaklang/common/ai/aid/liteforge"`。未加载时明确报错，不启用旧底层。

- 文本流：按输出 schema 生成 JSON，由 ActionMaker 解析。已有 `@action` 与 `call-tool / params` 包装保持兼容；开放业务对象无需自行声明 `@action`，系统在校验后绑定 action。
- Function call：只声明一个输出函数，并用 tool choice 指定它。和 mainloop 一样，`aiprojection.CreateActionSchema` 构建受信任的函数声明，放在 `semi-dynamic-2`，由 ChatBase 发送前的投影转换为 provider `tools`，同时从消息正文移除；执行器不直接设置 `WithTools`。参数沿用业务 schema，仅移除根部的传输字段 `@action`。必须是一次完整、名称正确、以 `tool_calls` 结束的函数调用；普通文本、多个调用、截断参数不会被当成成功。

两种协议都检查业务 schema 的最低约束，再执行调用方 OutputValidator；失败使用现有事务重试预算。开放对象与未知字段不裁剪，任意值使用 `{}`，未知键值 map 使用 `additionalProperties: true`。结果仍是一个对象；任意数组或标量可放在 schema 为 `{}` 的字段中。

```json
{
  "type": "object",
  "properties": {
    "summary": {"type": "string"},
    "payload": {},
    "metadata": {"type": "object", "additionalProperties": true}
  },
  "required": ["summary", "payload"],
  "additionalProperties": true
}
```

图片、模型选项、JSON hook、emitter、字段流回调、自定义响应 handler 和请求上下文继续透传。自定义 handler 自己拥有提示词与响应协议，不额外套输出函数。

## 上下文与缓存

两种协议各有一份中文 promptloader 模板，可信模板在填充数据前签名。high-static 是固定通用规则；semi-dynamic 保存稳定业务指令与已有持久上下文，文本协议还在其中保存旧 JSON schema；原生协议的函数声明只在 semi-dynamic-2 出现一次，发送时转为 tools，不重复复制参数 schema；timeline-open 只取最近 4096 tokens，调用方可禁用；dynamic 保存当次材料与参数。单次请求不再复制整个冻结历史。动态数据及伪造边界不能变成 system 指令。

Function call 的字段流直接读取 `ToolCallArgumentsStreamHandler` 提供的原始 arguments reader，与普通 content/reason 流分开。ToolCallCallback 只收集调用身份及用于一致性核对的参数副本，不重复触发字段回调。仅实现旧 callback 的 provider 仍可兼容返回结果。

字段流用于增量展示，可能来自最终被拒绝的尝试。只有完整结构、协议和业务校验都通过且字段回调已结束，才返回最终 Action。Provider 内部重试重新收集参数，不拼接前一次响应；若底层把多次 HTTP 响应混进同一个参数 reader，则拒绝该结果并使用事务重试。取消会结束参数管道。

## 验证

`go test ./common/ai/aid/liteforge` 覆盖开放 map、任意 JSON 值、必填约束、重试、原生调用身份和截断拒绝、增量回调、缓存前缀与伪造边界，以及旧嵌套参数包装。

`TestProtocolsProjectAtSendAndStreamBeforeResponseEnds` 使用本地 HTTP/SSE 服务验证实际发送路径：默认开启原生协议，关闭后兼容旧 `@action` JSON；工具必须由发送前的投影注入；服务端只有收到字段回调通知后才发送剩余响应，保证两种协议都能增量处理字段。

`TestLiteForgeYakAIMBothProtocols` 执行 [smoke.yak](smoke.yak)：抽取、分类、总结 × 文本流/function call，逐个验证 aim 与公开 LiteForge 入口。只有模型 provider 使用确定性响应，其余执行真实代码；每个入口恰好请求一次。这些测试验证链路与参数语义，不代表实际模型质量或 provider 缓存命中率。

`TestYakLiteForgeAndFunctionCallCrossProtocols` 用真实 Yak ScriptEngine 执行 [smoke_liteforge.yak](smoke_liteforge.yak) 和 [smoke_functioncall.yak](smoke_functioncall.yak)，共四次 HTTP/SSE 请求。检查实际 tools、tool_choice、消息投影、认证和模型选项；服务端等待 `onStream` 读到首字节才发送剩余结果，验证普通文本和 arguments 都增量输出。脚本的 INPUT、OPTIONS、VERIFY 由运行器注入，不替换模块函数。

```powershell
go test ./common/ai/aid/liteforge -run '^TestYakLiteForgeAndFunctionCallCrossProtocols$' -count=1 -v
# 从环境变量提供凭据，使用相同两个脚本验证真实模型，凭据不写入脚本。
# LITEFORGE_SMOKE_API_KEY 必填，PROVIDER/MODEL 默认为 aibalance/deepseek-v4.1-flash。
go test ./common/ai/aid/liteforge -run '^TestYakGatewayLiveSmoke$' -count=1 -v
```
