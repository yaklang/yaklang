# Yak 脚本模型 Provider

`yak-script` 是通用模型适配宿主。客户协议放在用户提供的可信脚本中，不内置客户端点、业务字段或适配模板。它以正常 Yak 运行时权限执行，每次尝试创建新的 VM；VM 隔离不等于操作系统权限沙箱。

## 三层职责

| 层 | 职责 |
| --- | --- |
| Go 宿主 | 创建 VM、注入接口、传递取消与超时、管理全局请求频率和恢复、把正文接入 StreamHandler / AIResponse，校验并交付原生工具调用 |
| Yak 脚本 | HTTP 请求映射、状态码与媒体类型检查、JSON / SSE 内部业务字段解析、完整性验证，交付正文、归一化工具调用或错误 |
| Agent | 解析文本 `@action` / answer_payload 或原生工具调用、纠正格式并执行工具 |

`data:` 和空行是 SSE 格式；`choices[].delta.content` 是模型接口格式；正文中的 `@action` 是 Agent 协议。HTTP 200、合法 SSE 或遇到 `[DONE]` 都不能单独证明业务成功。

## 模型配置

选择 Provider `yak-script`，运行配置保存 `Provider.Script` 和协议开关 `Provider.EnableNativeToolCalling`（默认 false）。不从 Provider 或 Model 的 ExtraParams 读取源码，不设覆盖优先级或旧配置转换。地址、凭据、上游模型 ID、Headers、代理、超时、采样参数以及业务选项全部定义在脚本源码中。

`AIModelConfig.ModelName` 只用作配置显示名称和路由标识，主/辅助类型仍用于模型选择；这两项不传入脚本，不决定上游请求。普通 Provider 的连接配置不适用于脚本模型。

```text
AIModelConfig
  ModelName: 显示与选择用的配置名称
  Provider:
    Type: yak-script
    Script: 完整 Yak 源码
    EnableNativeToolCalling: false # 原生工具调用；按每条配置单独保存
```

[内置通用示例](example.yak) 演示 Chat Completions，在 `chat` 开头修改 endpoint、model、apiKey、timeoutSeconds、proxy 和 headers。`mode = "json"` 可改为 `"sse"`，两种模式均完整验证后返回正文。新建时 Monaco 直接填入该模板；没有模板载入按钮或单独连接/超时表单。原生工具调用打开且本次请求包含工具定义时，示例使用非流式 JSON，解析完成后返回归一化工具调用。标准接口通常直接使用普通 Provider；此示例用于说明如何编写适配。

脚本中的 HTTP 超时在实际发起请求时生效，不受普通模型的默认 `AIConfig.Timeout` 截断。宿主仍传递调用方取消与总 deadline；需要约束脚本整体执行时，可以在脚本内从 req.Context 派生有期限的 Context。源码错误或无限循环仍可通过停止任务取消。

## 脚本入口与正式 ai 库

入口是 `chat(req, emit)`。辅助函数注册到正式 `ai.Exports`，由普通 Yak 引擎加载；不注入临时 `provider` map，也不新增专用脚本类型。前端使用普通 Yak Monaco 编辑器，通过既有语言服务获得库函数补全、悬浮说明与签名。

| req 字段 | 含义 |
| --- | --- |
| Context | 本次尝试的取消上下文，继承调用方总 deadline；HTTP 调用必须传 `poc.context(req.Context)` |
| RequestID | 逻辑模型调用 ID，宿主重试复用，新模型调用重新生成；不代表上游自动去重 |
| Prompt | 上层构造的完整提示词，可能包含历史、动作规范和工具结果，不要仅截取最后一句 |
| RawMessages | 显式消息列表；内置示例明确拒绝它，避免无声丢失角色 / 多模态信息 |
| Tools | 本次原生工具定义（name、description、parameters），文本模式为空 |
| ToolChoice | 本次原生工具选择约束；脚本应映射给上游 |

