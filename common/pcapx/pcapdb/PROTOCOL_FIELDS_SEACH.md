# 协议字段检索：存储、索引与研究路线

本文件随 pcapdb 第一批实现提供，用于指导 BIN Parser 协议字段的保存、直接查询和后续加速研究。文件名按需求保留为 PROTOCOL_FIELDS_SEACH.md。当前子库 schema version 为 4。

当前采用“完整 JSONB 原本 + 按需标量字段索引”：协议消息保留完整字段树，常用路径单独建立 B-tree 索引。动态字段、数组成员和全文检索采用下面的分层路线，后续扩展仍使用独立子库和 GORM。

## 1. 主库与子库的归属

| 位置 | 保存内容 | 模型文件 |
| --- | --- | --- |
| Yakit profile 主库 | `pcapfile_db_metadata`：库身份、路径、指纹、版本、状态和已提交进度摘要 | [pcap_metadata_schema.go](pcap_metadata_schema.go) |
| 每个流量子库 | `protocol_messages`：完整协议字段、会话快照、来源、解析状态和消息定位 | [pcap_database_schema.go](pcap_database_schema.go) |
| 每个流量子库 | `message_packets`：协议消息到物理包的关联；其他包、接口和检查点表 | [pcap_database_schema.go](pcap_database_schema.go) |
| 每个流量子库 | `sqlite_schema`：实际字段索引定义；SQLite 自己维护的查询统计 | SQLite 内部目录 |
| 每个流量子库 | `packets.data`、`protocol_messages.data`、`stream_chunks.data`：完整包、协议消息及分块重组字节 BLOB | [pcap_database_schema.go](pcap_database_schema.go) |
| 每个流量子库 | `capture_records`：文件头、包封装、PCAPNG 选项和未知块；关联包 ID，原样导出 | [pcap_database_schema.go](pcap_database_schema.go) |
| 每个流量子库 | `sessions`、`streams`、`session_packets`、`stream_chunk_packets`：双向会话、方向流和索引关联 | [pcap_database_schema.go](pcap_database_schema.go) |

profile 主库只注册登记模型。协议 JSON、字段索引，以及将来可能增加的字段拆分表，均由子库的独立 GORM 句柄管理。所有持久化业务模型组合 `gorm.Model`。

## 2. 保留哪些协议信息

`PCAPProtocolMessage` 保存以下信息：

- `Protocol`、`Transport`、消息/flow/事务 ID、方向、时间、来源和目标。
- `Rule`、`Entry`、`Profile`、`Status`、`Completeness`、错误和诊断。这些信息帮助解释字段由哪个解析规则产生。
- `FieldsJSON` 对应子库 `fields`：完整 BIN Parser 字段树，包括嵌套对象和数组。
- `SessionJSON` 对应 `session`：事件产生时的会话快照。
- `SourceBytesJSON` 对应 `source_bytes`：来源信息；`message_packets` 提供可索引的包关联。
- `Data/DataLength` 保存消息重组字节；`SessionID/StreamID` 关联网络会话与方向流。原始包通过 `Packet.ID` 读取 `packets.data` BLOB，不存文件偏移。

三个 JSON 列保存 SQLite JSONB BLOB。导入先由 Go 序列化字段树，再在同一个 GORM 批量事务中转换为 JSONB。字段、原始/重组 BLOB、包关联和检查点一起提交。原始 capture 文件只是来源信息，删除后仍可读取、查询及原样导出。

JSON 树保留 JSON 序列化后的内容和结构。Go 的 `[]byte` 按 `encoding/json` 规则成为 base64 字符串，无效 UTF-8 字符串按该规则替换为 U+FFFD；原始二进制仍可通过 `ReadProtocol/ReadPacket` 读取。解析错误单独保存，字段树允许为 JSON null。

