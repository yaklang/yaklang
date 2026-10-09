---
name: ai-history-memory
metadata:
  display_name_zh-CN: 会话历史与记忆
  auto_load: "true"
description: 使用 aihistory 与 aimemory 回顾当前会话压缩前原文、查询工作空间记忆，并管理明确的长期事实和偏好。
---

# 会话历史与记忆

这三个工具默认展示。先加载工具说明，再传业务参数；会话、记忆空间和项目库由本次运行时绑定。当前用户指令优先于旧记忆。

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

items, err = aimemory.Query("报告偏好", aimemory.memoryNamespace(aimemory.CurrentNamespace()), aimemory.memorySearchMode("bm25"))
if err != nil { die(err) }
for item in items { println(item.Dump()) }
```

先 read_memory 回顾稳定偏好，再 grep_timeline_history 找原始决定和工具证据，依据真实记录继续调查。用 amend_memory 保存明确、可复用的事实；修改或删除后用 read_memory 回读确认，并向用户说明变化。不要把会话中的临时片段全部搬进长期记忆。
