`cve` 库提供本地 CVE（通用漏洞披露）数据库的下载、导入、查询与统计能力，便于把指纹识别结果关联到已知漏洞，支撑漏洞情报与版本比对。

典型使用场景：

- 数据管理：`cve.Download` 下载 CVE 数据，`cve.LoadCVE` / `cve.Import` / `cve.Export` 加载导入导出，`cve.AICompleteFields` 用 AI 补全字段。
- 查询：`cve.GetCVE` 按编号取单条，`cve.Query` / `cve.QueryEx` 按条件流式查询，配合 `cve.product` / `cve.vendor` / `cve.cpe` / `cve.score` / `cve.severity` / `cve.after` / `cve.before` 等选项过滤；`cve.parseToCpe` 解析 CPE 串。
- 统计：`cve.NewStatistics` 生成统计视图。

与相邻库的关系：`cve` 与 `cwe`（弱点分类）、`sca`（成分分析）、`servicescan`（指纹/版本识别）协同：识别出产品与版本后，用 `cve` 关联已知漏洞。
