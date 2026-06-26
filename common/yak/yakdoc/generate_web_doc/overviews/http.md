`http` 库提供高层、链式风格的 HTTP 客户端，用 `http.Get` / `http.Post` 等便捷方法加选项发起请求，并内置 favicon 哈希、请求指纹等安全测绘小工具。相比 `poc` 更接近"通用 HTTP 客户端"的使用习惯。

典型使用场景：

- 发起请求：`http.Get` / `http.Post` / `http.Request` / `http.Do` / `http.NewRequest`，配合 `http.header` / `http.body` / `http.json` / `http.params` / `http.cookie` / `http.proxy` / `http.timeout` / `http.ua` 等选项。
- 响应处理：`http.GetAllBody` 取响应体，`http.dump` / `http.show` 打印请求/响应，`http.dumphead` / `http.showhead` 打印头部。
- 测绘指纹：`http.RequestFaviconHash` / `http.ExtractFaviconURL` 计算 favicon 哈希，`http.RequestToMD5` / `http.RequestToMMH3Hash128` / `http.RequestToSha256` 计算响应指纹（用于网络空间测绘比对）。

与相邻库的关系：`http` 偏"通用客户端 + 测绘"，`poc` 偏"原始报文级精确控制"，`fuzz`/`fuzzx` 偏"批量变异"；三者覆盖从易用到精细的不同 HTTP 需求。
