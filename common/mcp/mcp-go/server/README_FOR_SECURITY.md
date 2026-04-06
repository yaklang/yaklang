# MCP Server Security Notes

本文档描述当前 `common/mcp/mcp-go/server` 包内 MCP 服务端的安全边界与传输策略，只说明当前代码行为。

## 当前包含的传输方式

本目录当前包含三种服务端传输实现：

- Legacy SSE transport
- Streamable HTTP transport
- Stdio transport

其中，只有 Legacy SSE 和 Streamable HTTP 暴露 HTTP 端点。

## 当前 HTTP 端点

当前代码中的 HTTP 端点只有以下三个：

- `/sse`
- `/message`
- `/mcp`

说明如下：

- `/sse` 是 Legacy SSE transport 的事件流入口。
- `/message` 是 Legacy SSE transport 的 JSON-RPC 写入口。
- `/mcp` 是 Streamable HTTP transport 的统一入口。

本目录当前没有其他 HTTP 端点。

`stdio` 传输存在于本目录中，但不通过 HTTP 暴露，不参与浏览器跨域问题。

## Legacy SSE 方案

### `/sse`

`/sse` 只接受 `GET`。

当前行为：

- 响应头固定为 `text/event-stream`
- 设置 `Access-Control-Allow-Origin: *`
- 为每个连接分配独立 `sessionId`
- 建立连接后先下发一个 `endpoint` 事件，内容是带 `sessionId` 的 `/message` URL

允许使用范围：

- 浏览器页面可以跨源连接 `/sse`
- 浏览器扩展可以连接 `/sse`
- 非浏览器客户端也可以连接 `/sse`

安全边界：

- `/sse` 本身是读通道，用于建立会话和接收服务端事件
- `/sse` 不接收 JSON-RPC 写请求
- `/sse` 的跨源放开不等于 `/message` 可以被任意跨站写入

### `/message`

`/message` 只接受 `POST` 和 `OPTIONS`。

当前行为：

- `OPTIONS` 用于浏览器预检
- `POST` 用于发送 JSON-RPC 消息
- 必须携带查询参数 `sessionId`
- 必须携带合法的 `Content-Type: application/json`
- 请求体必须是合法 JSON

当前来源控制策略：

- 允许无 `Origin` 的请求
- 允许本地网页来源：`localhost`、`127.0.0.1`、`::1`
- 允许浏览器扩展来源：`chrome-extension://`、`moz-extension://`、`safari-web-extension://`
- 拒绝 `Origin: null`
- 拒绝任意非本地网站来源

这意味着：

- 非浏览器客户端默认不带 `Origin`，当前可以正常使用 `/message`
- 本地页面可以调用 `/message`
- 浏览器扩展可以调用 `/message`
- 远程站点不能直接通过浏览器向本机 `/message` 发起合法跨域写请求

当前 CORS 行为：

- 合法来源的 `OPTIONS` 预检会返回 `204 No Content`
- 合法来源会回显 `Access-Control-Allow-Origin`
- 预检允许的方法为 `POST, OPTIONS`
- 预检允许的请求头为 `Content-Type`
- 非法来源返回 `403 Forbidden`

### Legacy SSE 下的工具暴露限制

当前代码对 Legacy SSE transport 做了额外工具级限制。

限制内容：

- `tools/list` 不会暴露 `exec_yak_script`
- `tools/list` 不会暴露 `dynamic_add_tool`
- `tools/call` 会拒绝调用 `exec_yak_script`
- `tools/call` 会拒绝调用 `dynamic_add_tool`

允许使用范围：

- Legacy SSE 仍可正常使用一般 MCP 能力
- Legacy SSE 下明确不允许上述两类高风险工具

## Streamable HTTP 方案

Streamable HTTP transport 的默认路径是 `/mcp`。

`/mcp` 支持以下 HTTP 方法：

- `GET`
- `POST`
- `DELETE`
- `OPTIONS`

### `/mcp` `POST`

`POST /mcp` 是 Streamable HTTP 的 JSON-RPC 写入口。

当前行为：

- 严格要求 `Content-Type: application/json`
- 接受标准参数形式，例如 `application/json; charset=utf-8`
- 空请求体会被拒绝
- 非法 JSON 会被拒绝
- `initialize` 请求不能携带已有 session ID
- 非 `initialize` 请求必须携带有效 session ID
- 如提供协议版本头，则会校验版本合法性和与当前 session 一致性

初始化成功后：