详情解码使用 `json.Decoder.UseNumber`，数字保留为 `json.Number`，避免 64 位协议整数经 `float64` 舍入。搜索接受这些数字；整数查询参数限于 SQLite 的有符号 64 位范围，REAL 查询使用有限浮点值。超出此范围的精确整数搜索需要后续的 decimal/uint64 投影，完整 JSON 原本继续保留。

SQLite JSONB 可以降低重复解析文本的开销，但当前格式的大部分操作仍随 JSON 大小线性增长。跨消息快速检索依靠额外索引。[SQLite JSON 官方文档](https://www.sqlite.org/json1.html)

## 3. 现在就能使用的 Yak 操作

下面假设目标协议的 Fields 含有 `Method`。实际字段名和大小写以 `DiscoverProtocolFields` 或 `ProtocolDetails` 返回的树为准；不同协议规则不共享统一的字段布局。

```yak
db = pcapdb.GetOrCreatePCAPDatabase("file.pcap",
    pcapdb.withProtocols(true),
    pcapdb.withFieldIndex("$.Method"))~

rows = db.QueryProtocols(
    pcapdb.protocol("msgpack-rpc"),
    pcapdb.field("$.Method", "lab.status"),
    pcapdb.fieldExists("$.Method"),
    pcapdb.limit(100))~

if len(rows) > 0 {
    detail = db.ProtocolDetails(rows[0].ID)~
    println(detail.Fields)
    println(detail.Session)
    packetIDs = db.ProtocolPacketIDs(rows[0].ID)~
}
```

已有解析完成的库可以再次调用 GetOrCreate 并加 `withFieldIndex`。也可以直接使用 `pcapdb.EnsureProtocolFieldIndexes(datasetID, path...)`，不依赖原始 PCAP 文件，不重放协议解析。索引定义保存在子库 `sqlite_schema`；全部新增索引在一个事务内提交，重复调用幂等，取消或达到索引上限时回滚本次新增项。提交后的连接刷新若被取消，索引可能已生效，重新调用可安全确认。

先发现实际字段，再选择常用路径：

```yak
library = pcapdb.OpenPCAPDatabase(datasetID)~
defer library.Close()
fields = library.DiscoverProtocolFields(
    pcapdb.protocol("http"), pcapdb.limit(10), pcapdb.resultBytes(65536))~
println(json.dumps(fields, json.withIndent("")))
// 从 fields.Items 选择实际存在且 Searchable=true 的标量路径，避免猜字段名。
indexes = pcapdb.EnsureProtocolFieldIndexes(datasetID, selectedPath)~
println(library.ProtocolFieldIndexes()~)
```

`DiscoverProtocolFields` 返回路径、JSON 类型集合、出现次数、示例消息 ID、可搜索标记和已索引标记；不返回字段值，也不解码完整 JSON 树。返回路径保留 SQLite 的对象转义和固定数组下标，可直接用于 `field/fieldExists/withFieldIndex`。计数只代表当前采样页。`limit` 限制消息数，`next_cursor` 是最后完成采样的消息 ID，配合原来的筛选条件和 `after` 继续采样。一个采样页最多扫描 16 MiB JSONB、展开 4,096 个节点；单消息超过 8 MiB 或 4,096 节点时跳过，记录 `skipped` 和 `truncated`。输出字段集合受 JSON 预算限制，截断时明确标记，不能将采样结果当作全库完整 schema。可减小消息页或按协议、时间、flow 限定范围进一步研究。

### 3.1 有输出预算的结果接口

原有 Go/Yak 查询保持兼容。后续 AI 适配层优先使用以下有明确结果结构的接口：

| 查询 | 新接口 |
| --- | --- |
| 库登记摘要 | `pcapdb.ListPCAPDatabasesPage(...)` |
| 包、协议、会话、方向流摘要 | `QueryPacketsPage / QueryProtocolsPage / QuerySessionsPage / QueryStreamsPage` |
| 协议/块到包，以及包到会话 | `ProtocolPacketIDsPage / StreamChunkPacketIDsPage / PacketSessionsPage` |
| 包、协议消息、流块的二进制预览 | `ReadPacketPage / ReadProtocolPage / ReadStreamPage` |
| 完整协议字段详情 | `ProtocolDetailsPage` |

所有结果使用 `dataset_id, state, items, next_cursor, cursor_kind, has_more, truncated, error`。登记页的 dataset_id 为空，每条 item 自带身份。未能读取有效快照时 state 为 `unknown`。error 为 null 或 `{code, message, retryable}`；原生 Go 同时返回 error，Yak 使用通常的错误处理语法。失败不返回成功的部分结果，也不推进输入游标。

`resultBytes` 默认 64 KiB，范围 4 KiB..1 MiB，按 `encoding/json` 的完整紧凑 JSON 计算，包括转义和 base64 扩张。记录 ID 游标通过一次有界 lookahead 判断 has_more，不用 OFFSET 或整库 COUNT。输出预算耗尽时保留最后返回项的 ID，下一页不会丢记录。单条无法装入预算则返回 `result_too_large`；可降低 previewBytes 或提高 resultBytes。协议摘要有短文本预览，使用 `preview_truncated` 明确标识截断，不携带 fields 或任何 BLOB。

二进制预览默认 512 字节，`previewBytes` 范围 0..64 KiB。Data 在 JSON 中编码为 base64；记录 total_bytes、truncated 和原始 ID。流页以块 ID 翻页，截短的是每个块的预览，不能直接拼接预览作为完整流；关联 API 和完整导出仍能取得原数据。流预览保留 references_complete。完整字段详情超过预算返回错误，不截出不完整的 JSON 树。

大内容使用文件输出：`ExportPacket(id,path)`、`ExportProtocol(id,path)`、`ExportStream(id,path)`、`ExportProtocolFields(id,path)`，返回含路径、字节数、SHA256、dataset ID 和记录 ID 的 artifact。文件通过临时文件、Sync 和独占发布生成，不覆盖已有目标；取消不发布半成品。流导出持有共享实例锁并分块读取，导出的是已经保存的观察字节，不填补捕获缺口；完整性仍应结合 session 的 Complete/HasGaps 判断。

### 3.2 查询 context 与句柄生命周期

Yak 的 `GetOrCreatePCAPDatabase/OpenPCAPDatabase/RebuildPCAPDatabase` 返回每次调用独立的 `DatabaseHandle`。同库句柄共享最多四个只读连接，Close 只取消并释放自己的引用；最后一个管理器拥有的引用释放时关闭读池。普通 Go `*Database` 显式所有者继续保留原行为，仍可使用 `manager.Acquire(ctx,id)` 获得独立引用；有借用句柄时关闭原生共享 Database 返回 Busy，避免跨调用失效。

句柄继承当前 Yak 执行 context。列表、搜索、字段详情、BLOB 读取和输出都绑定它；额外 `queryContext` 可以收紧截止时间，不能用 Background 绕过任务取消。等待连接以及等待读池刷新锁都可取消。取消自动释放该调用的引用。管理器关闭先取消读操作，再等待操作及已从缓存移除的读池完成 checkpoint 清理，避免并发关闭卡住或在清理结束前返回。`ClosePCAPDatabases` 在 context 绑定的 Yak 模块中只释放本执行的句柄；Go 包级函数仍用于关闭默认管理器。使用 Background 的调用者需要显式 Close 或在结束时取消其 context。

这一批仅补齐离线工具的基础接口；AI 工具脚本、skill 和在线抓包任务生命周期另行实现。

Go 对应操作：

```go
db, err := manager.GetOrCreate(filename,
    pcapdb.WithProtocols(true),
    pcapdb.WithFieldIndex("$.Method"))

rows, err := db.QueryProtocols(
    pcapdb.QueryProtocol("msgpack-rpc"),
    pcapdb.QueryField("$.Method", "lab.status"),
    pcapdb.QueryField(`$."Message Type"`, "request"),
    pcapdb.QueryLimit(100))
```

多个 `field` 条件以 AND 合并。每次最多 16 个字段条件，每个子库最多 16 个可选字段索引。查询默认 100 条，上限 1000 条，使用 `after(messageID)` 游标翻页。

普通列表只加载摘要、解析来源及关联 ID。使用 `withFields(true)` 或 `ProtocolDetails` 才解码完整 Fields/Session；一次字段结果限制为 16 MiB。长查询应传 `queryContext(ctx)` 并设置调用方的超时。

| 查询意图 | 当前接口/语义 |
| --- | --- |
| 精确字符串 | `field("$.Method", "lab.status")`；区分大小写和类型 |
| 精确数字 | `field("$.Status", 200)`；整数和 REAL 按 SQLite 数值语义比较 |
| 布尔 | `field("$.Enabled", true)`；与数字 1 分开 |
| 显式 JSON null | `field("$.Optional", nil)`；字段必须存在且值为 null |
| 字段存在 | `fieldExists("$.Optional")`；null、对象、数组也属于存在 |
| 字段缺失 | `fieldMissing("$.Optional")` |
| 固定数组位置 | `field("$.records[0].key", "value")` |
| 从数组末端定位 | `field("$.records[#-1].key", "value")` |
| 带空格/点的字段名 | SQLite 路径如 `$."Request Method"`、`$.headers."x.y"` |

scalar equality 只匹配 JSON 标量。对象/数组的文本表示与字符串值分开处理。路径限制为 1024 字节、64 层；通配数组、范围、OR、任意子串和任意深度同名 key 搜索属于后续扩展。

当前 `field` 路径从 Fields 树根开始；Session 单独保留并在详情返回。后续增加 Session 查询时应使用明确的命名空间。

## 4. 当前字段索引怎样工作

每个选定路径建立一个子库局部索引，其键按下面的顺序组成：

1. `json_extract(fields, '固定路径')`：标量值。
2. `json_type(fields, '固定路径')`：JSON 类型。
3. `id`：协议消息 ID。

这是部分索引，条件为消息未软删除，且目标路径是 text/integer/real/true/false/null。缺失字段、对象和数组不加入索引，避免把大型 JSON 容器再次复制进 B-tree。

建索引使用 GORM `Model(...).Where(...).AddIndex(...)`。索引名由路径 SHA-256 派生，定义由 SQLite 内部目录持久化。相同路径重复请求幂等，新增数量受库级上限限制。

查询与索引使用同一表达式构建器。SQLite 要求查询表达式与索引表达式一致，`json_extract(fields, ?)` 的参数路径无法匹配固定路径表达式索引。当前实现验证并正确引用路径字面量，查询值继续使用绑定参数。[SQLite 表达式索引文档](https://www.sqlite.org/expridx.html)

类型参与过滤，避免 true/1、false/0、字符串/容器混淆。字符串或布尔等单一类型的等值检索可以沿索引的 ID 顺序翻页；数值同时匹配 integer/real 时可能需要合并排序，代价取决于匹配数量。

首次索引在协议数据批量落库后建立，减少初次导入的逐行索引维护。已有库上的多个新增索引在一个事务内提交；数量超限、取消或失败全部回滚。追加索引失败时，已有 Ready 数据仍可查询。

索引建立后执行有界的 `PRAGMA optimize=0x10002` 更新计划统计，并刷新本管理器已经打开的读取池，等待当前查询完成时支持取消。保留原 Database 对象。其他进程/管理器的长期连接若沿用旧统计，可 Close/Open 后重新检查计划。[SQLite ANALYZE/optimize 文档](https://www.sqlite.org/lang_analyze.html)

子库使用 WAL + `synchronous=FULL`，每个写入池只开一个连接；同一 dataset 的跨管理器/跨进程写入由 OS 文件锁串行化。读取池最多 4 个连接，使用 `mode=ro` 和 `query_only`。长期读事务会阻碍 WAL checkpoint，因此当前查询有结果上限、及时关闭 Rows，并支持查询取消，不向脚本暴露长期事务或独立 checkpointer。[SQLite WAL 文档](https://www.sqlite.org/wal.html)

新建子库目录先同步；macOS 的每个新 SQLite 连接还设置 `fullfsync=ON`。数据批次和计数在同一个 GORM 事务里提交，重试清空也保持这一规则。启动时由 SQLite 自行恢复 WAL，未提交的 BLOB 与记录原子回滚，再将失去进程锁的活动任务标记为 Interrupted。轻量校验检查版本、状态和完整的提交检查点；深校验检查计数、外键、BLOB 长度、capture/message/stream SHA-256 和流块连续性，损坏则标记 Invalid 并保留数据库。重试从原始捕获重建，不从任意 TCP 偏移恢复。[SQLite 同步设置](https://www.sqlite.org/pragma.html#pragma_synchronous)

仓库将 `yaklang/go-sqlite3` 固定到 `v0.0.2-0.20261010091851-f640de3cfce5`，内置 SQLite 升级为 3.51.3，包含官方 WAL-reset 并发缺陷修复；GORM 版本不变。驱动回归使用官方故障钩子触发 WAL 重置与 checkpoint 的竞争，关闭并重新打开后检查数据库完整性及提交内容，并验证 JSONB 和默认 FTS5 能力。使用 `libsqlite3` 构建标签时链接的是系统 SQLite，需单独确认其包含修复。本实现仍维持单 writer、不另设并发 checkpointer，应用层约束不能代替内核补丁。[SQLite 3.51.3 发布说明](https://www.sqlite.org/releaselog/3_51_3.html)、[SQLite WAL-reset 修复说明](https://www.sqlite.org/wal.html#walresetbug)

任意未建索引的路径仍可搜索，SQLite 根据可用索引选择协议、flow、时间或 ID 候选，然后读取 JSON 做剩余条件过滤。多个字段条件也由优化器选择访问路径；当前没有跨多个独立索引自动构建倒排集合交集。

## 5. 本机基准与使用边界

2026-10-10，darwin/arm64，本机单独运行基准的结果如下。表中延迟是三次 ns/op 结果的中位数，每次结果为 20 次调用的平均值，尚未采集请求延迟分位数。

| 查询 | 无字段索引 | 有字段索引 | 本例加速比 |
| --- | --- | --- | --- |
| 稀有值命中 1 条 | 25.642 ms/op | 0.183 ms/op | 约 140 倍 |
| 零命中 | 25.550 ms/op | 0.147 ms/op | 约 174 倍 |

一个字段索引的构建约 75.90 ms，SQLite 页面增量 1,124 KiB，包含新增索引和统计的内部开销，未计算 WAL 峰值。EXPLAIN 验证读取连接采用目标表达式索引，并且该字符串等值查询没有临时排序。

基准代码在 [pcap_protocol_fields_test.go](pcap_protocol_fields_test.go)。合成 50,000 条单协议消息，字段树含约 512 字节 Payload 和嵌套字段，目标值只在最后一条出现。使用完整公开查询入口、JSONB、GORM、Ready 检查和摘要投影；查询次数为每组 20 次，重复 3 次。

这是暖缓存、强选择性等值查询，导入和初始化不计入查询计时。它验证这类查询的索引收益，尚未证明 GB/TB 级 capture 能力、冷盘延迟或一般复杂查询的倍数。

新建索引要读取目标消息的 JSON，并写 B-tree 和 WAL。常用字段很短时空间代价较小；选中长文本字段会增加索引空间。高频重试/重建会维护已有索引，后续可以研究记录索引定义并在批量重建期间暂时移除、完成后重建。

## 6. 什么时候把字段拆开

先使用少量固定路径索引。如果需求变成数百/数千个动态 key、任意数组成员、跨多种字段布局的 key/value 检索，或动态范围查询频繁扫描，再研究可选的标量字段投影。

建议保留完整 JSONB，同时在**子库**新增以下 GORM 模型，实际实现时仍组合 `gorm.Model`：

| 拟议表 | 关键列 | 作用 |
| --- | --- | --- |
| `protocol_field_paths` | namespace、protocol、解析规则版本、canonical_path、last_key | 压缩路径字典并区分 Fields/Session |
| `protocol_field_values` | message_id、path_id、json_type、value_text/value_int/value_real、ordinal、element_scope | 保存被选择的标量叶子和数组作用域 |
| `protocol_field_index_jobs` | generation、policy、状态、已提交游标、错误、计数 | 管理可恢复的投影构建与完整性 |

需要优先验证的索引：

- 精确值：`(path_id, json_type, value_text, message_id)`。
- 数值范围：`(path_id, value_int, message_id)` / `(path_id, value_real, message_id)`，使用对应类型的部分索引。
- 任意同名 key/value：字典的 `(namespace, last_key, id)` 配合值索引；完整路径查询仍按 path_id。
- 反向维护：`(message_id, path_id, ordinal)`。

数组路径需要同时保留具体位置和可匹配 `[]` 的模板路径。同一消息多个命中必须返回去重的 message_id；两个条件要求“同一个数组元素”时必须关联 element_scope，直接按 message_id 合并会把不同元素误配。

missing 与 null 分开；布尔、字符串、数字分开。64 位无符号整数可另投影成规范十进制字符串，等值和范围策略分别设计；精确范围可研究定宽 uint64 编码。浮点比较、字符串排序规则、大小写和 Unicode 规范化需要明确查询契约。

按协议和路径白名单投影，给每条消息设置叶子数、值大小和总投影字节预算。若超出预算，应记录投影不完整并回退 JSON 查询，避免把部分投影误当完整索引而漏报。

投影的空间量级约为“消息数 × 每条被索引的标量数 × 每行及索引成本”。例如 2,000 万条消息、每条 30 个叶子意味着 6 亿条投影行，尚未计算 GORM 模型字段和多套 B-tree。应实测后选择字段策略。

## 7. 全文、子串和分析型查询

| 需求 | 研究方向 | 注意事项 |
| --- | --- | --- |
| Method/域名/状态码等精确值 | 当前路径索引，或少量稳定投影列 | 优先验证计划及选择性 |
| 数组中任意字段值 | 标量投影 + 数组作用域 | 去重和同元素关联 |
| Summary/命令/文本 Body 的词语检索 | 可选 FTS5 | 字段白名单、分词器、文本上限、增量维护 |
| 任意子串 | FTS5 trigram 或专门 n-gram 投影 | 短词、大小写、Unicode、索引体积 |
| 大范围 GROUP BY、跨库 JOIN、全量聚合 | SQLite 基准后再评估列式分析副本 | 解析和原始包仍以子库记录为依据 |

仓库 go.mod 已将 SQLite 驱动替换为启用 FTS5 的 yaklang fork。部署时仍应检查实际运行库的编译能力，再测试对应分词器。FTS5 的词语匹配和 trigram 子串匹配需要分别制定语义；当前 pcapdb 尚未建立 FTS 表。[SQLite FTS5 文档](https://www.sqlite.org/fts5.html)

稳定的少量字段也可以研究生成列/普通投影列与复合索引。生成列的迁移能力有具体限制，需要结合当前 GORM fork 和 SQLite 验证。[SQLite 生成列文档](https://www.sqlite.org/gencol.html)

## 8. 后续研究与验收顺序

1. 从综合样本读取完整 Fields/Session，统计协议、Rule/Entry、字段路径、类型、出现频率、值长度与基数；选择实际高频查询路径。
2. 以完整 JSON 查询为正确性基线，比对索引/投影查询的全部 message_id，再验证到原始包的关联。
3. 使用 5 万、100 万、1,000 万消息的独立数据集；真实 capture 验证解析语义，合成数据控制选择性、数组深度、JSON 大小与类型分布。
4. 覆盖稀有值、热点值、零命中、数值范围、多个字段 AND、数组同元素、缺失/null、最后一页和完整聚合。
5. 记录 p50/p95/p99、计划、扫描量、JSON/索引页数、WAL 峰值、RSS、分配量、导入和索引构建时间。冷盘和暖缓存分开测量；并发读测 1/4/16/32，写入仍按 SQLite 单 writer 设计。
6. 投影先构建新的 generation，事务提交检查点，校验消息覆盖范围及数量后发布可用状态。启动时只检查状态和游标，显式恢复或重建；旧 generation 在成功切换后清理。
7. 测试构建取消、进程中断、规则变更、catalog 丢失、批次回滚、索引缺失、超预算回退和已有读取连接刷新。加速结构可以重建，完整 JSONB 与原始包保留。

复现现有测试和字段查询基准：

```sh
go test ./common/pcapx/pcapdb -race -count=1
go test ./common/yak -run '^TestPCAPDBYak' -count=1
go test ./common/pcapx/pcapdb -run '^$' \
  -bench '^BenchmarkPCAPDBProtocolFieldSearch$' -benchmem -benchtime=20x -count=3
```

约 198 KiB 的综合压缩样本来自 bin-parser 已有的 24 份捕获，共 3,368 个包。测试逐个消息比较完整字段树、会话快照、Rule/Entry、包关联和重组字节，并验证原样导出。字段类型测试另外覆盖 true/1、false/0、null/缺失、对象/字符串、数组位置、大整数、多个条件、路径引用、索引幂等和事务回滚。

并发测试固定 4 个物理读取事务，验证单 writer 提交、快照隔离、只读保护和锁等待取消；另有 24 个并发公开查询与字段索引构建。故障测试在独立测试子进程的真实导入事务中调用 Process.Kill，覆盖包批次、重试清空、协议批次、Ready 发布和未提交索引，验证计数/关联/JSON/导出及去重；还截断实际已写出的未提交 WAL 帧，验证 WAL-index 重建和末尾恢复。所有故障文件均在测试临时目录内。强杀及文件级撕裂模拟不等同于真实断电；物理持久性仍依赖系统和存储正确执行同步，Windows 目录刷新尚无同等实现与实机验证。

## 10. 会话、流内容和界面加载

默认同时建立 session/stream。`withStreams(false)` 可以跳过重放，做纯包导入；后续在相同 DatasetID 上补建。`withProtocols(true)` 增加 BIN Parser 解析和 JSONB 信息。建立/重建分析时统一切换为 Analyzing，全部完成才发布 Ready；启动恢复 Interrupted 后显式重试，不从任意 TCP 包中点恢复。

`sessions` 表示捕获域内一次双向通信，TCP 端点复用会创建新 session；`streams` 是其中两个方向。Section、Interface 和 Encapsulation 参与身份，避免把不同捕获接口或隧道的相同端点合并。UDP 按端点及 30 秒空闲窗口分组，流 Kind 为 datagram，块边界保留数据报语义。

```yak
sessions = db.QuerySessions(pcapdb.transport("tcp"), pcapdb.limit(100))~
streams = db.QueryStreams(pcapdb.session(sessions[0].ID))~
packets = db.QueryPackets(pcapdb.stream(streams[0].ID), pcapdb.limit(100))~
messages = db.QueryProtocols(pcapdb.session(sessions[0].ID), pcapdb.limit(100))~

cursor = 0
for {
    page = db.ReadStream(streams[0].ID, pcapdb.after(cursor),
        pcapdb.limit(20), pcapdb.maxBytes(262144))~
    // 交付/处理当前页后释放内容，不把所有页积累成一个完整 stream。
    for chunk in page.Chunks { println(chunk.ID, chunk.ByteOffset, len(chunk.Data)) }
    if !page.HasMore { break }
    cursor = page.NextAfter
}
```

普通包/协议列表不取字节 BLOB；JSON 详情显式请求。摘要和内容查询采用同一个只读事务检查 Ready 并读取结果，避免看到半次重建。所有列表采用 ID 游标，上限 1,000 行。`ProtocolPacketIDs` 与 `StreamChunkPacketIDs` 也支持 after/limit 分页，关联索引可以反向追踪；按 session/stream 取包从关联游标开始，通过 CROSS JOIN 查包主键，不先物化整条流的 ID 集合；`PacketSessions` 查询包所属会话。Protocol 返回的 SessionID/StreamID 为整数，0 表示没有可确认的关联（数据库保存 NULL），不按端点猜测。

每个流块最多 64 KiB；ReadStream 默认最多 256 KiB 原始字节，允许 64 KiB..1 MiB，且仍受行数上限约束。这个预算是原始字节预算，JSON/base64 传输会产生编码开销。ReadPacket/ReadProtocol 是显式单记录读取，最高 16 MiB。JSON 字段导入上限为单消息 8 MiB，字段查询结果上限 16 MiB，超过预算返回错误而不截断字段树。

重放使用一个同步 worker，不保留完整 TCP stream，慢的 SQLite 批次施加背压。TCP 最多 4,096 个活动 flow，单方向等待乱序字节最多 2 MiB，捕获整体最多 16 MiB，另有 segment 数量上限；UDP 的活动会话 LRU 也最多 4,096 个。导入批次除行数外采用 8 MiB 字节目标，单个合法大包/消息可超过目标，但仍受单记录预算限制。

只做流索引时，乱序/重传使用已有重组器处理，保留确认的顺序字节。遇到缺口、非法段或资源上限时，保留可读内容并在 session 中记录 HasGaps/Complete/Midstream/CloseReason；缺口不拼接，EOF 不伪装成 FIN。启用协议解析仍遵守现有严格错误语义，失败保留已提交记录，Ready 查询拒绝该库。对首见字节加较长重传后缀等不能确认完整逐包来源的块，ReferencesComplete=false；流级索引仍保留控制包和重传包，可回到原包调查，不伪造精确来源。

capture_records 按 ID 顺序拼接 Prefix、关联的 Packet.Data、Suffix，包字节只保存一份。重放、深校验与导出一次仅加载一个有界记录，每 64 条结束读快照，避免整个捕获重放固定住 WAL。导出持有共享实例文件锁，多个导出允许并行，写入/重建拿独占锁。发布输出采用同目录临时文件、完整 SHA 校验、Sync 和独占发布，已有文件不覆盖。

关闭读池后，由取得实例独占锁的 writer 做 PASSIVE checkpoint。完全停止该子库的所有使用者后，可携带单个 SQLite 文件及 UUID 目录，启动时从子库检查点重建 profile 登记并更新路径；运行中的库要连同 WAL 保留，不能仅复制 .sqlite 文件。schema 4 不自动转换此前未发布的侧文件原型（schema 3）：原型库保留并报告 UnsupportedSchema，需另行明确迁移/重新导入。

冒烟脚本：`common/yak/yaktest/mustpass/files/pcapdb.yak`。按已有 mustpass 路径运行，覆盖导入/去重/登记/校验/包分页/会话/双向流/协议字段/源文件删除/原样导出/关闭再打开。Go 测试另覆盖并行 reader、乱序与重传、缺口、流分页、来源证据受限、跨域隔离，以及 packet/session/stream/protocol 事务中真实进程 kill 和未提交 WAL 尾帧损坏。

如果原始包 BLOB 完整，`pcapdb.RebuildPCAPDatabase(id)` / `db.RebuildAnalysis()` 可在不需要源文件的情况下重新解析和修复派生的协议/session/stream 数据。它先重建捕获字节并核对完整 SHA，确认后才进入 Analyzing 并清空派生表。捕获字节损坏或导入尚未完成时拒绝重建，保存损坏内容及 Invalid 诊断，需用户重新提供原捕获文件；不会用派生流反推或填补原始包。
