---
name: yaklang-system
description: Yaklang/Yakit/Memfit 专用系统能力：回顾压缩前的会话历史、管理工作空间记忆、识别本地项目并查询 HTTP 历史。依赖引擎上下文与数据库。
metadata:
  display_name_zh-CN: Yaklang 系统能力
  auto_load: "true"
---

# Yaklang 系统能力

本 Skill 统一描述 Yaklang 引擎与 Yakit/Memfit 的内部能力，依赖本次运行绑定的会话、记忆空间和项目数据库。先加载工具说明，再传业务参数；结果通过 stdout 的原始文本 dump 或导出文件读取。

## 按任务选择工具

| 需要做什么 | 工具 | 得到什么 |
|---|---|---|
| 回顾当前会话压缩前的决定、消息或工具证据 | `grep_timeline_history`：query，bm25/regexp/literal，limit/offset | 独立归档的历史 item dump，含 ID、时间和原文；排除当前 Timeline 已有 item |
| 回顾当前工作空间的长期事实和偏好 | `read_memory`：query，bm25/vector/hybrid，limit/token_limit | memory_id、namespace、时间、标签和原文 |
| 增加、删除或替换明确的长期记忆 | `amend_memory`：operator:add/delete/change，memory_id/content/tags | 操作信息和记忆 dump；change 先删后建，返回新的 memory_id |
| 定位引擎识别的 Yaklang/Yakit/Memfit 项目 | `list_yak_projects`：query，limit/offset | database_id、名称、来源、路径、大小、上次操作日期及可用状态 |
| 筛选当前库或其他项目的 HTTP 流量 | `query_http_packet_history`：database_id 可选，query/url/methods/status_code 等 | 分页结果、flow ID、URL、方法、状态码及预算内的请求/响应原文 |
| 取回指定 HTTP 流量的完整证据 | `fetch_http_packet_by_id`：id，database_id 可选，packet_limit/export | 小报文直接展示；大报文返回 request_file/response_file，继续用 grep/read_file 查阅 |

## 使用顺序与边界

- 接续旧任务：先 `read_memory` 回顾稳定偏好，再 `grep_timeline_history` 找原始证据。当前用户指令优先于旧记忆；历史搜索覆盖已归档的压缩前 item，无法恢复未归档的旧原文。
- 管理记忆：delete/change 使用精确 memory_id；add/change 提供 content。修改后回读确认，只保存明确且可复用的事实与偏好。
- 查询流量：省略 database_id 读取当前数据库。跨项目先列项目，选择 available=true 且 supports_http=true 的 database_id，查询和取包使用同一 database_id；操作只读，不切换当前项目。
- 读取长内容：历史 truncated=true 时按 history item_id 与 next_content_offset 续读。大 HTTP 正文先取包导出，再按返回的绝对路径搜索文件；数据库关键字查询不扫描旁路文件。直接读取 dump，避免 quoted/JSON 失真。

## 详细参考与扩展位置

需要完整参数、分页语义或 `aihistory.Query`、`aimemory.Search`、`db.ListYakProjects`、`db.QueryHTTPFlows` 等 Yak 库示例时，通过 `load_skill_resources` 读取 `@yaklang-system/reference.md`（[详细参考](reference.md)）。

后续新增依赖 Yaklang/Yakit/Memfit 运行时或本地库的系统能力时，在这里补充工具入口，将完整参数和案例放到参考资源。可跨系统复用的领域方法和 FuzzTag 用法保持各自的 Skill。
