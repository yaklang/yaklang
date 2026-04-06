# MCP Server Security Advisory

本文档用于说明 `common/mcp/mcp-go/server` 当前 HTTP 传输面的安全边界与默认限制策略。

本文档只描述当前代码行为。

## 摘要

当前 MCP 服务端在本目录内提供三种传输方式：

- Legacy SSE
- Streamable HTTP
- Stdio

其中，HTTP 暴露面仅包含以下三个端点：

- `/sse`
- `/message`
- `/mcp`

当前安全策略的核心目标是：

- 保留浏览器与客户端对 MCP 的正常接入能力
- 将 Legacy SSE 的写入口与读入口分离处理
- 对 JSON-RPC 写请求实施更严格的内容类型和来源限制
- 对 Legacy SSE 额外收敛高风险工具暴露范围

## 当前暴露面

### `/sse`

用途：Legacy SSE 的事件流入口。

当前特点：

- 仅接受 `GET`
- 返回 `text/event-stream`
- 返回 `Access-Control-Allow-Origin: *`
- 为连接分配独立 `sessionId`
- 通过 `endpoint` 事件告知对应的 `/message?sessionId=...` 写入口

当前定位：

- `/sse` 是读通道
- `/sse` 允许浏览器跨源接入
- `/sse` 本身不接收 JSON-RPC 写请求

### `/message`

用途：Legacy SSE 的 JSON-RPC 写入口。

当前特点：

- 仅接受 `POST` 和 `OPTIONS`
- 必须提供 `sessionId`
- 必须使用 `Content-Type: application/json`
- 请求体必须是合法 JSON
- 非法来源返回 `403`

当前来源策略：

- 允许无 `Origin` 的请求
- 允许 `localhost`
- 允许 `127.0.0.1`
- 允许 `::1`
- 允许浏览器扩展来源：`chrome-extension://`、`moz-extension://`、`safari-web-extension://`
- 拒绝 `Origin: null`
- 拒绝任意非本地网站来源，例如 `Origin: https://example.com/` 或 `Origin: example.com` 

当前定位：

- `/message` 是受保护写通道
- 浏览器本地页面可以使用
- 浏览器扩展可以使用
- 正常非浏览器客户端可以使用
- 远程网站不能通过浏览器合法写入本机 `/message`

### `/mcp`

用途：Streamable HTTP transport 的统一入口。

当前支持方法：

- `GET`
- `POST`
- `DELETE`
- `OPTIONS`

其中：

- `POST /mcp` 用于 JSON-RPC 写请求
- `GET /mcp` 用于建立事件流
- `DELETE /mcp` 用于关闭 session

当前 `POST /mcp` 限制：

- 必须使用 `Content-Type: application/json`
- 允许标准参数形式，例如 `application/json; charset=utf-8`
- 空请求体会被拒绝
- 非法 JSON 会被拒绝
- `initialize` 不能携带已有 session ID
- 非初始化请求必须携带有效 session ID
- 如提供协议版本头，则必须合法且与 session 一致

当前定位：

- `/mcp` 是标准化 Streamable HTTP 通道
- 当前重点加固的是 JSON、session 和协议版本约束
- 当前没有引入与 `/message` 相同的 `Origin` 白名单策略

## 当前工具暴露限制

Legacy SSE transport 当前额外限制以下工具：

- `exec_yak_script`
- `dynamic_add_tool`

当前行为：

- `tools/list` 不会暴露上述工具
- `tools/call` 会拒绝调用上述工具

该限制仅针对 Legacy SSE transport。

当前代码下：

- Streamable HTTP transport 仍保留这些工具的正常可见性与可调用性
- 无 transport 限定的直接服务端调用也不会被误伤

## 当前允许使用范围

从当前实现出发，可以这样理解允许范围：

- 浏览器页面可以跨源连接 `/sse`
- 本地页面可以访问 `/message`
- 浏览器扩展可以访问 `/message`
- 非浏览器客户端在不携带 `Origin` 的情况下可以访问 `/message`
- Streamable HTTP 客户端可以访问 `/mcp`

从当前实现出发，可以这样理解限制范围：

- 任意网站都不能通过浏览器向 `/message` 发起合法跨域写请求
- `text/plain` 不能作为 `/message` 或 `POST /mcp` 的内容类型
- 缺失 `Content-Type` 不能通过 `/message` 或 `POST /mcp`
- Legacy SSE 不能列出或调用 `exec_yak_script` 与 `dynamic_add_tool`

## 当前测试结论

当前安全行为由本目录单元测试覆盖。

### SSE 覆盖点

`sse_test.go` 当前验证：

- `/sse` 可建立会话
- `/message` 可完成正常初始化
- `/message` 拒绝 `text/plain`
- `/message` 拒绝缺失 `Content-Type`
- `/sse` 保持 wildcard CORS
- `/message` 允许 localhost 来源
- `/message` 允许浏览器扩展来源
- `/message` 允许无 `Origin` 的客户端请求
- `/message` 拒绝远程网站来源
- `/message` 对本地来源允许预检
- `/message` 对远程网站来源拒绝预检
- Legacy SSE 隐藏受限工具
- Legacy SSE 拒绝受限工具调用

### Streamable HTTP 覆盖点

`streamable_http_test.go` 当前验证：

- `POST /mcp` 初始化成功
- 接受 `application/json; charset=utf-8`
- 拒绝 `text/plain`
- 拒绝缺失 `Content-Type`

### 分发层覆盖点

`server_test.go` 当前验证：

- Legacy SSE 下工具列表过滤生效
- Legacy SSE 下工具调用拒绝生效
- Streamable HTTP 下受限工具保持可用
- 非 transport 上下文下工具行为不被误伤
