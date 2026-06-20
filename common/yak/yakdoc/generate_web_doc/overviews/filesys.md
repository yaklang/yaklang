`filesys` 库提供文件系统抽象与递归遍历能力，统一处理本地目录、ZIP、内存等不同来源的文件系统，常用于目录扫描、批量文件处理与跨文件系统拷贝。

典型使用场景：

- 递归遍历：`filesys.Recursive(path, opts...)` 递归遍历目录，配合 `filesys.onFileStat` / `filesys.onDirStat` / `filesys.onStat` 等回调逐项处理，`filesys.dir` 指定 glob，`filesys.onStatEx` 支持中途停止。
- 跨文件系统：`filesys.CopyToRefLocal` / `filesys.CopyToTemporary` 把任意文件系统拷贝到本地/临时目录，`filesys.Glance` 快速预览文件。

与相邻库的关系：`filesys` 是文件系统抽象层，比 `file` 更适合统一处理 ZIP/内存等虚拟文件系统，常与 `diff`（差异比对）、`zip`（归档）、`ssa`（代码分析）配合。
