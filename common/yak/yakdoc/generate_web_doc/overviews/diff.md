`diff` 库用于比较两份内容的差异，支持文本、目录、文件系统与 ZIP 包级别的对比，常用于版本变更分析、补丁比对与固件/代码审计。

典型使用场景：

- 文本差异：`diff.Diff(raw1, raw2, handler...)` 比较两段内容。
- 目录与归档：`diff.DiffDir` 比较两个目录，`diff.DiffZIPFile` 比较两个 ZIP 包，`diff.DiffFromFileSystem` 比较两个文件系统抽象。

与相邻库的关系：`diff` 是分析工具，常与 `filesys`（文件系统遍历）、`zip`（归档）、`sca`/`ssa`（代码与成分分析）配合，用于"前后版本变了什么"的比对场景。
