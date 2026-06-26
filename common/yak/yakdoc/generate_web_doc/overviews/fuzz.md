`fuzz` 库是 yaklang 的 Web 模糊测试核心，围绕"FuzzHTTPRequest"对 HTTP 请求的各个部位（路径、参数、Header、Body、Cookie 等）做变异与批量发包，是漏洞探测与参数爆破的主力工具。它同时提供字符串模糊（fuzztag）与协议数据变异能力。

典型使用场景：

- 构造请求：`fuzz.HTTPRequest` / `fuzz.MustHTTPRequest` 从原始报文构建可变异请求，`fuzz.UrlToHTTPRequest` / `fuzz.UrlsToHTTPRequests` 从 URL 构建。
- 字符串变异：`fuzz.Strings` / `fuzz.StringsWithParam` / `fuzz.StringsFunc` 用 fuzztag 语法批量生成 Payload，`fuzz.FuzzCalcExpr` 生成数学表达式探测。
- 协议数据：`fuzz.ProtobufBytes` / `fuzz.ProtobufJSON` / `fuzz.ProtobufHex` / `fuzz.ProtobufYAML` 解析与变异 Protobuf。
- 发包池：`fuzz.WithConcurrentLimit` / `fuzz.WithDelay` / `fuzz.WithTimeOut` 控制并发与节流，`fuzz.https` / `fuzz.proxy` / `fuzz.context` 控制传输。

与相邻库的关系：`fuzz` 与 `poc`（单发/精确请求）互补——`poc` 偏"构造与发送一个确定请求"，`fuzz` 偏"对请求批量变异探测"；爬取（`crawler`）得到的请求常交给 `fuzz` 做深入测试。`fuzzx` 是其更新的变体。

快速上手（fuzztag 字符串变异，本地生成 Payload，不出网即可验证）：

```yak
// fuzztag 语法: {{int(1-3)}} 展开为 1、2、3, 与前缀拼接生成一组 payload
payloads = fuzz.Strings("admin{{int(1-3)}}")
println(payloads)                       // 预期输出: [admin1 admin2 admin3]
assert len(payloads) == 3, "int(1-3) should expand to 3 payloads"

// 常见用途: 把生成的 payload 套进请求做批量变异(需要可达目标, 这里仅示意)
// for p in payloads {
//     rsp, req = fuzz.HTTPRequest(`GET /?id=__P__ HTTP/1.1\r\nHost: example.com\r\n\r\n`)~ ...
// }
```
