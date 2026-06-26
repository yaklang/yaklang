`httptpl` 库提供基于 YAML 模板（Nuclei 风格）的 HTTP 匹配与提取能力，对给定请求/响应套用 matcher/extractor 规则，判断是否命中并抽取数据，是模板化漏洞检测的底层匹配引擎。

典型使用场景：

- 匹配提取：`httptpl.MatchOrExtractHTTPFlow(req, rsp, yamlString, opts...)` 用 YAML 模板对一次 HTTP 交互做匹配与字段提取。
- 选项：`httptpl.https` 指定 HTTPS，`httptpl.vars` 注入模板变量。

与相邻库的关系：`httptpl` 与 `nuclei`（完整 PoC 模板执行）、`fuzz`/`poc`（请求构造发送）配合，把"matcher/extractor 规则"应用到任意流量上。
