---
name: http-packet-history
metadata:
  display_name_zh-CN: 项目与 HTTP 历史
  auto_load: "true"
description: 使用项目列表定位 Yakit、Memfit 数据库，过滤 HTTP 历史并按 ID 查看原始报文；大报文导出后用 grep/read_file 继续调查。
---

# 项目与 HTTP 历史

三个工具默认展示。先加载工具说明，再传业务参数。所有结果通过 stdout 展示原文。

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

item, err = db.QueryHTTPFlowByID(42, db.packetLimit(8192))
if err != nil { die(err) }
println(item.Dump())
files, err = item.ExportPackets("", "both")
if err != nil { die(err) }
for packetFile in files { println(packetFile.Dump()) }
```

SQL 中原有 quoted 存储由 API 解码；直接使用 dump 文本和导出文件核验证据。超限时遵循文件路径提示，避免将整个大报文搬入对话。
