`udp` 库提供 UDP 客户端与服务端能力，用于无连接的数据报收发，常用于 UDP 协议交互、自定义协议测试与服务探测。

典型使用场景：

- 客户端：`udp.Connect(target, port, opts...)` 创建 UDP 连接收发数据（配 `udp.clientTimeout` / `udp.clientLocalAddr`）。
- 服务端：`udp.Serve(host, port, opts...)` 起 UDP 服务（配 `udp.serverCallback` 处理数据报、`udp.serverTimeout` / `udp.serverContext`）。
- 测试桩：`udp.MockUDPProtocol` 快速起 mock UDP 服务用于联调。

与相邻库的关系：`udp` 与 `tcp`（面向连接）互补，是无连接协议的实现基础，常用于 DNS、SNMP 等 UDP 服务的交互测试。
