# pcapdb 流量查看器 gRPC：第一阶段实现

## 本轮目标与边界

状态：后端接口已实现并通过真实内存 gRPC 测试。前端接线和 PCAP 专用旧入口清理留作下一步。

目标是让数据包列表、协议详情树和字节视图形成完整交互：选择一个原始捕获帧，可以检查它自己的包头；选择完成帧上的应用协议字段，可以查看并高亮相应重组消息中的字节。

- 列表一行仍对应 `packets` 的一条捕获记录，分页和精确计数都以去重后的 packet 为单位。
- 应用协议过滤沿用当前完成帧语义，例如 HTTP 过滤命中完成 HTTP 消息的帧；一个帧完成多条消息也只占一行。
- 详情提供该帧完成的各条应用消息及其字节源。非完成的贡献帧显示自己的字段和重组去向，支持跳转完成帧。
- 完整重组数据在本轮指一条协议消息的完整可用字节；会话、整条双向 TCP 流和 Follow Stream 留到后续。
- 当前为内部开发阶段，允许直接调整 PCAP proto 和数据库 schema，通过重新导入验证新结构，不增加旧 PCAP 数据迁移或双版本兼容路径。
- 保留现有数据集导入、查询、删除和导出能力。本轮不增加展示索引表，不重写已有 TCP 重组逻辑。

## 接口分工

数据浏览使用三个 RPC。列表与详情各自有明确的返回模型；字节读取独立出来，是为了支持字段跳转和大消息随机定位。

| RPC | 请求 | 响应与职责 |
| --- | --- | --- |
| `QueryPCAPPackets` | `DatasetID`、`Filter`、`Pagination`、可选 `IncludeTotal` | `Data: PacketSummary[]`、`NextCursor`、`HasMore`、可选精确 `Total`；只返回列表信息 |
| `GetPCAPPacketDetail` | `DatasetID`、`PacketID` | 单个包摘要、协议树、字节源描述、重组关系；小额初始字节可随详情返回 |
| `ReadPCAPBytes` | `DatasetID`、`SourceID`、`Offset`、`Length` | `Data: bytes`、`Offset`、`TotalLength`、`EOF`；读取指定字节源的一个区间 |

`QueryPCAPPackets` 替代现有 PCAP 列表查询契约，不与 `QueryPCAPPacket` 长期并存。新前端完成切换后清理 `QueryTrafficPacket` / `ParseTraffic` 的 PCAP 专用适配。其他仍在使用老 traffic 接口的功能不属于这次重构范围，删除前检查调用方。

沿用项目 CRUD 的 `Filter`、`Pagination`、`Data` 组织方式和 gRPC 错误处理。HTTPFlow 本身也有独立的查询、按 ID 获取以及正文读取接口，不把 `Full` 作为这轮设计的约束。

实现位置：

- 列表与分页：[grpc_pcapdb_query.go](../common/yakgrpc/grpc_pcapdb_query.go)、[pcap_packet_view.go](../common/pcapx/pcapdb/pcap_packet_view.go)
- 字节读取：[grpc_pcapdb_bytes.go](../common/yakgrpc/grpc_pcapdb_bytes.go)、[pcap_data_range.go](../common/pcapx/pcapdb/pcap_data_range.go)
- 单包详情：[grpc_pcapdb_detail.go](../common/yakgrpc/grpc_pcapdb_detail.go)、[pcap_packet_detail_query.go](../common/pcapx/pcapdb/pcap_packet_detail_query.go)
- 展示字段和范围：[display_field_tree.go](../common/pcapx/pcaputil/display_field_tree.go)

## 核心返回结构

```text
PacketDetail
  Packet: PacketSummary
  Nodes: FieldNode[]
  ByteSources: ByteSource[]
  RelatedMessages: MessageRef[]

FieldNode
  Name / DisplayValue
  Children[]
  Ranges[]: {SourceID, BitOffset, BitLength}

ByteSource
  ID / Kind / Name
  Length / Completeness
  InitialChunk?: {Offset, Data}

MessageRef
  MessageID / Protocol / SourceID
  CompletedInPacketID / ContributorPacketIDs[]
  Completeness
```

