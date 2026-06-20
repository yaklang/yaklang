`io` 库是 Go 标准库 `io` 的 yak 封装，提供读写流的基础工具：拷贝、限长、多路合并、管道与稳定读取等，是处理网络/文件流的底层依赖。

典型使用场景：

- 拷贝与读取：`io.Copy` / `io.CopyN` 在 reader/writer 间拷贝，`io.ReadAll` 读尽，`io.ReadFile` 读文件，`io.WriteString` 写字符串。
- 流组合：`io.MultiReader` 合并多个 reader，`io.TeeReader` 边读边复制，`io.LimitReader` 限长，`io.NopCloser` 包装，`io.Pipe` 创建管道。
- 流式读取：`io.ReadStable` / `io.ReadEvery1s` 按稳定/周期读取（适合实时输出）。

与相邻库的关系：`io` 是底层流工具，与 `bufio`（缓冲）、`file`（文件）、`tcp`/`udp`（网络连接）配合使用。
