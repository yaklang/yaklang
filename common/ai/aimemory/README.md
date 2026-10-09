# aimemory

独立的记忆检索和管理库；公开业务入口为 `Search` 和 `Amend`。

```yak
items, err = aimemory.Search(
    "报告格式偏好",
    aimemory.memoryNamespace("default"),
    aimemory.memoryTokenLimit(1500),
    aimemory.memorySearchMode("hybrid"),
    aimemory.memoryLimit(5),
)
if err != nil { die(err) }
for item in items { println(item.Dump()) }
```

`namespace` 就是 memory 集 ID，对应 memory 表中的 `session_id` 和 RAG 集合 `ai-memory-<id>`。数据来自宿主绑定的项目数据库；只用一个 ID，不引入 workspace。AI 工具通过 `aimemory.CurrentNamespace()` 取得运行时实际使用的 memory 集，独立 Yak 脚本默认 `default`。持久化会话 ID 与 memory 集 ID 没有通用的等价关系，已有会话表也没有保存这种映射。

`Search` 返回封装现有 `AIMemoryEntity` 数据行的 `Item`，字段直接可读，`Dump()` 输出包含 ID、空间、时间、标签和原始正文的文本，使用现有 RAG BM25/语义向量查询和 memory 表查询接口。支持 `bm25`、`vector`、`hybrid`；问句索引之外补充正文/标签关键词匹配，合并排名、去重并过滤过期和已删除记录。没有匹配时返回空数组。搜索不初始化 triage、不创建集合、不触发索引重建；hybrid 向量不可用时保留关键词结果并记录英文诊断，vector 返回错误。上下文取消在检索前后检查；向量请求沿用底层 embedding 服务的超时机制。

`Amend(operator, memoryID, content, tags, opts...)` 管理当前空间的精确记忆条目，返回可 `Dump()` 的 `Item`。`add` 创建新 ID，`delete` 返回被删除条目作为回执，`change` 删除旧条目及索引后创建新 ID；替换写入失败时恢复旧条目。

默认最多 5 条，范围 1–20；默认正文总预算 1500 tokens，范围 64–8192。超长正文可能截断。检索结果的字段选择、标签和时间展示统一在 `search_memory.yak` 中维护，普通工具与 smart_qa 的 memory action 都执行这个脚本，结果通过普通工具 Timeline 保存。

自动 injection 独立复用现有 `SearchMemoryWithoutAI`，只在意图识别落地时执行一次；显式工具搜索不会更新其快照。自动记忆抽取仅来自 Timeline 压缩：执行中沿用阈值压缩，完整用户任务正常结束且有新增业务内容时收尾。候选通过会话通知可靠保存，支持重复通知去重、部分失败重试及恢复补处理；审核等待、阶段切换和 gRPC 断开不作为正常收尾。手动创建记忆及查询、编辑、删除接口保留。

本地冒烟：从仓库根目录设置 `AISMOKING_CASE=memory_search`，运行 `yak common/ai/aismoking/run.yak`。包含独立库、两种 action 协议及 smart_qa 转发；只供本地开发，不加入 CI。
