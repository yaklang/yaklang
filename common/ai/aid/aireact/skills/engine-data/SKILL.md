---
name: engine-data
metadata:
  display_name_zh-CN: 引擎数据检索与管理
  auto_load: "true"
description: 查询和管理当前 Yakit 工作空间的记忆、会话历史、项目、HTTP 流量、风险、知识和 payload，依据已有记录继续调查或生成 HTTP 测试。
---

# 引擎数据检索与管理

这些工具默认出现在能力列表。先加载所需工具说明，再传业务参数；结果中的 ID 用于精确回读、修改或删除。会话、记忆空间和数据库由运行时绑定，当前用户要求优先于旧记忆。没有命中时说明查询条件和范围，不能把空结果当作结论。

| 工具 | 如何使用 | 结果与下一步 |
|---|---|---|
| `read_memory` | 用 `query` 回顾偏好、事实、决定；`search_mode` 可选 bm25/vector/hybrid | `namespace`、`hits`，每条含 `memory_id`、正文、标签和时间；按来源继续工作 |
| `amend_memory` | `operator:add` + content/tags；delete 用 memory_id；change 用 memory_id 和完整新 content | 当前空间的 memory_id；change 先删旧记录及索引再创建替换，记住返回的新 ID |
| `grep_timeline_history` | query + bm25/regexp/literal；留空分页浏览；结构化原值用 `view:value_json` | 当前会话的 item_id、历史正文及时间；压缩后也能回顾已存档原文 |
| `query_yak_projects` | 可用 keyword 搜索项目，limit/offset 分页 | project_id、数据库路径、当前项目标记、yakit_home；将登记 ID 传给 HTTP/风险查询 |
| `query_http_history` | keyword/url/methods/status_code；高级条件用 filter_json；flow_id 回读报文 | 流量元数据、请求/响应分段；project_id=0 是当前项目，其他登记项目使用只读连接 |
| `query_cybersecurity_risk` | keyword/severity/network/ports；risk_id 回读证据 | 风险信息、证据 JSON、http_flow_ids；使用相同 project_id 回读关联流量，保留已读状态 |
| `query_knowledge` | source=collections 找知识库；entries 搜原文；vector_collections/documents 查已有索引 | knowledge_base_id、entry_id/entry_uuid、正文与来源；利用证据推进分析 |
| `manage_knowledge` | knowledge_base_id 或精确 collection；add 传 title/content；delete/change 精确定位条目 | 新 ID/UUID、清理索引数；change 完整替换、失败回滚，不继承遗漏元数据 |
| `query_payloads` | 不传 group 列组，传 group/query 搜内容，payload_id 精确读取 | 组、条目 ID、样本、editable 和 usage；使用返回的原生 payload FuzzTag |
| `manage_payloads` | add 用 group + contents 数组；change 用单个 payload_ids + 新 contents；delete 精确 IDs；delete_group 删除组 | affected、条目 ID 和 FuzzTag 引用；保留每个数组元素的逗号、换行、空白及标签文字 |

## 连续任务

**恢复调查进度**：先 `read_memory` 查稳定偏好，再 `grep_timeline_history` 查本会话之前的决定和工具结果。返回 item_id 后精确续读，引用实际记录，不从摘要猜原文。升级前已丢弃的原文无法恢复。

**风险到流量证据**：`query_yak_projects` 选库 → `query_cybersecurity_risk` 搜候选 → risk_id 回读证据 → `query_http_history` 用关联 flow_id 回读报文。每一步保持 project_id，一次改变一个过滤条件。风险信息是已存记录，仍需根据请求响应判断证据是否充分。

**维护调查知识**：`query_knowledge` 找知识库和条目 → `manage_knowledge` 保存或替换笔记 → 用返回的新 ID 再查询验证。add/change 的原文立即可搜索，`semantic_index_status:pending` 表示新语义向量尚未生成，需走 Yakit 正常索引流程。change/delete 会清理旧问题/向量文档和图缓存。

**字典驱动 HTTP 测试**：`query_payloads` 找组 → 必要时 `manage_payloads` 添加原始载荷 → `do_http_request` 设置 `fuzztag:true`，例如 `query:{"q":"{{payload(api_cases)}}"}`、`max-requests:20`。使用单数 payload；`payload:nodup` 去重，`payload:full` 保留每条完整多行内容，嵌套编码如 `{{base64({{payload(api_cases)}})}}`。URL 模式 query/form 传原始值，工具执行一次编码；多个标签默认取笛卡尔积，同层 `::row` 可同步配对。先保留正常基线，再比较响应与实际请求值。

## 回读和管理边界

正文有 `truncated` 和 `next_content_offset` 时，保持原 ID 和筛选范围，用 content_offset 续读；offset 是条目分页，content_offset 是正文字符偏移。风险 content 为 JSON 时拼接完整后解析。每页有限额，截断样本不要作为完整载荷发送。

记忆、知识、payload 的删除使用精确 ID；知识还必须属于选中知识库。文件型 payload 返回 editable=false，只读样本；delete_group 只移除登记，不删除磁盘文件。修改后回读确认，并向用户说明具体改变的条目。
