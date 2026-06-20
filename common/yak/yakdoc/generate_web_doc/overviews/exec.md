`exec` 库用于在本机执行系统命令并获取输出，支持同步执行、上下文取消、流式监听输出与批量并发执行，是与操作系统交互、集成外部工具的入口。

典型使用场景：

- 直接执行：`exec.System` 执行命令并返回输出，`exec.SystemContext` 带上下文取消，`exec.SystemBatch` 批量并发执行。
- 构造命令：`exec.Command` / `exec.CommandContext` 创建 `*exec.Cmd` 以做更精细控制，`exec.CheckCrash` 判断进程是否崩溃。
- 流式监听：`exec.WatchStdout` / `exec.WatchStderr` / `exec.WatchOutput` 在超时内回调处理输出流；`exec.concurrent` / `exec.timeout` / `exec.callback` 控制批量行为。

与相邻库的关系：`exec` 是系统交互工具，常用于调用外部安全工具、采集本机信息（与 `os`、`hids` 配合）。注意命令内容须可信，避免注入风险。
