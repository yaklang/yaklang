`fuzzx` 库是 `fuzz` 的新一代实现，围绕 `FuzzRequest` 对 HTTP 请求做变异与高性能批量发包，提供更清晰的请求构造与连接池/重定向控制，是 Web 模糊测试的推荐入口之一。

典型使用场景：

- 构造请求：`fuzzx.NewRequest` / `fuzzx.MustNewRequest` 从原始报文构建可变异请求。
- 传输控制：`fuzzx.https` / `fuzzx.host` / `fuzzx.port` / `fuzzx.proxy` 配置目标与代理，`fuzzx.concurrentLimit` / `fuzzx.delay` / `fuzzx.timeout` / `fuzzx.connPool` 控制并发与连接池，`fuzzx.noRedirect` / `fuzzx.redirectTimes` / `fuzzx.noFixContentLength` 控制重定向与报文修正。

与相邻库的关系：`fuzzx` 与 `fuzz` 同源、定位一致（HTTP 变异测试），常配合 `httpool`（请求池）与 `poc`（精确请求）使用。
