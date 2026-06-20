`nuclei` 库是 Nuclei 兼容的模板化漏洞扫描引擎，加载 YAML PoC 模板对目标批量检测，支持按 tag/severity 筛选、反连（interactsh）、并发与代理控制，是大规模已知漏洞扫描的主力。

典型使用场景：

- 扫描：`nuclei.Scan(target, opts...)` 对目标执行模板扫描，返回 `*PocVul` channel；`nuclei.ScanAuto` 自动处理批量目标。
- 模板与库：`nuclei.AllPoC` 列出模板，`nuclei.UpdateDatabase` / `nuclei.PullDatabase` / `nuclei.UpdatePoC` 维护模板库，`nuclei.templates` / `nuclei.tags` / `nuclei.severity` / `nuclei.excludeTags` 选择模板。
- 控制：`nuclei.targetConcurrent` / `nuclei.templatesThreads` / `nuclei.rateLimit` 控速，`nuclei.proxy` / `nuclei.timeout` / `nuclei.https` 控制传输，`nuclei.enableReverseConnection` 启用反连验证；`nuclei.PocVulToRisk` 把结果转为风险对象。

与相邻库的关系：`nuclei` 与 `httptpl`（模板匹配引擎）、`nasl`（NASL 引擎）同属模板化扫描；发现结果常经 `risk` 记录、`report` 汇总、`bot` 告警。
