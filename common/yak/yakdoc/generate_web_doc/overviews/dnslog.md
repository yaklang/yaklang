`dnslog` 库提供 DNSLog 带外（OOB）检测能力：申请一个临时域名，诱导目标对其发起 DNS 解析，再回查解析记录来确认无回显漏洞（如 SSRF、命令注入、Log4j、反序列化）的存在。

典型使用场景：

- 申请与回查：`dnslog.NewCustomDNSLog` 创建自定义 DNSLog 客户端获取域名与 token，触发后用其查询解析记录；`dnslog.LookupFirst` 查询某域名的首条解析。
- 模式配置：`dnslog.mode` / `dnslog.local` / `dnslog.random` / `dnslog.script` 选择平台、本地模式、随机域名与自定义脚本，`dnslog.QueryCustomScript` 查询可用脚本。

与相邻库的关系：`dnslog` 与 `risk`（记录漏洞）、`yso`（生成带 DNSLog 的反序列化 Payload）、`poc`（发起触发请求）紧密配合，是无回显漏洞验证的关键带外通道。
