`browser` 库提供对无头/有头 Chrome 浏览器实例的管理能力，基于 CDP（Chrome DevTools Protocol）驱动真实浏览器，用于需要渲染 JS、模拟真实用户行为的爬取、截图与自动化场景。

典型使用场景：

- 实例管理：`browser.Open` / `browser.Get` 启动或获取一个浏览器实例，`browser.Close` / `browser.CloseAll` 关闭，`browser.List` 列出当前实例。
- 环境检查：`browser.HaveBrowserInstalled` 判断本机是否已安装可用浏览器。
- 启动选项：`browser.headless`（无头模式）、`browser.exePath`（指定可执行文件）、`browser.proxy`（代理）、`browser.wsAddress` / `browser.controlURL`（连接已有浏览器）、`browser.noSandBox` / `browser.leakless` / `browser.timeout` 等。

与相邻库的关系：相比 `crawler`/`crawlerx` 的 HTTP 层爬取，`browser` 走真实浏览器内核，适合强 JS 渲染的页面；常与 `crawlerx`（浏览器爬虫）配合完成动态站点的资产发现。
