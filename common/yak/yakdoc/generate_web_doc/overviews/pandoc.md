`pandoc` 库提供文档格式转换能力（基于 pandoc），目前主要支持把 Markdown 转换为 Word/docx，常用于把扫描报告、分析结果导出为可分享的文档。

典型使用场景：

- Markdown 转 Word：`pandoc.SimpleConvertMarkdownFileToDocx(md)` 把 Markdown 转为 docx，`pandoc.SimpleCoverMD2Word(ctx, in, out)` 指定输入输出文件转换，带上下文版本支持取消。

与相邻库的关系：`pandoc` 是报告输出工具，常与 `report`（生成 Markdown/HTML 报告）、`file`（落盘）配合，把结果交付为 Word 文档。
