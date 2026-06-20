`omnisearch` 库提供统一的搜索接口，对接多种搜索后端（搜索引擎/网络空间测绘/自定义源），用一套 API 完成跨源检索，常用于情报收集与资产发现。

典型使用场景：

- 统一检索：`omnisearch.Search(query, opts...)` 执行搜索并返回结果集。
- 后端与参数：`omnisearch.backendType` / `omnisearch.type` 选择后端，`omnisearch.apikey` / `omnisearch.baseurl` / `omnisearch.proxy` 配置访问，`omnisearch.page` / `omnisearch.pagesize` / `omnisearch.timeout` 控制分页与超时，`omnisearch.customSearcher` 注册自定义搜索源。

与相邻库的关系：`omnisearch` 是情报检索聚合层，常作为 AI 工具（`aiagent`/`aim` 的搜索能力）或资产发现流程的输入端，与 `spacengine`（网络空间测绘）思路相通。
