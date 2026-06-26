`subdomain` 库提供子域名枚举能力，结合字典爆破与多源搜索发现目标域名的子域，支持递归枚举与泛解析处理，是资产测绘的重要一环。

典型使用场景：

- 枚举：`subdomain.Scan(target, opts...)` 对目标域枚举子域并流式返回结果。
- 控制：`subdomain.mainDict` / `subdomain.recursiveDict` 指定字典，`subdomain.recursive` / `subdomain.maxDepth` 控制递归，`subdomain.wildcardToStop` 处理泛解析，`subdomain.dnsServer` 指定解析服务器，`subdomain.targetConcurrent` / `subdomain.workerConcurrent` 控制并发。

与相邻库的关系：`subdomain` 处于资产发现前端，与 `dns`（解析）、`spacengine`（测绘）配合发现资产，结果可交给 `servicescan`/`poc` 做后续扫描。
