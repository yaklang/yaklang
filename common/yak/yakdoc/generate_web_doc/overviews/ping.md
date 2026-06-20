`ping` 库提供主机存活探测能力，支持 ICMP Ping 与 TCP Ping，常用于扫描前的存活筛选，把"活着的主机"先挑出来再做端口/服务扫描，提升整体效率。

典型使用场景：

- 单点与批量：`ping.Ping(target, opts...)` 探测单个目标，`ping.Scan(target, opts...)` 对网段批量探测并流式返回结果。
- 控制：`ping.tcpPingPorts` 用 TCP Ping 指定端口（绕过禁 ICMP 环境），`ping.concurrent` 控制并发，`ping.timeout` / `ping.proxy` / `ping.dnsServers` 控制超时与解析，`ping.scanCClass` 扫描整个 C 段，`ping.onResult` 处理每个结果。

与相邻库的关系：`ping` 处于扫描链路前端，存活结果常交给 `synscan`/`servicescan` 做后续端口与服务扫描。
