`zip` 库提供功能完整的 ZIP 压缩/解压能力，支持加密、按模式提取、以及在不解压的情况下对压缩包内容做正则/子串检索（grep），常用于归档处理、固件/样本分析与压缩包内的敏感信息搜索。

典型使用场景：

- 压缩：`zip.Compress` / `zip.CompressByNameWithOptions` 打包文件，`zip.CompressRaw` 压缩内存数据，`zip.CompressWithPassword` 加密压缩。
- 解压与提取：`zip.Decompress` / `zip.DecompressWithPassword` 解压，`zip.ExtractFile` / `zip.ExtractByPattern` 按文件名/模式提取，`zip.Recursive` 遍历包内条目。
- 内容检索：`zip.GrepRegexp` / `zip.GrepSubString` / `zip.GrepPathRegexp` 在压缩包内按正则/子串/路径检索，`zip.NewGrepSearcher` 创建可复用检索器，`zip.RRFRankResults` 对结果排序。

与相邻库的关系：`zip` 与 `gzip`（单流压缩）、`filesys`（文件系统遍历）、`diff`（`DiffZIPFile`）配合，是归档处理与"压缩包里找东西"的主力。
