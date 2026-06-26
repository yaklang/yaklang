`spacengine` 库对接主流网络空间测绘引擎（FOFA、Hunter、Quake、Shodan、ZoomEye、Zone 等），用统一接口按语法查询全网资产，常作为外部资产发现与情报收集的入口。使用前需要相应平台的 API Key/凭据。

典型使用场景：

- 统一查询：`spacengine.Query(filter, opts...)` 用 `spacengine.engine` 指定引擎与认证后统一查询；也可直接用 `spacengine.FofaQuery` / `spacengine.HunterQuery` / `spacengine.QuakeQuery` / `spacengine.ShodanQuery` / `spacengine.ZoomeyeQuery` / `spacengine.ZoneQuery`。
- 控制：`spacengine.maxPage` / `spacengine.maxRecord` / `spacengine.pageSize` 控制结果量，`spacengine.retryTimes` / `spacengine.randomDelay` 控制请求节奏。

与相邻库的关系：`spacengine` 查到的资产可交给 `servicescan`（`ScanFromSpaceEngine`）做指纹核验、`poc`/`fuzz` 做漏洞测试；与 `omnisearch`（聚合搜索）思路相通。
