`mitm` 库是中间人代理（MITM）能力封装，启动一个 HTTP/HTTPS 代理，对经过的请求/响应进行劫持、查看、改写或 mock，是流量分析、被动扫描与交互式测试的核心。支持国密 TLS、透明代理、WebSocket 与 JA3 随机化。

典型使用场景：

- 启动代理：`mitm.Start(port, opts...)` 启动 MITM，`mitm.Bridge` 串联下游代理。
- 劫持改写：`mitm.hijackHTTPRequest` / `mitm.hijackHTTPResponse` 在回调里修改或丢弃报文，`mitm.mockHTTPRequest` 直接返回 mock 响应，`mitm.callback` 旁路观察全部流量。
- 证书与传输：`mitm.AddMITMRootCertIntoSystem` / `mitm.VerifyMITMRootCertInstalled` 管理根证书，`mitm.rootCA` / `mitm.gmtls` / `mitm.sni` / `mitm.randomJA3` 控制 TLS 行为，`mitm.wscallback` 处理 WebSocket。

与相邻库的关系：`mitm` 是流量入口，常与 `hook`（驱动插件链处理流量）、`fuzz`/`poc`（对劫持到的请求做测试）、`db`（流量入库）协同，构成被动扫描与交互测试流水线。
