`xhtml` 库提供 HTML 内容的解析、遍历、查找与比较能力，常用于网页内容提取、DOM 差异分析与生成元素的 XPath 定位。

典型使用场景：

- 查找与遍历：`xhtml.Find(htmlRaw, matchStr)` 在 HTML 中查找匹配节点，`xhtml.Walker(h, handler)` 遍历 DOM 节点，`xhtml.MatchBetween` 抽取两个标记之间的内容。
- 定位与比较：`xhtml.GenerateXPath(node)` 为节点生成 XPath，`xhtml.CompareHtml` 比较两份 HTML 的差异。

与相邻库的关系：`xhtml` 与 `xpath`（XPath 查询）、`crawler`/`crawlerx`（爬取）配合，用于从网页中精确提取与比对内容。
