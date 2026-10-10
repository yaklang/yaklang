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
