# Yaklang 系统能力参考

以下能力依赖 Yaklang 引擎及 Yakit/Memfit 的运行时上下文和本地数据库。先加载相应工具说明，再传业务参数。

## 会话历史与记忆

会话、记忆空间和项目库由本次运行时绑定。当前用户指令优先于旧记忆。

| 工具 | 使用方式 | stdout 结果 |
|---|---|---|
| grep_timeline_history | query + bm25/regexp/literal；query 留空浏览，limit/offset 分页 | 逐条原始 Timeline dump，包含 history item_id、原始 timeline_item_id、类型、时间、正文和续读标记 |
| read_memory | query + bm25/vector/hybrid，limit/token_limit 控制预算 | 每条记忆的 memory_id、namespace、时间、标签、原文 |
| amend_memory | operator:add/delete/change；delete/change 必须给精确 memory_id；add/change 给 content，tags 是非空、首尾无空白且无逗号的字符串数组 | 操作信息及记忆 item dump；change 先删除旧记录和索引，再建立替换，使用返回的新 memory_id |

历史库只搜索当前会话的独立归档 item，覆盖压缩前原文，排除当前 Timeline 已有的 item。不会从摘要猜测原文；升级前已经丢弃且未归档的内容无法恢复。stdout 是直接文本，不读 RESULT/JSON，不使用 quoted 内容替代正文。truncated=true 时保持 history item_id，query 留空，用 next_content_offset 续读；offset 是条目偏移，content_offset 是正文字符偏移。

Yak 库也可以直接调用：

```yak
items, err = aihistory.Query("认证失败", aihistory.aiSession(aihistory.CurrentSession()), aihistory.searchMode("literal"))
if err != nil { die(err) }
for item in items { println(item.Dump()) }

items, err = aimemory.Search("报告偏好", aimemory.memoryNamespace(aimemory.CurrentNamespace()), aimemory.memorySearchMode("bm25"))
if err != nil { die(err) }
for item in items { println(item.Dump()) }
```

先 read_memory 回顾稳定偏好，再 grep_timeline_history 找原始决定和工具证据，依据真实记录继续调查。用 amend_memory 保存明确、可复用的事实；修改或删除后用 read_memory 回读确认，并向用户说明变化。不要把会话中的临时片段全部搬进长期记忆。

## 项目与 HTTP 历史

所有结果通过 stdout 展示原文。

| 工具 | 调用参数 | stdout 结果 |
|---|---|---|
| list_yak_projects | query 匹配名称/描述；limit/offset 分页 | database_id、名称、Yakit/Memfit/引擎来源、路径、大小、上次操作日期、current/available/supports_http |
| query_http_packet_history | database_id 可省略；query/url/methods/status_code/source_type/after_id/before_id 过滤；limit/offset 分页 | 总数、next_offset、has_more；每条 flow ID、URL、方法、状态码、小请求和小响应原文 |
| fetch_http_packet_by_id | id 必须；database_id 与列表查询一致；packet_limit 控制展示；export=true/output_dir 可选 | 小报文直接展示；大报文自动导出，返回 request_file/response_file 的绝对路径及大小 |

默认读取本次运行绑定的当前数据库。查询其他项目，先 list_yak_projects，选择 available=true 且 supports_http=true 的 database_id，后两步都传同一个 database_id。数字项目 ID、flow ID 和文件路径不能代替 database_id；不同库的 flow ID 可以重复。跨库查询只读，不切换当前项目。数据库大小包含主文件与 WAL；上次操作日期是登记更新时间与文件修改时间的较新值，不等于最后读取时间。

典型任务：找到“认证调查”项目，查询 POST /api/orders 的 400–499 历史，按返回 ID 取回报文，检查 Cookie、CSRF 与响应错误。大报文会完整保存到文件，用 grep 的 path=返回路径、pattern=CSRF、pattern-mode=substr、limit=10 查找；再用 read_file 分段查看上下文。query 的关键字过滤复用引擎 HTTPFlow 数据库过滤器，不扫描旁路文件里的大正文；此时必须 fetch 后搜索文件。报文是原始字节，可能含二进制，导出不执行其中的 fuzztag。

Yak 库调用：

