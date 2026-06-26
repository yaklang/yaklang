`bufio` 库是 Go 标准库 `bufio` 的 yak 封装，提供带缓冲的读写器与扫描器，用于高效地按行/按块处理 I/O 流（文件、网络连接、内存缓冲等）。

典型使用场景：

- 读取：`bufio.NewReader` / `bufio.NewReaderSize` 创建带缓冲读取器，`bufio.NewScanner` 创建按行/按 token 扫描器逐段读取大流。
- 写入：`bufio.NewWriter` / `bufio.NewWriterSize` 创建带缓冲写入器，`bufio.NewReadWriter` 组合读写。
- 缓冲与管道：`bufio.NewBuffer` 创建内存字节缓冲，`bufio.NewPipe` 创建内存管道（一端写、一端读）。

与相邻库的关系：`bufio` 是底层 I/O 工具，常与 `io`、`file`、`tcp`/`udp` 等配合，用于流式读取网络/文件数据而不一次性占满内存。