- `SourceID` 在 `DatasetID` 内标识一个稳定字节源，例如 `frame:38` 和 `message:51`。读取时由服务端验证类型、记录及数据集归属。
- 字段范围从所属 source 的起点计算，使用位偏移和位长度，以支持 TCP flags 等位字段；读取接口的 Offset/Length 使用字节单位。
- 一个字段可有多个不连续范围。计算值、上下文值或尚无可靠定位的字段可以没有范围，不能用文本搜索猜偏移。
- `ByteSource.Length` 是实际可读取的字节长度。`EOF` 表示读到该 source 末尾，不代表原始消息完整；完整性由 `Completeness` 单独表达。
- HTTP body 的解压结果与原始消息不是同一字节源。本轮先展示原始重组消息；解压、解密等变换字段没有对应 source 时不标注错误范围。
- 一帧完成多条消息时，每条消息各有协议子树和字节源，不只保留第一条。

例如，点击第 38 帧的 TCP Sequence 字段，高亮 `frame:38` 的对应范围；点击该帧完成的 HTTP Status 字段，切换到 `message:51`，按需读入范围并高亮。前端手动选择字节时，也可用同一组范围定位已返回的字段。

## 实施顺序与验收

### 1. 定义 proto 和公共模型（已完成）

在 `common/yakgrpc/yakgrpc.proto` 定义三个查看接口及上述结构，生成绑定，保留数据集管理 RPC。明确 int64 在前端的精确表示、缺失时间和端口零值、范围单位及错误码。

列表只提供 ID 升序的 `AfterId + Limit` 游标分页，默认 100、最大 1000；前后翻页由前端保存游标历史。本轮不提供任意页码跳转或自定义排序。

`IncludeTotal` 使用可选字段：未设置时首页计算、后续游标页不重复计算；显式为 true 时任何页都返回精确总数。Total 按完整筛选条件统计，不受游标影响；没有统计时缺失，不能返回 0 代表未知。更换筛选条件须重置游标和总数。

验收：能用一个多包 HTTP 消息完整表达列表、完成帧详情和节点字节选择；空结果与未知总数可区分。

### 2. 补齐字段范围（已完成）

在 `protocol_messages` 增加 `display_fields` BLOB，保存解析器已经确认的可靠范围和相对位置；与消息字节、完成帧和贡献关系在同一导入批次提交。数据层使用独立 Go 模型，不依赖 gRPC 类型。

主要修改 `pcap_database_schema.go`、`pcap_protocol.go` 和 pcaputil 的协议字段输出。复用 `DisplayFieldValues()` 已有的可靠范围，但不重复保存没有范围的字段值；完整字段值继续使用原有 `fields`。

HTTP/1 请求行、响应行、重复 Header、固定 body 和 chunked body 的范围直接由重组消息前缀生成，在 `GetPCAPPacketDetail` 时计算，不写入数据库。这样避免为每条 HTTP 消息重复保存几百字节的展示树，同时仍能返回准确范围。

原始帧的 Ethernet/IP/TCP/UDP 字段继续由详情时的单帧解码产生，并校正位字段范围。应用协议的重组和范围计算在导入阶段完成。

验收：HTTP 字段范围可以从持久化消息字节中取出正确内容；重复同值 Header 不串位；chunked body 的 Size 与 Content 分别定位；范围不越界；未知或计算字段不伪造范围。缺包、解析资源限制、body 省略等状态必须保留，不能把预览标为完整数据。

### 3. `QueryPCAPPackets`（已完成）

复用 `pcap_packet_view.go` 的列表、完成帧筛选和精确计数，将当前 PCAP 查询能力收敛到新接口。分页继续作用于 packets，页内只批量补充完成消息的协议和摘要。

沿用 `messages_completed_in` 和 `messages_protocol_completed` 的现有用途：前者服务当前页/单帧的完成消息查询，后者服务协议过滤下的精确去重计数。移除这条新路径中探测旧 schema、从 JSON 解析完成帧、列表补读原始字节的兼容分支。

