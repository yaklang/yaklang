`file` 库是 yaklang 的文件与路径操作工具箱，覆盖读写、创建删除、遍历、路径处理、类型/MIME 识别、哈希计算等近 50 个函数，是本地文件处理的核心依赖。

典型使用场景：

- 读写：`file.ReadFile` / `file.ReadLines` / `file.ReadAll` 读取，`file.Save` / `file.SaveJson` 写入，`file.Open` / `file.OpenFile` / `file.Create` / `file.TempFile` 打开句柄，`file.TailF` 跟踪追加。
- 文件操作：`file.Cp` / `file.Mv` / `file.Rm`（`file.Remove`）/ `file.Mkdir` / `file.MkdirAll` / `file.Rename`。
- 路径处理：`file.Join` / `file.Abs` / `file.Clean` / `file.Dir` / `file.GetBase` / `file.GetExt` / `file.Split`。
- 信息与遍历：`file.IsExisted` / `file.IsDir` / `file.IsFile` / `file.Stat`、`file.Ls` / `file.Dir` / `file.Walk` 遍历，`file.Md5` / `file.Sha256` 计算哈希。
- 类型识别：`file.DetectFileType` / `file.DetectMIMETypeFromFile` / `file.DetectMIMETypeFromRaw`，以及 `file.MatchMalicious` 恶意文件匹配。

与相邻库的关系：`file` 是基础 I/O 库，与 `filesys`（文件系统抽象遍历）、`os`（系统）、`io`/`bufio`（流）、`mimetype`（类型）协同，几乎所有需要落地/读取数据的脚本都会用到。
