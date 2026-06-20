`crawler` 库是基于 HTTP 的站点爬虫，从一个起始 URL 出发自动发现并抓取站内链接，产出一系列请求（`*Req`），用于资产测绘、目录发现与攻击面收集。它工作在 HTTP 层，速度快、资源省。

典型使用场景：

- 启动爬取：`crawler.Start(url, opts...)` 返回 `*Req` 的 channel，可边爬边消费；`crawler.RequestsFromFlow` 从已有流量里提取请求。
- 范围与认证：`crawler.domainInclude` / `crawler.domainExclude` / `crawler.disallowSuffix` 控制爬取范围，`crawler.basicAuth` / `crawler.cookie` / `crawler.autoLogin` 处理认证，`crawler.header` 自定义请求头。
- 性能与超时：`crawler.concurrent` 控制并发，`crawler.connectTimeout` / `crawler.bodySize` / `crawler.context` 控制超时与资源；`crawler.urlExtractor` 自定义链接提取规则；`crawler.aiJSExtract` 借助 AI 从 JS 中提取链接。

与相邻库的关系：`crawler` 走 HTTP 层，`crawlerx` 走真实浏览器（适合强 JS 站点）；爬到的请求常交给 `poc`/`fuzz` 做进一步测试，或经 `hook` 插件链路处理。
