`gzip` 库提供 gzip 压缩与解压能力，用于处理 HTTP 响应、日志、归档等 gzip 编码的数据。

典型使用场景：

- 压缩/解压：`gzip.Compress` 压缩数据，`gzip.Decompress` 解压。
- 识别：`gzip.IsGzip` 判断字节流是否为 gzip 格式。

与相邻库的关系：`gzip` 是数据处理小工具，常与 `http`/`poc`（处理压缩响应）、`zip`（归档）、`codec`（编解码）配合。
