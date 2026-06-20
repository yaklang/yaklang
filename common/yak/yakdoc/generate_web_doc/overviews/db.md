`db` 库是 yaklang 的数据持久化与资产管理入口，封装了对 Yakit 内置数据库（Profile 全局配置库、Project 项目库）以及任意 SQLite/MySQL 数据库的读写能力。它把"扫描产出的资产、流量、漏洞、Payload、插件"等核心数据沉淀下来，并提供 `Yield*` 流式游标按需读取海量数据，避免一次性加载撑爆内存。

典型使用场景：

- 键值存储：`db.SetKey` / `db.GetKey` / `db.DelKey` 在 Profile 库里跨脚本、跨运行共享配置与中间结果；`db.SetKeyWithTTL` 写入带过期时间的缓存；`db.SetProjectKey` / `db.GetProjectKey` 则隔离在当前项目库中。
- 资产入库：`db.SaveHTTPFlowFromRaw` / `db.SaveHTTPFlowFromRawWithOption` 把原始 HTTP 请求/响应入库，配合 `db.saveHTTPFlowWithTags` 等选项打标签；`db.SavePayload` / `db.SavePayloadByFile` 管理字典。
- 数据检索：`db.QueryHTTPFlowsAll` / `db.QueryHTTPFlowsByKeyword` / `db.QueryPortsByTaskName` 等 `Query*` 家族按条件查询资产；`db.YieldPayload` / `db.YieldYakScriptAll` 以流式游标遍历大数据集。
- 临时插件与原始库：`db.CreateTemporaryYakScript` 创建临时插件供 `hook` 库加载（记得 `db.DeleteYakScriptByName` 清理）；`db.OpenSqliteDatabase` / `db.OpenTempSqliteDatabase` / `db.ScanResult` 直接操作自定义数据库。

与相邻库的关系：`db` 负责"把数据持久化与查询"，`yakit` 负责"把结果展示给人"，`risk` 负责"漏洞对象"，`hook` 通过 `db.CreateTemporaryYakScript` 装载临时插件。它们在扫描类脚本中常协同：发现结果 → `risk` 记录 → `db` 入库 → `yakit` 实时展示。