```yak
projects, err = db.ListYakProjects(db.keyword("认证调查"))
if err != nil { die(err) }
for project in projects { println(project.Dump()) }

page, err = db.QueryHTTPFlows(db.url("/api/orders"), db.methods("POST"), db.statusCode("400-499"))
if err != nil { die(err) }
println(page.Dump())

// 按 ID 42 取得带展示预算的历史条目；使用工具时直接传 id=42。
page, err = db.QueryHTTPFlows(db.afterID(41), db.beforeID(43), db.limit(1), db.packetLimit(8192))
if err != nil { die(err) }
if len(page.Items) == 0 { die("HTTP flow 42 not found") }
item = page.Items[0]
println(item.Dump())
files, err = item.ExportPackets("", "both")
if err != nil { die(err) }
for packetFile in files { println(packetFile.Dump()) }
```

SQL 中原有 quoted 存储由 API 解码；直接使用 dump 文本和导出文件核验证据。超限时遵循文件路径提示，避免将整个大报文搬入对话。

已有 `db.QueryHTTPFlowByID(id)` 保留原 HTTPFlow 类型和完整存储字段，可以继续传给 `db.SaveHTTPFlowInstance`；有限展示与文件导出使用上述历史条目 API。
## MCP 服务器与客户端

MCP 配置保存于 profile 系统表 `mcp_servers`，与 Yakit 的 MCP 管理共用。AI 先 require `manage_mcp_server`，再用 `op=add/delete/list` 操作。add 不覆盖同名配置，也不连接网络或启动进程；delete 使用精确名称，清理对应工具与桥接缓存。list 包含禁用服务器；按 `has_more` 和 `next_offset` 续查，列表只展示环境变量和请求头的键。

```json
{"op":"add","name":"demo","type":"streamable_http","endpoint":"http://127.0.0.1:8080/mcp","headers":{"Authorization":"Bearer token"}}
```

`type` 支持 `stdio/sse/streamable_http`。stdio 的 endpoint 为完整命令行，支持引号和空格路径，按 argv 执行，不经过 shell；`env` 为字符串值对象。`disabled` 默认 false；true 只保存禁用配置。HTTP 的 endpoint 必须为绝对 HTTP(S) URL。

添加后，当前会话 require `query_mcp_tools`，传 `server_name`。其输出是实时的远端工具清单和完整 inputSchema；`nextCursor` 非空时传给下次的 `cursor`。再 require `call_mcp_tool`，传 `server_name`、远端裸 `remote_tool`、符合 schema 的 `arguments` 对象。两工具均支持单次请求 `timeout`（秒），调用后关闭连接；远端 `isError=true` 会输出错误内容并使工具调用失败。

```json
{"server_name":"demo","remote_tool":"echo","arguments":{"message":"hello"}}
```

Yak 的 `mcp` 标准库也能直接编写客户端：

```yak
client = mcp.Connect("demo", mcp.timeout(10))~
defer client.Close()
page = client.ListTools()~
dump(page.Tools)
// page.NextCursor 非空时，继续 client.ListTools(string(page.NextCursor))。
result = client.CallTool("echo", {"message":"hello"})~
dump(result.Content)
```

不保存配置时直接创建连接：

```yak
client = mcp.NewClient("sse", "http://127.0.0.1:8080/sse", mcp.headers({"Authorization":"Bearer token"}))~
defer client.Close()
// stdio 示例：
local = mcp.NewClient("stdio", `python3 "/tmp/my server.py"`, mcp.env({"TOKEN":"value"}))~
defer local.Close()
```

客户端还提供 `Ping()`、`ListResources(cursor?)`、`ReadResource(uri)`、`ListPrompts(cursor?)` 和 `GetPrompt(name, arguments)`。每次请求单独计时，连接不因初始化超时上下文结束而关闭。用 `mcp.context(ctx)` 绑定连接生命周期，结束时 `Close()`；重复 Close 安全。AI 工具自动绑定本次调用的上下文和 profile 库。

Yak 管理接口为 `mcp.AddServer(name,type,endpoint,options...)`、`mcp.DeleteServer(name,options...)`、`mcp.ListServers(keyword,mcp.limit(20),mcp.offset(0))`。列表不回显命令参数和凭据值。已有会话桥接连接和在途调用不会因删除配置被强行撤销；后续的脚本连接会按最新配置检查存在性和启用状态。

`DeleteServer` 和 `Ping` 仅返回 error，在 Yak 中使用 `die(mcp.DeleteServer("demo"))`、`die(client.Ping())` 检查错误；有 `(result,error)` 返回值的接口可使用 `~`。
