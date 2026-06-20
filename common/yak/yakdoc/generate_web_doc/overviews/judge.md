`judge` 库用于比较两次 HTTP 响应的相似度，量化差异程度，常用于布尔盲注、差异判定、WAF 拦截识别等"基于响应变化"的检测逻辑。

典型使用场景：

- 相似度比较：`judge.CompareHTTPResponse` 比较两个响应对象，`judge.CompareRaw` 比较两段原始响应字节，返回相似度分值。
- 判别器：`judge.NewDiscriminator(origin)` 以基准响应创建判别器，用于持续判定后续响应是否"显著不同"。

与相邻库的关系：`judge` 是检测判定工具，常与 `fuzz`/`poc`（发包）配合，把"响应差异"转化为漏洞是否触发的判断依据。
