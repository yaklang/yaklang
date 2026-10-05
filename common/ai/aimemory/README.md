# aimemory

独立的记忆检索库；搜索和选项都在 `aimemory` 中。

```yak
memories = aimemory.SearchMemory(
    "报告格式偏好",
    aimemory.memoryNamespace("default"),
    aimemory.memoryTokenLimit(1500),
    aimemory.memorySearchMode("hybrid"),
    aimemory.memoryLimit(5),
)~
```

`namespace` 就是 memory 集 ID，对应 memory 表中的 `session_id` 和 RAG 集合 `ai-memory-<id>`。数据来自宿主绑定的项目数据库；只用一个 ID，不引入 workspace。AI 工具通过 `aimemory.CurrentNamespace()` 取得运行时实际使用的 memory 集，独立 Yak 脚本默认 `default`。持久化会话 ID 与 memory 集 ID 没有通用的等价关系，已有会话表也没有保存这种映射。

`SearchMemory` 返回现有的 `AIMemoryEntity` 数据行，使用现有 RAG BM25/语义向量查询和 memory 表查询接口。支持 `bm25`、`vector`、`hybrid`；问句索引之外补充正文/标签关键词匹配，合并排名、去重并过滤过期和已删除记录。没有匹配时返回空数组。搜索不初始化 triage、不创建集合、不触发索引重建；hybrid 向量不可用时保留关键词结果并记录英文诊断，vector 返回错误。上下文取消在检索前后检查；向量请求沿用底层 embedding 服务的超时机制。

默认最多 5 条，范围 1–20；默认正文总预算 1500 tokens，范围 64–8192。超长正文可能截断。检索结果的字段选择、标签和时间展示统一在 `search_memory.yak` 中维护，普通工具与 smart_qa 的 memory action 都执行这个脚本，结果通过普通工具 Timeline 保存。

自动 injection 独立复用现有 `SearchMemoryWithoutAI`，只在意图识别落地时执行一次；显式工具搜索不会更新其快照。本轮不调整 triage 写入策略。

本地冒烟：从仓库根目录设置 `AISMOKING_CASE=memory_search`，运行 `yak common/ai/aismoking/run.yak`。包含独立库、两种 action 协议及 smart_qa 转发；只供本地开发，不加入 CI。