验收：HTTP 筛选只返回完成帧；一帧多消息不重复计数；翻页无遗漏或重复；列表查询不加载 raw、fields 或 display_fields。接口完成后单独汇报请求、响应和测试结果。

### 4. `ReadPCAPBytes`（已完成）

在 pcapdb 增加按范围读取 packet/message BLOB 的能力，gRPC 只做数据集定位、范围校验和模型转换。复用现有快照、取消及句柄生命周期。

请求长度必须有上限，起点等于末尾返回空数据和 EOF，越界、负值、溢出和非法 source 明确报错。大消息读取按区间进行，不能先调用全量 `ReadProtocol` 再在 Go 中切片。现有 `substr` 预览可作为起点，需实测 SQLite 层的内存和 IO，不能仅凭响应分块认定数据库读取也已分块。

验收：逐块拼接与持久化原字节完全一致；随机读首、中、尾正确；取消读取释放资源；大消息读取不会重新触发重组。接口完成后单独汇报。

### 5. `GetPCAPPacketDetail`（已完成）

将当前 `PacketMessages` 一次性加载字段和整个消息 BLOB 的方式拆成元数据/展示字段查询与字节读取。详情按 `completed_in_packet_id` 加载本帧完成消息的协议树；通过 `message_packets` 查询贡献关系和重组去向。

用强类型 Nodes/ByteSources 代替 `Details string` 和内嵌 base64 RAW。初始字节设置总预算，正文大小不决定详情能否打开；树和引用的独立预算也必须明确，超限不能静默冒充完整结果。解析失败时仍返回可用原始帧及诊断。

验收：跨包 HTTP 完成帧展示完整消息的可读 source；非完成帧可跳转完成帧；一帧多条消息都可查看；超过旧详情 1 MiB 预算的消息可以正常打开并分块读取。接口完成后单独汇报。

### 6. 前端切换与整体回归（后端完成，GUI 待接入）

前端改为列表调用 Query、选包调用 Detail、字节窗口按需调用 Read。点击协议节点按 SourceID 切换字节页并定位范围；快速切包时取消旧请求，避免旧 source 数据污染当前视图。

按同一数据集和筛选条件管理游标、精确总数及字节缓存；更换数据集或重新导入后清理相关状态。切换完成后删除 PCAP 专用的 Full/Details JSON 适配与重复入口，并更新调用文档和测试。

验收先覆盖单帧 UDP、跨包 HTTP、多消息共用完成帧、重复 Header、chunked body、缺包和大消息，再复用此前 79 万/311 万包的数据做回归。分别记录导入耗时与库体积增量、精确 COUNT、首页、后续页、详情、随机字节读取的延迟和峰值内存。不要只用翻页速度代表完整性能。

这轮完成标准：用户能在实际 GUI 中打开完成帧，浏览重组消息，并通过协议节点准确选中属于该消息的字节。

## 已完成的验收

功能覆盖：

- UDP 单帧、跨包 HTTP、请求与响应分别完成、贡献帧到完成帧的跳转。
- 一帧完成多条消息时，列表只计一帧，详情返回全部消息。
- HTTP Status、重复 Header、固定 body 和 chunked body 的字段范围。
- 180,000 字节响应详情正常打开；详情只带 64 KiB 初始前缀，其余通过 `ReadPCAPBytes` 读取并拼接为原始完整响应。
- `Total` 可选精确计算；后续游标页未统计时保持缺失，显式 `IncludeTotal=true` 可重算。
- 非法 source、缺失消息、越界范围、负偏移、超过 8 MiB 的单次读取均有明确 gRPC 错误。

真实抓包测试：

| 抓包 | 原始帧 | 协议消息 | 导入耗时 | 子库大小 | 新增范围数据 |
| --- | ---: | ---: | ---: | ---: | ---: |
| `bigFlows.pcap`，351 MiB | 791,615 | 306,579 | 88 秒 | 1.95 GB | 30.0 MB |
| `DoS-HTTP_Flood1.pcap`，约 1.4 GiB | 3,114,983 | 1,375,996 | 4 分 10 秒 | 6.76 GB | 65.7 MB |

