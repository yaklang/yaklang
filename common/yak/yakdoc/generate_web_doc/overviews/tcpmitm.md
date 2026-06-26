`tcpmitm` 库提供 TCP 层的中间人（MITM）能力，劫持任意 TCP 连接与帧，对原始字节流进行查看与改写，适合非 HTTP 协议的流量分析与篡改。

典型使用场景：

- 启动劫持：`tcpmitm.Start(ch, opts...)` 启动 TCP MITM，`tcpmitm.hijackTCPConn` 在连接级回调处理，`tcpmitm.hijackTCPFrame` 在帧级回调改写字节，`tcpmitm.dialer` 自定义上游连接方式，`tcpmitm.context` 控制生命周期。

与相邻库的关系：`tcpmitm` 工作在 TCP 字节流层，`mitm` 工作在 HTTP/HTTPS 层；前者适合自定义/二进制协议，常与 `tcp`（连接）、`pcapx`（抓包）配合。
