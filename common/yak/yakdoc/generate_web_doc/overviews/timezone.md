`timezone` 库提供时区处理能力，按时区名获取位置对象与该时区的当前时间，常用于跨时区的时间展示与日志归一化。

典型使用场景：

- 获取时区：`timezone.Get(name)` 按 IANA 时区名（如 `Asia/Shanghai`）获取 `*time.Location`。
- 时区时间：`timezone.Now(name)` 获取指定时区的当前时间。

与相邻库的关系：`timezone` 与 `time`（时间处理）配合，用于把时间转换/展示到指定时区。