311 万帧结果与原导入路径此前约 4 分 13 秒的耗时处于同一水平。展示范围没有按消息值重复存储；HTTP 范围按详情请求从消息前缀恢复。

同一 311 万帧数据集的 HTTP 查询实测：

| 操作 | 耗时 |
| --- | ---: |
| HTTP 完成帧总数 | 285,174 |
| 首页冷查询（含精确 COUNT） | 3.75–3.86 秒 |
| 首页热查询（含精确 COUNT） | 211 毫秒 |
| 后续 100 行游标页 | 4–5 毫秒 |
| 单包详情（3 条消息、初始字节） | 1–2 毫秒 |
| 单次 1 字节范围读取 | 83 微秒 |

## 历史：上一版实现与契约

以下保留改造前的实现记录，便于定位现有代码；其中共用接口、旧数据兼容及 Full 的设计不再约束上面的新计划。

数据集管理接口已实现；流量查看器现改为扩展老 `QueryTrafficPacket`，不传 DatasetID 保持旧行为。定义见 [yakgrpc.proto](../common/yakgrpc/yakgrpc.proto)，兼容实现见 [grpc_traffic_pcapdb.go](../common/yakgrpc/grpc_traffic_pcapdb.go)，测试见 [grpc_traffic_pcapdb_test.go](../common/yakgrpc/grpc_traffic_pcapdb_test.go)。

### 改造前前端使用的契约

入口位于 Yakit「实验性功能 → 流量分析」。列表和详情都调用 `QueryTrafficPacket`：

```text
列表：{DatasetID, Full:false, SrcIP, DstIP, SrcPort, DstPort,
       TransportLayerTypes, ApplicationLayerTypes, Keyword,
       FromId, Pagination:{Page:1, Limit:100}}
详情：{DatasetID, Id:"38", Full:true}
响应：{Data:TrafficPacket[], Total, NextCursor, HasMore, Truncated}
```

- 一行始终对应 `packets` 的一条原始捕获记录。消息过滤用 EXISTS，在计数与分页前执行；多条消息不会把一个包重复计数。
- `Total` 是筛选后的原始包总数，不随游标变化。支持 ID 升序的 Page/Limit 或 FromId/AfterId，页码和游标不能混用；前端使用游标历史前后翻页。
- 旧请求的 IP、端口和协议字段支持逗号、空格或 `|` 分隔的同字段 OR，不同字段 AND；端口字符串 `"0"` 是有效筛选值。网络层支持 IPv4/IPv6，链路层支持常用名称或数字类型。
- `Keyword` 是端点、传输协议或已保存协议摘要的字面子串，不是 Wireshark 显示过滤语法，也不扫描 Raw。StartTimestampNS/EndTimestampNS 筛选捕获时间的半开区间。
- pcapdb 模式拒绝旧 SessionId/TimestampNow，避免把会话 UUID、入库时间误当成新库的会话编号或捕获时间。
- 列表不读取 Raw、Payload、协议字段 BLOB。Full 增加当前包 Raw/Payload 和 Details JSON；单条过大时返回 ResourceExhausted，整页超过 32 MiB 则缩短页并返回 Truncated/HasMore。
- int64 的 ID、游标及纳秒时间在 Electron 中保持字符串，避免 JavaScript 浮点数损失精度。

`TrafficPacket.Details` 的 JSON 结构：

```text
{
  Result: [旧 ParseTraffic 的 {name,value} 协议树],
  ProtocolsIndexed: true,
  Messages: [{
    MessageId, Protocol, Info, Status, Completeness,
    PacketIds, CompletedInPacketId, Kind,
    RAW: "消息字节的 base64", Result: [消息字段树], DecodeError
  }],
  DecodeError
}
```

原始包 Raw 和每条消息 RAW 是独立字节源。节点叶值沿用 `{leaf:true,verbose,scope?}`；scope 是相对于该树字节源的位区间 `[start,end)`。原始包头可提供可靠偏移；已保存应用字段没有可靠偏移时不生成 scope，不重新解析应用消息、不把消息偏移映射到当前 TCP 段。前端据此分别展示 HEX/ASCII/UTF-8 与协议树。

