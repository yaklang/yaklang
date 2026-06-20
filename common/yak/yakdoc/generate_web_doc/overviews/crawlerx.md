`crawlerx` 库是基于真实浏览器（CDP）的动态爬虫，能渲染 JS、自动填表、模拟登录、点击交互，适合 SPA/Vue 等强 JS 站点的深度爬取与攻击面发现。相比 HTTP 层的 `crawler`，它更贴近真实用户行为但更重。

典型使用场景：

- 启动爬取：`crawlerx.StartCrawler(url, opts...)` 返回 `ReqInfo` 的 channel；`crawlerx.PageScreenShot` 对页面截图；`crawlerx.OutputResult` 导出结果。
- 登录与会话：`crawlerx.loginUsername` / `crawlerx.loginPassword`、`crawlerx.cookies` / `crawlerx.rawCookie`、`crawlerx.localStorage` / `crawlerx.sessionStorage` 维持登录态，`crawlerx.formFill` / `crawlerx.fileInput` 自动填表单。
- 范围与隐蔽：`crawlerx.whitelist` / `crawlerx.blacklist`、`crawlerx.maxDepth` / `crawlerx.maxUrl`、`crawlerx.scanRangeLevel` / `crawlerx.scanRepeatLevel` 控制范围，`crawlerx.stealth` 反检测，`crawlerx.evalJs` 注入脚本。

与相邻库的关系：`crawlerx` 依赖 `browser` 提供的浏览器实例，与 HTTP 层 `crawler` 互为补充；产出的请求可入库（`db`）或交给 `poc`/`fuzz` 做后续测试。
