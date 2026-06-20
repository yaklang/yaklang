`ja3` 库提供 JA3/JA3S TLS 指纹的解析与应用能力，用于按指定 TLS ClientHello 指纹发起请求（指纹伪装/反检测）或识别 TLS 客户端/服务端特征。

典型使用场景：

- 解析指纹：`ja3.ParseJA3` / `ja3.ParseJA3S` 解析 JA3/JA3S 串，`ja3.ParseJA3ToClientHelloSpec` 转成 ClientHello 规格。
- 应用指纹：`ja3.GetTransportByClientHelloSpec` 用指定 ClientHello 规格构造 HTTP Transport，从而以特定 TLS 指纹发起请求。

与相邻库的关系：`ja3` 与 `tls`（TLS 能力）、`http`/`poc`（发起请求）配合，用于绕过基于 TLS 指纹的检测或做 TLS 指纹分析。
