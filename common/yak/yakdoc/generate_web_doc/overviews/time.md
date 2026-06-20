`time` 库是 Go 标准库 `time` 的 yak 封装，提供时间获取、休眠、定时器、解析与格式化能力，是脚本中处理时间、节流与定时任务的基础。

典型使用场景：

- 当前时间：`time.Now` 取当前时间，`time.Unix` 由时间戳构造，`time.GetCurrentDate` / `time.GetCurrentMonday` 取日期。
- 休眠与定时：`time.Sleep` 休眠，`time.After` / `time.AfterFunc` 延时触发，`time.NewTimer` / `time.NewTicker` 创建定时器/打点器。
- 解析与计算：`time.Parse` 解析时间字符串，`time.ParseDuration` 解析时长，`time.Since` / `time.Until` 计算时间差。

与相邻库的关系：`time` 是基础库，常与 `context`（超时控制）、`sync`（并发节奏）配合，用于扫描节流、定时执行等场景。
