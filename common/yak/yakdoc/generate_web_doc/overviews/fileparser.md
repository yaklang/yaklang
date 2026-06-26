`fileparser` 库用于从各类文件中解析、提取结构化内容（如文档中的嵌入对象、元数据、内嵌文件等），常用于文件取证与样本分析。

典型使用场景：

- 解析文件：`fileparser.ParseFile(filePath)` 解析给定文件，返回按类型分组的提取结果（`map[string][]File`）。

与相邻库的关系：`fileparser` 是文件内容提取工具，与 `pandoc`（文档转换）、`mimetype`（类型识别）、`file`（文件读写）配合，用于"从文件里挖出有用内容"的取证与分析场景。
