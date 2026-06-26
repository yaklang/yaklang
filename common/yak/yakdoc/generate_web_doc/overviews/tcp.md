`tcp` 库提供 TCP 客户端与服务端能力，支持连接、收发数据、TLS、端口转发以及搭建测试服务，是裸 TCP 协议交互与自定义协议测试的基础。

典型使用场景：

- 客户端：`tcp.Connect(host, port, opts...)` 建立连接（配 `tcp.clientTimeout` / `tcp.clientProxy` / `tcp.clientTls` / `tcp.clientLocal`），之后收发字节。
- 服务端与转发：`tcp.Serve(host, port, opts...)` 起 TCP 服务（配 `tcp.serverCallback` 处理连接、`tcp.serverTls`），`tcp.Forward` 做端口转发。
- 测试桩：`tcp.MockServe` / `tcp.MockTCPProtocol` 快速起 mock 服务用于联调测试。

与相邻库的关系：`tcp` 是裸协议层工具，与 `udp`（无连接）、`tls`（加密）、`bufio`/`io`（流处理）配合，用于自定义协议的客户端/服务端实现。
