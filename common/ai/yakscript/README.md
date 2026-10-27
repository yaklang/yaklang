# Yak 脚本模型 Provider

`yak-script` 是通用模型适配宿主。客户协议放在用户提供的可信脚本中，不内置客户端点、业务字段或适配模板。它以正常 Yak 运行时权限执行，每次尝试创建新的 VM；VM 隔离不等于操作系统权限沙箱。

## 三层职责

| 层 | 职责 |
| --- | --- |
| Go 宿主 | 创建 VM、注入接口、传递取消与超时、管理全局请求频率和恢复、把正文接入 StreamHandler / AIResponse |
| Yak 脚本 | HTTP 请求映射、状态码与媒体类型检查、JSON / SSE 内部业务字段解析、完整性验证，交付正文或错误 |
| Agent | 解析正文中的 `@action` / answer_payload、纠正格式并执行工具 |

`data:` 和空行是 SSE 格式；`choices[].delta.content` 是模型接口格式；正文中的 `@action` 是 Agent 协议。HTTP 200、合法 SSE 或遇到 `[DONE]` 都不能单独证明业务成功。

## 模型配置

选择 Provider `yak-script`，填写源码 `ExtraParams[script]` 和单次超时 `ExtraParams[script_timeout_seconds]`。连接配置使用 BaseURL / Endpoint / APIKey / Headers / Proxy。业务选项直接定义在源码中，没有独立脚本参数 JSON。

[内置通用示例](example.yak) 演示文本 Chat Completions：BaseURL 包含 API 前缀（例如 `https://model.example/v1`），脚本追加 `/chat/completions`；启用 Endpoint 时直接使用完整 URL。模型名称填上游模型 ID。源码中的 `mode = "json"` 可改为 `"sse"`，两种模式均完整验证后返回正文。标准接口通常直接使用内置普通 Provider；此示例用于说明如何编写适配。

界面的“载入通用 Provider 示例”只填写源码和默认超时，不覆盖地址、模型名或凭据。仅更新客户端或引擎不会改写已保存的用户脚本。

## 宿主注入的接口

入口是 `chat(req, emit)`，不需要自行 import 宿主对象。

| req 字段 | 含义 |
| --- | --- |
| Context | 本次尝试的取消 / 超时上下文，HTTP 调用必须传 `poc.context(req.Context)` |
| RequestID | 逻辑模型调用 ID，宿主重试复用，新模型调用重新生成；不代表上游自动去重 |
| Prompt | 上层构造的完整提示词，可能包含历史、动作规范和工具结果，不要仅截取最后一句 |
| Model | 模型名称 |
| BaseURL / Endpoint / EnableEndpoint | 连接配置；路径拼接规则由脚本决定 |
| APIKey / Proxy / Headers | 凭据、代理、附加请求头 |
| Timeout | 单次尝试秒数，从获准请求后开始；排队不消耗此期限 |
| RawMessages | 显式消息列表；内置示例明确拒绝它，避免无声丢失角色 / 多模态信息 |

| 注入函数 | 用法和边界 |
| --- | --- |
| `emit(text)` | 交付模型正文片段；用 `emit(part)~` 传播失败，最后 `return nil` |
| `provider.ReadSSE(reader, callback)` | 只解析 SSE 信封；回调接收字符串 `(event, data)`，JSON 和业务字段仍由脚本解析 |
| `provider.Retry(req, reason, retryAfter)` | 标记本次可恢复失败并返回 error；应 `return provider.Retry(...)`；不会在脚本内 sleep / 重发 |
| `provider.IsTransientError(err)` | 检查 EOF、网络超时、连接重置等传输失败；不认识 HTTP 状态或业务错误码 |

`retryAfter` 接受 HTTP Retry-After 秒数或 HTTP 日期字符串，空字符串表示没有上游等待提示。错误原因应简短，避免把凭据和原始报文塞入 UI 错误文本。

### 输出契约

二选一：返回非空正文字符串，或者多次 `emit(text)~` 后返回 nil。不能既 emit 又返回正文。错误用 `return error(...)`、断言或 `provider.Retry(...)` 表达，不能返回包含报错的字符串假装成功。

只交付模型正文：不返回完整 HTTP / SSE 信封、choices 包装、进度或错误页；不在脚本里补造 `@action`。动作格式校验留在 Agent。

目前脚本没有单独的思考 emitter，也没有 usage / native tool calls / 多模态输出 ABI。通用示例忽略 reasoning_content，不把思考拼入正文。标准 Provider 的独立 ReasonStreamHandler 不等于脚本自动获得这一能力。

开启全局无限重试时，宿主先缓冲 emit，只有尝试成功才提交，失败尝试的残缺正文被丢弃。关闭时，emit 按到达顺序交付；已交付文本无法撤回，宿主不重放已经输出部分正文的失败尝试。需要严格完整性的适配优先采用“校验后 return”。

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
