`csrf` 库用于根据一个原始 HTTP 请求自动生成 CSRF（跨站请求伪造）PoC 页面，便于在 Web 安全测试中验证目标接口是否缺乏 CSRF 防护。

典型使用场景：

- 生成 PoC：`csrf.Generate(raw, opts...)` 传入原始 HTTP 请求报文，生成可直接打开的 HTML 表单 PoC。
- 行为选项：`csrf.autoSubmit` 让页面加载后自动提交，`csrf.https` 指定使用 HTTPS，`csrf.multipartDefaultValue` 处理 multipart 表单的默认值。

与相邻库的关系：`csrf` 是 Web 漏洞验证小工具，输入常来自 `fuzz`/`poc` 构造或抓取的请求，产出的 PoC 可用于复现与报告（`report`）。
