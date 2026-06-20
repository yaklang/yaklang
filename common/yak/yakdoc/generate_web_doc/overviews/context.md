`context` 库是 Go 标准库 `context` 的 yak 封装，用于在并发与网络操作中传递取消信号、超时与截止时间，是控制长耗时操作（扫描、请求、AI 调用）生命周期的基础设施。

典型使用场景：

- 创建上下文：`context.Background` / `context.New` 创建根上下文。
- 超时控制：`context.Seconds` / `context.WithTimeoutSeconds` 快速创建带超时的上下文，`context.WithTimeout` / `context.WithDeadline` 精细控制。
- 取消与传值：`context.WithCancel` 返回可手动取消的上下文与取消函数，`context.WithValue` 在上下文中携带键值。

与相邻库的关系：`context` 几乎被所有支持 `context(ctx)` 选项的库（`poc`、`crawler`、`synscan`、`aiagent` 等）使用，把它传入即可统一控制超时与中止。