`ParseTraffic({DatasetID,Id,Type:"packet"})` 也兼容同一详情，响应保持 `{OK,Result,Message}`；Result 内增加当前包 RAW。前端无需再调用这个独立详情接口。

导入现在启用 `WithProtocols(true)`、`WithStreams(false)`。TCP 重组和协议分析在导入时完成；查询只读 `protocol_messages`、`message_packets`。旧的仅包索引库仍可查看原始包，重新导入相同文件会补建协议索引。CompletedInPacketId 是最后贡献完整消息字节的捕获包，不保证所有异常场景与 Wireshark 的展示归属相同。

已经过真实 gRPC 与 Electron 组件链路验证：Wireshark HTTP 示例共 43 个原始包；第 38 包为 478 字节，其 HTTP 消息为 18,364 字节，关联 14 个贡献包。额外单测覆盖同一包多条消息的计数去重、过滤分页、旧库不带 DatasetID 的兼容、精确详情、不可靠字段不高亮、导入订阅清理和过期响应隔离。

### 1. 已确定的方向（上一版）

- 复用 pcapdb 现有存储、解析和查询能力。
- 第一阶段聚焦数据包，会话和流查询留到后续阶段。
- 按返回模型划分接口，不设置通用 QueryType。
- 前端的数据包列表、过滤查询、单包详情共用扩展后的 `QueryTrafficPacket`；先前实现的 `QueryPCAPPacket` 保留为原始包基础接口。
- 参考 HTTPFlow 的 `Full`：控制返回结构完整度，不改变查询对象、过滤语义或响应类型。

### 2. 第一阶段的 5 个接口（上一版）

| RPC | 职责 |
| --- | --- |
| `ImportPCAPFile` | 导入 PCAP/PCAPNG，服务端流返回进度和 DatasetID；建立包索引并保存协议分析 |
| `QueryPCAPDatabase` | 查询数据集列表；按 DatasetID 获取对应元信息 |
| `QueryPCAPPacket` | 过滤查询数据包，Full 控制摘要/完整结构 |
| `DeletePCAPDatabase` | 按 DatasetID 删除数据集，保留底层清理状态 |
| `ExportPCAPData` | 导出完整捕获文件，或指定包的原始字节 |

本阶段不增加独立 GetPacket、ReadPacket、协议消息查询、会话查询、流查询或重建管理 RPC。已有底层分析能力保持复用，不因接口分阶段而重写存储模型。

### 3. 对齐 HTTPFlow 的查询设计（上一版）

代码依据：

- [yakgrpc.proto](../common/yakgrpc/yakgrpc.proto)：`QueryHTTPFlowRequest.Full` 和 `IncludeId`。
- [grpc_httpflow.go](../common/yakgrpc/grpc_httpflow.go)：`QueryHTTPFlows` 先查询，再把 Full 传入模型转换。
- [yakit/httpflow.go](../common/yakgrpc/yakit/httpflow.go)：`BuildHTTPFlowQuery` 在数据库投影阶段控制大字段读取。
- [grpc_note.go](../common/yakgrpc/grpc_note.go)：参考 Filter、Pagination、Data 和操作结果的组织。

下述为仍保留的原生 `QueryPCAPPacket` 契约；新前端使用上文的老接口扩展。原生数据包请求形状：

```text
QueryPCAPPacketRequest
  DatasetID
  Filter
    Id
    Transport
    SourceIP / DestinationIP
    SourcePort / DestinationPort
    TimeRange
  Pagination
  Full
```

数据包响应形状：

```text
QueryPCAPPacketResponse
  Pagination
  Data[]             // 始终为同一种 PCAPPacket，包括单包详情查询
  Total              // 仅在有准确值时提供，未统计不能伪装为 0
  NextCursor
  HasMore
```

使用方式：

```text
列表：DatasetID=...，Filter.SourceIP=...，Full=false
详情：DatasetID=...，Filter.Id=123，Full=true
完整过滤结果：DatasetID=...，Filter.Transport=udp，Full=true
```

Full 仅控制投影，不强制要求按 ID 查询，也不隐式切换成单条响应。`Filter.Id + Full=true` 是前端请求单包详情的常用组合。

