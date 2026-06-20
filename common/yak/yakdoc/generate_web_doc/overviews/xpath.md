`xpath` 库提供基于 XPath 的 HTML/XML 文档查询能力，加载文档后用 XPath 表达式精确定位节点并提取内容，常用于结构化网页数据提取。

典型使用场景：

- 加载文档：`xpath.LoadHTMLDocument(htmlText)` 解析 HTML 为节点树。
- 查询节点：`xpath.Find` / `xpath.FindOne` / `xpath.QueryAll` / `xpath.Query` 用 XPath 表达式定位节点，`xpath.InnerText` 取文本，`xpath.SelectAttr` / `xpath.ExistedAttr` 取/判断属性，`xpath.OutputHTML` 输出节点 HTML。

与相邻库的关系：`xpath` 与 `xhtml`（HTML 遍历/比较）、`crawler`/`crawlerx`（爬取）配合，是网页结构化提取的主力查询工具。