| 入口回调 / ai 库函数 | 用法和边界 |
| --- | --- |
| `emit(text)` | 交付模型正文片段；用 `emit(part)~` 传播失败，最后 `return nil` |
| `ai.ReadSSE(reader, callback)` | 只解析 SSE 信封；回调接收字符串 `(event, data)`，JSON 和业务字段仍由脚本解析 |
| `ai.Retry(req, reason, retryAfter)` | 标记本次可恢复失败并返回 error；应 `return ai.Retry(...)`；不会在脚本内 sleep / 重发 |
| `ai.IsTransientError(err)` | 检查 EOF、网络超时、连接重置等传输失败；不认识 HTTP 状态或业务错误码 |

`retryAfter` 接受 HTTP Retry-After 秒数或 HTTP 日期字符串，空字符串表示没有上游等待提示。错误原因应简短，避免把凭据和原始报文塞入 UI 错误文本。

### 输出契约

二选一：返回非空正文字符串，或者多次 `emit(text)~` 后返回 nil。不能既 emit 又返回正文。错误用 `return error(...)`、断言或 `ai.Retry(...)` 表达，不能返回包含报错的字符串假装成功。

只交付模型正文：不返回完整 HTTP / SSE 信封、choices 包装、进度或错误页；不在脚本里补造 `@action`。动作格式校验留在 Agent。

目前脚本没有单独的思考 emitter，也没有 usage / 多模态输出 ABI。通用示例忽略 reasoning_content，不把思考拼入正文。标准 Provider 的独立 ReasonStreamHandler 不等于脚本自动获得这一能力。

开启全局无限重试时，宿主先缓冲 emit，只有尝试成功才提交，失败尝试的残缺正文被丢弃。关闭时，emit 按到达顺序交付；已交付文本无法撤回，宿主不重放已经输出部分正文的失败尝试。需要严格完整性的适配优先采用“校验后 return”。

### 原生工具调用开关

界面位于脚本模型的 Monaco 编辑器上方，默认关闭。关闭时 Agent 在正文中使用 `@action`；开启后 Agent 使用原生工具调用协议，将定义通过 `req.Tools` / `req.ToolChoice` 传入脚本。选择另一条模型配置时重新读取该配置的开关；显式的 Agent 协议选项仍优先。普通 Provider 的默认行为不变。

开启开关不替脚本猜测上游格式。脚本负责解析其 HTTP / SSE 格式，然后返回如下对象（不是完整的 Chat Completions 响应）：

```yak
return {
    "content": "", // 可选的模型正文
    "tool_calls": [{
        "id": "call_1",
        "type": "function",
        "function": {
            "name": "directly_answer", // 必须是 req.Tools 中声明的名字
            "arguments": `{"answer_payload":"42"}`, // 完整 JSON 对象的字符串
        },
    }],
    "finish_reason": "tool_calls",
}
```

宿主先验证所有调用的唯一非空 ID、类型、已声明的名称、完整 JSON 对象参数以及 `finish_reason=tool_calls`，成功后才交付 `ToolCallCallback` 和 `FinishReasonCallback`。Agent 再验证具体动作参数并执行。工具参数不会作为正文传入 `emit`。原生请求始终缓冲到验证结束，避免失败重试时重复提交部分调用。

无工具调用的完成对象使用非空 `content`、空 `tool_calls`、`finish_reason="stop"`。字符串正文和辅助文本调用仍可使用原契约。不能同时 emit 和返回对象；关闭开关时返回对象会报错。自定义 SSE 适配需要在脚本中组装增量调用，完整验证后一次性返回对象；当前不提供原生工具参数增量 emitter。

`applyProviderFunctionCallDefault()` 只根据选中的 Provider 配置确定 Agent 默认协议，不再从 `yak-script` 类型硬编码推断能力。单独的模型健康检查仍是文本连通性检查，不代替原生工具调用测试。

### SSE 解析

`ReadSSE` 支持 BOM、LF / CRLF / CR、注释和多行 data；默认事件名是 message。回调返回 nil 继续，返回 io.EOF 正常停止，其他 error 中止并向上传播。

它不解析 JSON，不实现 EventSource 自动重连，不暴露 id / retry 字段。未以空行结束的最后事件返回 UnexpectedEOF；事件边界完整但缺少业务结束标记时，仍需脚本检查。单行 / 单事件有约 8 MiB 限制。

上游增量应追加，累计快照应替换或去重，最终答案应按协议确认唯一性。不要把三种形式一律拼接。通用模板检查 finish_reason=stop、非空正文，SSE 模式额外检查 [DONE]。

