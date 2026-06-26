`jsonstream` 库提供流式 JSON 解析能力，边读边触发回调，适合处理超大 JSON、不完整/边到达的 JSON（如 AI 流式输出），而无需一次性载入全部内容。

典型使用场景：

- 流式提取：`jsonstream.Extract(input, opts...)` 解析输入，`jsonstream.ExtractFromReader` 从 reader 流式解析。
- 回调订阅：`jsonstream.onObject` / `jsonstream.onArray` / `jsonstream.onKeyValue` 处理对象/数组/键值，`jsonstream.onField` / `jsonstream.onFieldGlob` / `jsonstream.onFieldRegexp` 按字段名/通配/正则定向处理，`jsonstream.onError` / `jsonstream.onFinished` 处理错误与完成。

与相邻库的关系：`jsonstream` 与 `json`（完整解析）互补，专攻流式与大体量场景，常用于解析 `ai` 的流式响应或大型导出文件。
