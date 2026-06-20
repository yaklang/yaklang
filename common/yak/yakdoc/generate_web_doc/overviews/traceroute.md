`traceroute` 库提供路由追踪能力，探测到达目标主机途经的每一跳网关，常用于网络拓扑探测、链路诊断与目标网络环境分析。部分协议探测可能需要相应权限。

典型使用场景：

- 路由追踪：`traceroute.Diagnostic(host, opts...)` 对目标做路由追踪并流式返回每一跳结果，配 `traceroute.protocol`（ICMP/UDP/TCP）、`traceroute.hops` / `traceroute.firstTTL` 控制跳数范围，`traceroute.timeout` / `traceroute.retry` 控制超时重试。

与相邻库的关系：`traceroute` 与 `ping`（存活探测）、`pcapx`（底层数据包）同属网络探测工具，用于刻画到达目标的网络路径。
