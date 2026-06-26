`rag` 库是检索增强生成（Retrieval-Augmented Generation）的完整实现，提供知识库构建、文本向量化（Embedding）、向量存储与语义检索能力，让 AI 能基于私有知识作答。它内置 HNSW 向量索引、实体关系建模与知识图谱（k-hop）检索。

典型使用场景：

- 构建知识库：`rag.BuildCollectionFromFile` / `rag.BuildCollectionFromRaw` / `rag.BuildCollectionFromReader` 从文件/内容构建集合，`rag.AddDocument` 增量加文档，`rag.BuildIndexKnowledgeFromFile` 建索引。
- 向量化：`rag.Embedding` / `rag.LocalEmbedding` / `rag.OnlineEmbedding` 把文本转向量。
- 检索：`rag.Query` 语义检索，`rag.QueryDocuments` 在指定知识库检索，`rag.DBQueryKnowledge` / `rag.DBQueryEntity` 直查知识/实体；配 `rag.queryLimit` / `rag.querySimilarityThreshold` / `rag.khopk` 等调参。
- 管理：`rag.ListCollection` / `rag.GetCollection` / `rag.DeleteCollection` / `rag.Export` / `rag.Import` 管理集合。

与相邻库的关系：`rag` 是知识层，与 `ai`/`liteforge`（模型与 Embedding 来源）、`aiagent`/`aim`（把知识库作为工具挂载）、`aireducer`（长文分块）协同，构成"带私有知识的 AI"能力。
