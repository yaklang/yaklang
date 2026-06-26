`mimetype` 库用于检测数据的 MIME 类型，基于内容魔数而非文件后缀判断真实类型，常用于文件上传安全检查、内容分类与取证。

典型使用场景：

- 检测类型：`mimetype.Detect(i)` 检测字节/字符串数据的 MIME 类型，`mimetype.DetectFile(path)` 检测文件的 MIME 类型。

与相邻库的关系：`mimetype` 与 `file`（`file.DetectMIMEType*`）、`fileparser`（文件解析）配合，用于"这份数据/文件到底是什么类型"的判断。