示例先用 poc.Post 收完 Body 再 ReadSSE，属于缓冲解析。实时交付需要在 poc.bodyStreamHandler 中消费 Reader、检查 HTTP 头并处理回调错误；不能只把 return 改成 emit 就宣称实现了实时网络流。当前 ChatStream 方法也返回缓冲 Reader，逐段输出通过 Chat 的 StreamHandler 接入。

## 全局频率：按开始时间的滚动窗口

桌面入口：设置 → AI 配置 → 自动化配置 → 模型调用频率。`ModelRequestsPerMinute` 为 0..60000，默认 0（关闭主动限频）。同一引擎内，接入统一调用入口的普通模型、脚本模型、主 / 辅助调用和重试共享队列。

采用开始时间日志：每次获准请求记录时间 t，统计 `(now - 60s, now]` 内的次数。已达 N 次则 FIFO 排队，直到对应的旧时间戳过期。成功、失败、超时都计数；响应结束不返还额度。正常请求允许并发，RPM 不是并发数限制。

例如 3 RPM：在 0、10、20 秒开始三次请求，第 4 次最早在 60 秒开始。第 1 次在 1 秒结束或在 90 秒结束，都不改变这个规则。最初可以同时发出三次，不强制每 20 秒一条。

| 算法 | 是否满足任意连续 60 秒最多 N 次 |
| --- | --- |
| 固定分钟窗口 | 否，边界前后可能形成双倍突发 |
| 容量 N、每分钟补 N 的令牌桶 | 否，初始突发后可以在同一 60 秒内再次消耗补充令牌 |
| 请求完成释放槽位 | 否，这是限制在途请求数量的并发控制 |
| 请求开始时间滚动窗口 | 是，本实现采用此方式 |

限频只限制当前引擎主动发起的逻辑尝试，不知道上游 Agent 内部又发了多少模型请求。脚本应一次尝试最多发起一个模型请求，保留 poc.retryTimes(0) / poc.redirectTimes(0)，不要在源码内另写请求循环绕过计数。底层透明重连的物理请求数不等同于逻辑尝试数。

更改全局上限会唤醒队列按新值重新计算；失败恢复的 Retry-After 不因关闭主动限频而被清除。配置保存在 profile DB，时间戳和冷却状态仅在进程内，不跨引擎共享，重启后不保留。

算法参考：[Redis 限流算法比较](https://redis.io/tutorials/howtos/ratelimiting/)、[Microsoft 对并发与时间限流的区分](https://learn.microsoft.com/en-us/aspnet/core/performance/rate-limit?view=aspnetcore-10.0)。

## 无限重试和限频后的恢复

`AIGlobalConfig.RetryForever` 是独立全局开关，默认关闭。启用后，尚未提交正文的执行阶段失败持续重试，包括未知业务错误、脚本执行错误、空结果、单次超时；Agent 格式纠正也不受普通重试次数限制。配置预检查失败仍直接报错。

有主动限频时，临时故障触发共享冷却：指数退避从 2 秒增长，退避基数上限 5 分钟，加正向抖动；Retry-After 作为最短等待时间，不受 5 分钟上限截断。冷却结束只放行一个恢复探测，请求成功后恢复正常窗口放行。之前在途请求迟到的成功响应不能解除后来收到的 Retry-After。

未开启主动限频且尚无共享冷却时，恢复按每次逻辑调用单独退避。等待不占单次请求超时，但用户取消和调用方总 deadline 始终有效。已完成的本地工具不会因模型重试而重新执行；上游自身的副作用需要上游提供幂等性。

网络调试与这些开关相互独立，见 [网络诊断说明](../NETWORK_DEBUG.md)。

## 验证

通用模板使用本地 httptest 服务验证，无需真实密钥。覆盖请求字段映射、JSON / SSE、结束条件、reasoning 隔离、无效 JSON、截断、HTTP 429、返回错误和 emit 契约。

```sh
go test -short -tags irify_exclude ./common/ai/yakscript -count=1
go test -short -tags irify_exclude ./common/yak -run '^TestYakScriptProvider' -count=1
go test -short -tags irify_exclude ./common/ai/aispec -run '^TestGlobalModelLimit' -count=1
```
