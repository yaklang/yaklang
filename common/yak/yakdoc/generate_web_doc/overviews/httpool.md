`httpool` 库提供 HTTP 请求池，对一批请求（可结合 fuzztag 变异）做高并发批量发送并流式返回结果，适合大规模探测、批量验证与字典爆破类场景。

典型使用场景：

- 批量发包：`httpool.Pool(i, opts...)` 传入请求（或可变异的模板），返回 `*HttpResult` 的 channel。
- 控制：`httpool.size` / `httpool.perRequestTimeout` 控制并发与超时，`httpool.fuzz` / `httpool.fuzzParams` 启用变异，`httpool.https` / `httpool.host` / `httpool.proxy` / `httpool.rawMode` 控制传输，`httpool.noRedirect` / `httpool.redirectTimes` 控制重定向。

与相邻库的关系：`httpool` 是发包引擎，常作为 `fuzz`/`fuzzx` 的底层批量执行层，也可直接用于大批量 URL 探测。