- 服务端创建新的 session
- 响应头返回 MCP session ID
- 响应头返回协商后的协议版本

### `/mcp` `GET`

`GET /mcp` 用于建立事件流。

当前行为：

- 客户端必须接受 `text/event-stream`
- 必须携带有效 session ID
- 如提供协议版本头，则会校验其与 session 一致
- 建立成功后返回 SSE 数据流

### `/mcp` `DELETE`

`DELETE /mcp` 用于关闭已有 session。

当前行为：

- 必须携带有效 session ID
- 如提供协议版本头，则会校验其与 session 一致
- 成功后关闭 session 并返回 `204 No Content`

### `/mcp` `OPTIONS`

当前会返回允许的方法声明：

- `GET, POST, DELETE, OPTIONS`

### Streamable HTTP 的当前安全边界

当前 Streamable HTTP transport 的加固重点是协议与内容校验：

- 严格 JSON MIME 校验
- session 约束
- 协议版本约束
- 请求体合法性约束

当前代码没有对 `/mcp` 增加与 `/message` 相同的 `Origin` 白名单限制。

因此，当前 `/mcp` 与 Legacy SSE 的安全模型并不完全相同：

- `/message` 额外做了浏览器来源限制
- `/mcp` 当前主要做协议面和内容面的严格校验

## 允许使用的范围

从当前代码看，允许使用范围可以概括为：

- 非浏览器客户端可以使用 Legacy SSE 的 `/message`，前提是发送合法 JSON 请求
- 本地网页可以使用 Legacy SSE 的 `/sse` 和 `/message`
- 浏览器扩展可以使用 Legacy SSE 的 `/sse` 和 `/message`
- 远程网站可以连接 `/sse`，但不能合法写入 `/message`
- Streamable HTTP 客户端可以使用 `/mcp`，前提是满足 session、协议版本和 JSON 内容要求

## 当前加固范围

当前代码已经做的加固主要包括：

- Legacy SSE `/message` 严格要求 `Content-Type: application/json`
- Streamable HTTP `/mcp` `POST` 严格要求 `Content-Type: application/json`
- Legacy SSE `/message` 对浏览器来源做白名单限制
- Legacy SSE `/message` 对非法来源预检直接拒绝
- Legacy SSE transport 隐藏并拒绝 `exec_yak_script`
- Legacy SSE transport 隐藏并拒绝 `dynamic_add_tool`

## 当前测试策略

本目录当前使用单元测试覆盖 HTTP 传输的成功路径与拒绝路径。

### Legacy SSE 测试

`sse_test.go` 当前覆盖的重点包括：

- `/sse` 能建立会话并返回消息端点
- `/message` 能正常完成初始化请求
- 多会话并发处理
- 拒绝 `text/plain`
- 拒绝缺失 `Content-Type`
- `/sse` 保持 wildcard CORS
- 允许本地页面来源访问 `/message`
- 允许浏览器扩展来源访问 `/message`
- 允许无 `Origin` 的客户端访问 `/message`
- 拒绝远程网站来源访问 `/message`
- 允许本地来源通过 `OPTIONS` 预检
- 拒绝远程网站来源的预检
- Legacy SSE 下隐藏受限工具
- Legacy SSE 下拒绝受限工具调用

### Streamable HTTP 测试

`streamable_http_test.go` 当前覆盖的重点包括：

- `POST /mcp` 初始化成功
- 接受 `application/json; charset=utf-8`
- 拒绝 `text/plain`
- 拒绝缺失 `Content-Type`

### 服务端分发测试

`server_test.go` 当前覆盖的重点包括：

- Legacy SSE transport 下 `tools/list` 的过滤行为
- Legacy SSE transport 下 `tools/call` 的拒绝行为
- Streamable HTTP transport 下受限工具仍保持可用
- 非 transport 上下文下不误伤正常工具能力

## 结论

当前代码对 Legacy SSE 和 Streamable HTTP 采用的是两套不同但明确的安全边界：

- Legacy SSE 保持浏览器可接入，允许跨源读取 `/sse`，但把 `/message` 作为受保护写入口处理，并额外收敛高风险工具
- Streamable HTTP 保持标准 MCP HTTP 语义，重点收敛在 JSON、session 和协议版本校验上

如果从当前实现出发理解 HTTP 暴露面，可以简单记成：

- `/sse` 是开放读通道
- `/message` 是受来源限制的 Legacy SSE 写通道
- `/mcp` 是受协议和内容限制的 Streamable HTTP 通道