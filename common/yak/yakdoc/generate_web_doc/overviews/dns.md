`dns` 库提供 DNS 查询能力，支持解析 A/IP、NS、TXT 记录以及域传送（AXFR）检测，用于资产测绘、子域发现与 DNS 配置安全检查。

典型使用场景：

- 解析记录：`dns.QueryIP` / `dns.QueryIPAll` 查 A 记录，`dns.QueryNS` 查域名服务器，`dns.QueryTXT` 查 TXT 记录。
- 域传送检测：`dns.QueryAxfr`（及别名 `dns.QuertAxfr`）尝试 AXFR 域传送，发现配置不当的 DNS 服务器。
- 选项：`dns.dnsServers` 指定上游 DNS，`dns.timeout` 控制超时。

与相邻库的关系：`dns` 是资产发现的基础设施，常与 `subdomain`（子域枚举）、`dnslog`（带外检测）配合，把域名解析为可扫描的 IP。