- `Full=false`：返回时间、地址、端口、长度、链路类型等摘要，查询时不加载完整包 BLOB。
- `Full=true`：返回相同模型，并补齐完整原始字节及当前包模型的详情字段。现有 Packet 的重字段主要是原始 Data，协议消息 Fields 属于另一种模型，不在这里承诺新增协议解析树。
- 响应预算为 32 MiB。大结果缩短页并保留 NextCursor，设置 HasMore/Truncated；Full 返回的单个包字节不静默截断。单条无法放入响应时返回 ResourceExhausted。

### 4. 复用与最小适配（上一版）

第一阶段沿用 pcapdb 的 ID 游标，使用 Paging 的 Limit/AfterId。Limit 默认 100，最大 1000；Page 只能省略或为 1，排序固定 ID 升序。BeforeId、RawOrder、其他排序及页码跳转返回 InvalidArgument。

QueryPCAPDatabase.Total 是当前登记查询的总数。QueryPCAPPacket.Total 使用 optional：无筛选时返回整库包数；指定 Id 且没有 AfterId 时返回匹配条数 0/1；其他过滤查询不提供 Total。前端不能将缺失的 Total 解释为 0，应按 HasMore/NextCursor 翻页。

handler 直接调用 pcapdb，负责参数校验、模型转换和进度返回。列表复用现有摘要投影，Full 模式按需读取完整字节；没有新增通用查询框架或修改数据库结构。底层新增 GetOrCreateHandle 以正确释放导入租约，以及 DeleteResult.AffectedRows 以报告真实删除行数。

继续保留已有的 DatasetID 隔离、请求取消、句柄释放、完整导出及删除清理语义。纳秒时间和端口过滤保留“未设置”与数值 0 的区别。

### 5. 调用示例（上一版）

以下是字段形状示例；int64 在具体前端绑定中应按其 64 位整数表示处理。

```text
ImportPCAPFile({InputFile: "/engine/capture.pcap"})
  → 持续接收进度；仅 Finished=true 表示完成，最终 Database 包含 DatasetID

QueryPCAPDatabase({Filter: {DatasetID: "返回的 UUID"}})

QueryPCAPPacket({DatasetID: "...", Filter: {SourceIP: "10.0.0.1"},
                 Pagination: {Limit: 100}, Full: false})
  → Data、NextCursor、HasMore；后续把 NextCursor 放入 Pagination.AfterId

QueryPCAPPacket({DatasetID: "...", Filter: {Id: 123}, Full: true})
  → 同一个 Data 列表，匹配包的 Raw 为完整捕获字节

ExportPCAPData({DatasetID: "...", TargetPath: "/engine/output.pcap"})
  → 完整捕获文件；指定 PacketID 时输出该包原始字节，而非独立 PCAP 文件

DeletePCAPDatabase({DatasetID: "..."})
  → Deleted=true 且 CleanupPending=false 表示清理完成
```

InputFile 和 TargetPath 都是引擎所在机器上的路径。导出不覆盖已有文件，并返回路径、字节数、SHA256。导入来源文件删除后仍可读取和导出。

删除遇到活跃读取/写入时返回 Aborted；逻辑删除已提交但清理失败时，返回 Deleted=true、CleanupPending=true 和 CleanupError，可使用同一 DatasetID 重试。重复删除成功时 EffectRows=0。原始捕获文件和已有导出文件均保留。

### 6. 验证（上一版）

每个接口均通过实际内存 gRPC 传输验证，覆盖重复导入、断开取消、数据集分页、Full 开关、过滤及时间边界、1000 条分页边界、精确导出、禁止覆盖、删除占用保护与清理重试。

```sh
go test ./common/pcapx/pcapdb -count=1
go test ./common/yakgrpc -run '^TestPCAPGRPC' -count=1
```

关键验收：同一筛选条件下 Full 开关不改变记录集合；单包详情仍返回统一 Data 列表；完整模式字节与捕获一致；摘要查询不读取完整 BLOB；分页无遗漏或重复；空结果、未就绪、查询失败可以区分。
