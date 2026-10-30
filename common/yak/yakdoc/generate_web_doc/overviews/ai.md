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


## 自定义模型 Provider

模型脚本使用 `chat(req, emit)`，可定义 `shouldRetry(req, failure)`。编辑器使用
普通 Yak 模式，复用正式库导出的补全、悬浮文档和函数签名。公开工具函数如下：

- `ai.ReadSSE(reader, callback)` 只解码 SSE 事件信封；回调解析上游模型的 JSON。
- `ai.ModelError(req, kind, status, code, message, retryAfter)` 报告一次失败，不返回错误正文给 Agent。
- `ai.Retry(reason, retryAfter)`、`ai.StopRetry(reason)` 返回重试决策，供 shouldRetry 使用；nil 使用默认策略。
- `ai.IsTransientError(err)` 判断网络中断、超时等错误。

脚本不自行循环重试。aispec 统一负责请求配额、Retry-After、退避和取消，Agent
独立负责动作输出校验和纠正。完整示例见 `common/ai/yakscript/example.yak`。
