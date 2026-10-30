# Yak 脚本模型 Provider

`yak-script` 是通用模型适配宿主。客户协议放在用户提供的可信脚本中，不内置客户端点、业务字段或适配模板。它以正常 Yak 运行时权限执行，每次尝试创建新的 VM；VM 隔离不等于操作系统权限沙箱。

## 三层职责

| 层 | 职责 |
| --- | --- |
| Go 宿主 | 创建 VM、提供正式 ai 库 API、传递取消与超时，由 aispec 统一管理请求频率和恢复、把正文接入 StreamHandler / AIResponse，校验并交付原生工具调用 |
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

## 正式 API 与入口类型

入口是 `chat(req, emit)`，可选入口是 `shouldRetry(req, failure)`。辅助函数通过正式 `ai` 库导出，不需要 import。编辑器使用普通 `yak` 语言模式，既有语言服务从 script engine 的库表读取正式导出，并结合文档提供 `ai.*` 补全、悬浮说明和函数签名。`req` / `failure` 是宿主运行时传入的入口参数，其字段契约见下表；不另设专用编辑器类型或入口参数类型注入。

| req 字段 | 含义 |
| --- | --- |
| Context | 本次尝试的取消上下文，继承调用方总 deadline；HTTP 调用必须传 `poc.context(req.Context)` |
| RequestID | 逻辑模型调用 ID，宿主重试复用，新模型调用重新生成；不代表上游自动去重 |
| Prompt | 上层构造的完整提示词，可能包含历史、动作规范和工具结果，不要仅截取最后一句 |
| RawMessages | 显式消息列表；内置示例明确拒绝它，避免无声丢失角色 / 多模态信息 |
| Tools | 本次原生工具定义（name、description、parameters），文本模式为空 |
| ToolChoice | 本次原生工具选择约束；脚本应映射给上游 |

| API | 用法和边界 |
| --- | --- |
| `emit(text)` | 交付正文片段；最后返回 nil，不同时返回正文 |
| `ai.ReadSSE(reader, callback)` | 只解析 SSE 信封，回调接收 `(event, data)` |
| `ai.ModelError(req, kind, status, code, message, retryAfter)` | 返回结构化错误；支持 `return` 或 `~` 传播，不将错误交给 Agent 当作正文 |
| `ai.IsTransientError(err)` | 判断 EOF、网络超时、连接重置等传输失败 |
| `ai.Retry(reason, retryAfter)` | 在 shouldRetry 中返回重试决策，不自行 sleep 或发送请求 |
| `ai.StopRetry(reason)` | 在 shouldRetry 中拒绝重试；返回 nil 使用默认判断 |

`failure` 含 Kind、StatusCode、Code、RetryAfter、Cause、Attempt。Kind 为 network、http、protocol、business、configuration 或 script。业务错误由适配脚本明确判断，不按错误字符串猜测。脚本错误、明确的不可恢复 HTTP 客户端错误和取消不能强制重试。回调自身异常或返回非决策值作为脚本错误直接报告。

`retryAfter` 接受 HTTP Retry-After 秒数或 HTTP 日期字符串，空字符串表示没有上游等待提示。错误原因应简短，避免把凭据和原始报文塞入 UI 错误文本。

### 输出契约

二选一：返回非空正文字符串，或者多次 `emit(text)~` 后返回 nil。不能既 emit 又返回正文。错误用 `return error(...)`、断言或 `ai.ModelError(...)` 表达，不能返回包含报错的字符串假装成功。

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

`AIGlobalConfig.RetryForever` 默认关闭，只控制可重试失败的预算是否无限；不改变失败分类。HTTP 429（有无 Retry-After 均可）、临时网络故障、接口响应结构不完整可以重试。明确的凭据错误、配置缺失、脚本错误直接返回；未知业务错误由 shouldRetry 明确选择。Agent 的动作格式纠正仍独立进行，不受普通格式纠正次数限制。

aispec.RunModelCall 是唯一请求恢复执行器。普通 Provider 与脚本 Provider 执行一次尝试；Agent 包装层使用同一执行器接入自定义 AICallback，嵌套调用不叠加重试预算。普通 Agent 请求的有限预算沿用 AiAutoRetry；独立 ai.Chat/脚本调用在未开启无限重试时默认一次。事务层不再等待/重发 HTTP 429。

HTTP 临时错误和可识别网络故障触发共享冷却，即使 RPM=0 也有效：指数退避从 2 秒增长，基数上限 5 分钟，加正向抖动；Retry-After 为最短等待时间，不受上限截断。协议错误/脚本主动选择重试的业务错误采用调用级退避，不令整个引擎进入网络故障冷却。恢复时逐个探测，旧在途成功不会解除较新冷却。

等待不占单次 HTTP 超时，但用户取消与总 deadline 始终有效。重试复用 RequestID，纠正提示词后建立新的逻辑调用。RequestID 不是幂等性保证，上游副作用需上游支持去重。脚本一次 chat 只应发送一次模型请求，禁用底层自动重试/重定向，不自行循环；任意手写多请求循环无法自动获得逐请求的配额保证。

网络恢复失败不会进入 Agent 的动作纠正提示；动作校验失败才会修改提示词。已执行的本地工具不会因模型请求重试而重放。

网络调试与这些开关相互独立，见 [网络诊断说明](../NETWORK_DEBUG.md)。

## 验证

通用模板使用本地 httptest 服务验证，无需真实密钥。覆盖请求字段映射、JSON / SSE、结束条件、reasoning 隔离、无效 JSON、截断、HTTP 429、返回错误和 emit 契约。

```sh
go test -short -tags irify_exclude ./common/ai/yakscript -count=1
go test -short -tags irify_exclude ./common/yak -run '^TestYakScriptProvider' -count=1
go test -short -tags irify_exclude ./common/ai/aispec -run '^TestGlobalModelLimit' -count=1
```
