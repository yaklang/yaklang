`poc` 库是 yaklang 的底层 HTTP 报文工具核心，提供"原始报文级"的请求发送与对 HTTP 数据包任意部位的读取/构造/改写能力（近 190 个函数）。它不替你做任何隐式修改，适合编写 PoC、漏洞利用与对协议细节高度敏感的测试。

典型使用场景：

- 发送请求：`poc.HTTP` / `poc.HTTPEx` 直接发原始报文，`poc.Get` / `poc.Post` / `poc.Do` 按方法发送，配合 `poc.timeout` / `poc.proxy` / `poc.https` / `poc.host` 等选项（这些选项均为 `PocConfigOption`）。
- 读取报文：`poc.GetHTTPPacketBody` / `poc.GetHTTPPacketHeader` / `poc.GetAllHTTPPacketQueryParams` / `poc.GetStatusCodeFromResponse` 等 `Get*` 家族解析请求/响应。
- 构造改写：`poc.ReplaceHTTPPacketBody` / `poc.ReplaceHTTPPacketHeader` / `poc.AppendHTTPPacketQueryParam` / `poc.DeleteHTTPPacketCookie` 等 `Replace*`/`Append*`/`Delete*` 家族精确改包。
- 转换修复：`poc.FixHTTPRequest` / `poc.FixHTTPPacketCRLF` 修复报文，`poc.CurlToHTTPRequest` / `poc.HTTPRequestToCurl` 与 curl 互转，`poc.GetUrlFromHTTPRequest` 提取 URL。

与相邻库的关系：`poc` 是 `fuzz`/`fuzzx`（批量变异）、`nuclei`/`httptpl`（模板检测）的底层报文基石；相比 `http`（通用易用客户端）更贴近字节、可控性更强。务必为网络请求设置 `poc.timeout`。
