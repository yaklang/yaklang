# LiteForge 单次请求底层

`common/ai/aid/liteforge` 是独立的单次结构化请求执行器。[liteforgeapp](liteforgeapp/README.md) 的应用、Yak `liteforge.Execute`、`ai.FunctionCall`（含分级模型入口）和基于 `aicommon` 的 typed helper 共用它。执行器使用 `aicommon.Config` 的模型选择、重试和用量通道，以及 `aiprojection` 的请求投影；不依赖 coordinator、coordinator_legacy、ReAct 或 aiforge，也不创建 PLAN、任务循环或旧事件循环。Forge 的多步执行暂不迁移。

应用层的文件分析、多媒体、知识提炼、索引和 ERM 实现统一归入 `liteforgeapp`。核心包不导入应用包；Go 调用方导入 `github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp`，Yak 的模块名和函数名保持不变。

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

普通默认主循环的真实模型冒烟使用独立 [default_task.yak](../../aismoking/live/default_task.yak)，直接由 Yak CLI 执行，不需要 Go 运行器注入变量。脚本通过 `aim.InvokeReAct` 读取订单材料、保存 evidence 并写对账报告；校验金额、去重和异常数组，输出 provider 用量、调用事件、提示词快照路径及按 token 加权的缓存率。凭据从 `LITEFORGE_SMOKE_API_KEY` 读取；可用 `LITEFORGE_TASK_DIR` 指定新的输出目录，`LITEFORGE_SMOKE_MODEL` 指定模型，默认 `deepseek-v4.1-flash`。总体缓存统计包含收到 provider usage 的请求；没有完整用量的取消请求保留在事件与日志中，不计入此分母。

```powershell
yak common/ai/aismoking/live/default_task.yak
```

`go test ./common/ai/aid/liteforge` 覆盖开放 map、任意 JSON 值、必填约束、重试、原生调用身份和截断拒绝、增量回调、缓存前缀与伪造边界，以及旧嵌套参数包装。

统一验证入口在 [AI 冒烟测试](../../aismoking/README.md)。直接执行：

```text
yak common/ai/aismoking/liteforge.yak
```

脚本使用本地 HTTP/SSE provider，实际验证 LiteForge 与 `ai.FunctionCall` × 两种协议四条链路；服务端等待 `onStream` 读到首字节才发送剩余响应，验证文本和 arguments 的增量处理。另覆盖抽取、分类、总结 × 双协议 × aim/public 两个入口，共 16 次请求；检查 tools、tool_choice、投影、开放 JSON 和字段语义，不需要 Go 注入。

内部 Go 回归 `TestLiteForgeAIMBothProtocols` 保留流式输出与结果断言，测试公开 AI engine 集成；provider 缓存率需真实模型 usage，mock 只验证分区与执行链路。
