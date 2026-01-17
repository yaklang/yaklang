# AI Balance

AI Balance 是一个高性能的 AI 模型负载均衡和代理服务，支持多提供商、多模型的智能路由和健康检查。

## 功能特性

### 核心功能

- **多提供商支持**：支持多个 AI 服务提供商（OpenAI、智谱 GLM、通义千问、Gemini、DeepSeek 等）
- **智能负载均衡**：基于延迟的加权随机选择算法，自动选择最优提供商
- **健康检查机制**：实时监控提供商健康状态，自动剔除故障节点
- **流式响应支持**：完整的 SSE (Server-Sent Events) 流式响应处理
- **Tool Calls 支持**：正确处理和转发 AI 模型的 function calling / tool_calls 响应
- **推理内容支持**：支持 AI 模型的推理内容（reasoning_content）分离输出
- **API Key 管理**：支持 API Key 权限控制和模型访问限制
- **Memfit 认证**：支持基于 TOTP 的 Memfit 模型访问控制

### 高级特性

- **延迟监控**：实时记录每个提供商的请求延迟
- **故障自动切换**：当提供商不可用时自动切换到备用提供商
- **统计信息**：记录请求成功率、输入输出字节数等统计信息
- **HTTP 代理**：支持 HTTPS 代理和自定义域名
- **连接池**：使用连接池提高性能

## API 接口

### 聊天补全

**端点**: `POST /v1/chat/completions`

**请求头**:
```
Authorization: Bearer <api_key>
```

**请求体**:
```json
{
  "model": "gpt-4",
  "messages": [
    {"role": "user", "content": "Hello"}
  ],
  "stream": true
}
```

**响应** (SSE 流式):
```
data: {"id":"chat-ai-balance-xxx","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chat-ai-balance-xxx","choices":[{"finish_reason":"stop"}]}

data: [DONE]
```

### 嵌入向量

**端点**: `POST /v1/embeddings`

### 模型列表

**端点**: `GET /v1/models`

### 管理界面

**端点**: `GET /portal`

## 负载均衡策略

1. 过滤出健康的提供商（延迟 < 10 秒）
2. 使用延迟的倒数作为权重
3. 基于权重进行随机选择
4. 故障时自动切换到下一个提供商

## 许可证

MIT License

