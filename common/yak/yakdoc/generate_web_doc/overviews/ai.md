AI 模块提供了与多种大语言模型集成的能力，支持 OpenAI、ChatGLM、Moonshot 等主流 AI 服务。通过统一的接口调用不同的 AI 服务，支持对话、函数调用、流式输出等功能。

模型配置选项可用于 `ai.Chat`，也可传给 `aim.aiConfig`、`aim.qualityPriorityAIConfig` 等 Agent 配置函数。以下选项复用前端模型配置使用的底层配置函数：

| 选项 | 用途 |
| --- | --- |
| `ai.baseURL(string)` | API 基础地址 |
| `ai.endpoint(string)`、`ai.enableEndpoint(bool)` | 设置并启用完整请求地址 |
| `ai.apiType(string)` | API 协议，如 `chat_completions` 或 `responses` |
| `ai.extraHeader(any)` | 自定义请求头，支持 map 或 `json.loads` 返回的有序映射，值必须是字符串 |
| `ai.maxTokens(int)` | 最大输出 token 数 |
| `ai.temperature(float)`、`ai.topP(float)`、`ai.topK(int)` | 采样参数 |
| `ai.frequencyPenalty(float)` | 频率惩罚 |
| `ai.reasoningEffort(string)`、`ai.thinkingLevel(string)` | 思考强度 |

不传采样选项时保留未设置状态；显式传入 `0` 会保留零值。具体参数是否支持及如何发送由模型和 API 协议决定。

```yak
options = [
    ai.apiKey("your-api-key"),
    ai.model("your-model"),
    ai.endpoint("https://example.com/v1/responses"),
    ai.enableEndpoint(true),
    ai.apiType("responses"),
    ai.extraHeader({"X-Benchmark": "test-run"}),
    ai.maxTokens(8192),
    ai.temperature(0.7),
    ai.reasoningEffort("high"),
]
qualityConfig = aim.qualityPriorityAIConfig("openai", options...)
```

## Yak 脚本模型适配

`yak-script` Provider 使用普通 Yak 运行时，通过 `chat(req, emit)` 接收调用数据。
以下函数由正式 ai 库导出，沿用普通 Yak 编辑器的补全、悬浮说明与函数签名：

- `ai.ReadSSE(reader, callback)` 只解析 SSE 事件信封，JSON 与业务字段由回调解析。
- `ai.Retry(req, reason, retryAfter)` 标记本次可恢复失败并返回 error，宿主按既有策略等待和重试。
- `ai.IsTransientError(err)` 识别 EOF、网络超时和连接重置等临时传输错误。

一次 `chat` 应只发起一次模型请求；不要在脚本中自行循环重试。完整示例见
`common/ai/yakscript/example.yak`。
