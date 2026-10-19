---
name: yaklang-system
description: Yaklang/Yakit/Memfit 专用系统能力：回顾会话历史、管理工作空间记忆、查询项目与 HTTP 历史、管理和查询知识库，以及管理 MCP 服务器并调用远端工具。依赖引擎上下文与数据库。
metadata:
  display_name_zh-CN: Yaklang 系统能力
  auto_load: "true"
---

# Yaklang 系统能力

本 Skill 统一描述 Yaklang 引擎与 Yakit/Memfit 的内部能力，依赖本次运行绑定的会话、记忆空间和项目数据库。先加载工具说明，再传业务参数；`require_tool_payload` 每次只传一个工具名字符串，需要多个工具时分别加载，不传数组或拼接名称。结果通过 stdout 的原始文本 dump 或导出文件读取。

## 按任务选择工具

| 需要做什么 | 工具 | 得到什么 |
|---|---|---|
| 回顾当前会话压缩前的决定、消息或工具证据 | `grep_timeline_history`：query，bm25/regexp/literal，limit/offset | 独立归档的历史 item dump，含 ID、时间和原文；排除当前 Timeline 已有 item |
| 回顾当前工作空间的长期事实和偏好 | `read_memory`：query，bm25/vector/hybrid，limit/token_limit | memory_id、namespace、时间、标签和原文 |
| 增加、删除或替换明确的长期记忆 | `amend_memory`：operator:add/delete/change，memory_id/content/tags | 操作信息和记忆 dump；change 先删后建，返回新的 memory_id |
| 定位引擎识别的 Yaklang/Yakit/Memfit 项目 | `list_yak_projects`：query，limit/offset | database_id、名称、来源、路径、大小、上次操作日期及可用状态 |
| 筛选当前库或其他项目的 HTTP 流量 | `query_http_packet_history`：database_id 可选，query/url/methods/status_code 等 | 分页结果、flow ID、URL、方法、状态码及预算内的请求/响应原文 |
| 取回指定 HTTP 流量的完整证据 | `fetch_http_packet_by_id`：id，database_id 可选，packet_limit/export | 小报文直接展示；大报文返回 request_file/response_file，继续用 grep/read_file 查阅 |
| 管理与 Yakit 共享的 MCP 服务器 | `manage_mcp_server`：op:add/delete/list，name/type/endpoint/env/headers，limit/offset | 持久配置、启用状态与分页列表；凭据值不在列表回显 |
| 发现并调用外部 MCP 工具 | `query_mcp_tools`：server_name/cursor；`call_mcp_tool`：server_name/remote_tool/arguments | 远端工具原始 inputSchema 与 MCP content，刚添加即可调用 |
| 列出、增删改查和构建知识库，管理知识条目 | `manage_knowledge_base`：op:list/get/create/update/delete/build/add_entry/update_entry/delete_entry/rebuild_index，name，metadata/index_options/build_options | 与 Yakit 共用的 profile 知识库元信息、分页列表、条目 ID 和实际构建结果 |
| 查询指定知识库的知识正文 | `query_knowledge_base`：name、token_limit 必填；mode:keyword/semantic/get，query/entry_id，limit/offset/content_offset | 库内原文、条目 ID、score、token 用量与截断续读位置；不隐式跨库 |

## 使用顺序与边界

- 接续旧任务：先 `read_memory` 回顾稳定偏好，再 `grep_timeline_history` 找原始证据。当前用户指令优先于旧记忆；历史搜索覆盖已归档的压缩前 item，无法恢复未归档的旧原文。
- 管理记忆：delete/change 使用精确 memory_id；add/change 提供 content。修改后回读确认，只保存明确且可复用的事实与偏好。
- 查询流量：省略 database_id 读取当前数据库。跨项目先列项目，选择 available=true 且 supports_http=true 的 database_id，查询和取包使用同一 database_id；操作只读，不切换当前项目。
- 读取长内容：历史 truncated=true 时按 history item_id 与 next_content_offset 续读。大 HTTP 正文先取包导出，再按返回的绝对路径搜索文件；数据库关键字查询不扫描旁路文件。直接读取 dump，避免 quoted/JSON 失真。
- 管理 MCP：先 `require_tool` 加载 `manage_mcp_server`；`op=list` 检查名称，`op=add` 保存新配置，`op=delete` 删除精确名称并清理工具缓存。配置存在本次运行的 profile 系统表，与 Yakit 共用；重复添加报错。禁用 MCP 的运行不能使用这些工具。
- 使用 MCP：添加后直接 require `query_mcp_tools` 读取远端工具参数，按 `nextCursor` 续查；再 require `call_mcp_tool`，传远端裸工具名和 JSON 参数对象，当前会话即可执行。也可用 `mcp.Connect` 或 `mcp.NewClient` 编写 Yak 客户端。操作失败时读取实际错误，不声称调用成功；完整用法见参考资源的 MCP 一节。
- 管理知识库：先 `require_tool` 加载 `manage_knowledge_base`；`op=list` 定位名称，再按需创建、更新元信息、构建和管理条目。create 配置模型/维度/HNSW，build 配置输入、分块和并发；构建需等待实际落库。知识库列表属于管理，不单设列表工具。
- 查询知识：require `query_knowledge_base`，显式给出 name 和 token_limit。keyword 可分页浏览，semantic 使用该库语义索引，get 按 entry_id 精确续读；截断后用 next_content_offset 继续。旧 RAG action 和开关已停用，知识操作通过本 Skill 引导调用工具；`aiknowledge.Manage` / `aiknowledge.Query` 提供直接 Yak API。

## 详细参考与扩展位置

需要完整参数、分页语义或 `aihistory.Query`、`aimemory.Search`、`db.ListYakProjects`、`db.QueryHTTPFlows` 等 Yak 库示例时，通过 `load_skill_resources` 读取 `@yaklang-system/reference.md`（[详细参考](reference.md)）。

后续新增依赖 Yaklang/Yakit/Memfit 运行时或本地库的系统能力时，在这里补充工具入口，将完整参数和案例放到参考资源。可跨系统复用的领域方法和 FuzzTag 用法保持各自的 Skill。
