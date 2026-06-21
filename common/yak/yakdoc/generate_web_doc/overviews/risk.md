`risk` 库是漏洞/风险对象的创建与管理中心，负责把发现的漏洞结构化记录（标题、等级、目标、请求/响应、CVE、解决方案等）并入库，同时提供反连（DNSLog/HTTPLog/RMI/ICMP/随机端口）平台用于无回显漏洞的带外验证。

典型使用场景：

- 记录漏洞：`risk.NewRisk(target, opts...)` / `risk.CreateRisk` 创建风险，配 `risk.title` / `risk.severity` / `risk.cve` / `risk.request` / `risk.response` / `risk.payload` / `risk.solution` 等丰富信息，`risk.Save` 保存。
- 反连验证：`risk.NewDNSLogDomain` / `risk.NewHTTPLog` 申请带外域名与 token，`risk.CheckDNSLogByToken` / `risk.CheckHTTPLogByToken` 回查是否被触发；`risk.NewLocalReverseHTTPUrl` / `risk.NewPublicReverseRMIUrl` 等生成反连地址。
- 查询：`risk.QueryRisksByKeyword` / `risk.YieldRiskByTarget` / `risk.YieldRiskByRuntimeId` 检索；`risk.GetSSARiskByID` / `risk.GetSSARiskWithDataFlow` 处理代码审计风险。

与相邻库的关系：`risk` 是漏洞中枢，上游接 `poc`/`fuzz`/`nuclei`（发现漏洞）、`dnslog`（带外），下游接 `db`（持久化）、`report`（报告）、`yakit`（展示）。

快速上手（结构化记录一条漏洞并入库）：

```yak
// risk.CreateRisk 创建并保存一条风险记录, 用选项补充标题/等级/类型/payload 等
r = risk.CreateRisk("127.0.0.1",
    risk.title("demo sql injection"),    // 漏洞标题
    risk.severity("high"),               // 等级: info/low/middle/high/critical
    risk.type("sqli"),                   // 漏洞类型
    risk.payload("' or 1=1 -- "),        // 触发用的 payload
)
println("risk created:", r.Title, r.Severity) // 预期输出: risk created: demo sql injection high
assert r.Title == "demo sql injection", "title should be set"
assert r.Severity == "high", "severity should be set"
// 无回显漏洞可用反连验证: domain, token = risk.NewDNSLogDomain()~ ; risk.CheckDNSLogByToken(token)
```
