# HTTP/2 与 MySQL 实时会话解析

`pcapx` 在观察到 HTTP/2 客户端 preface 或 MySQL v10 服务端 greeting 后建立连接状态，
从报文确定角色，支持非标准端口。TCP 乱序与重传先经过现有重组器，随后才进入协议分帧。
没有握手时不会把端口号当作方向、版本、认证或协商能力的证据。

```go
err := pcaputil.ReplayPcapFile("session.pcap", pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) {
    // Fields 来自严格的 YAML 消息入口；Session 是该消息时刻的独立上下文。
    fmt.Println(e.Protocol, e.Status, e.Summary, e.Session)
}))
```

同样的选项可用于 `Start` / `Sniff`。Yak 使用 `pcap_onProtocolMessage`，按 `http2` 或
`mysql` 过滤即可。`Direction` 仍表示首次观察方向；角色在 `Session` 中单独给出。

## HTTP/2

- 明文 prior-knowledge preface、双方初始 SETTINGS、逐批 ACK 和双向连接状态。
- 持续维护每个方向的 HPACK 动态表；SETTINGS 约束作用于对向编码器。
  未 ACK 的降低限制允许在途数据；ACK 后检查缩表及连续降低/增加中的最小表尺寸更新。
- HEADERS 与同 stream 的连续 CONTINUATION 合为一个事件，TCP 分片和粘包不改变结果。
  解压后的头保存在 `Session.Headers`，不为它们编造线上的叶字段字节位置。
- 多 stream 请求/响应关联、信息响应、尾部头、DATA、END_STREAM、RST_STREAM、GOAWAY；
  检查常见伪头约束、半关闭方向、流 ID 重用及连接/流窗口。
- `Session` 包含 `Stream ID`、`Header Kind`、`Headers`、`Request Method`、`End Stream`
  和 `Exchange Complete`（适用时）。它表示观察到的消息结束，不证明业务执行成功。

首版不处理 PUSH_PROMISE、扩展 CONNECT、HTTP/1.1 h2c Upgrade、TLS 解密或任意中途
接入的 HPACK 字典恢复。不声称完整 HTTP 语义验证，例如 Content-Length 与正文一致性。
缺失请求或遇到未支持的会话阶段会产生 `context-required`；错误帧/状态产生 `malformed`。

本地上限：每个消息/头块 1 MiB（也受调用者更小的 `MaxMessageBytes` 限制），
1024 个头块帧、1024 个活动 stream、每方向 128 批未确认 SETTINGS、64 KiB HPACK 表、
4096 个解压头、单个解压字符串 65536 字节、解压头合计 1 MiB。
字典和会话容器按保守容量占用计入共享 `MaxBufferedBytes`，关闭连接后释放。

## MySQL / MariaDB

- 校验 v10 greeting、4.1 客户端握手、实际能力交集、方向、认证阶段和包序号。
- AuthSwitch、客户端认证续包、`caching_sha2_password` AuthMoreData、OK/ERR。
  `Authentication OK Observed` 只表示捕获到服务端 OK，不校验凭证。
- SSLRequest 后切换到 TLS 记录分帧；不把后续密文当作 SQL 或结果行。
- COM_QUERY、COM_INIT_DB、COM_PING、COM_QUIT；每个命令分配连接内事务 ID，响应沿用该 ID。
- 普通 OK、SESSION_TRACK OK、ERR、带完整列定义的文本结果集、NULL/空值与多结果集。
  支持传统 EOF 和 MySQL DEPRECATE_EOF 的 OK 终止包（含 SESSION_TRACK）；
  MariaDB CACHE_METADATA + EXTENDED_METADATA 组合仅接受携带新鲜元数据的传统 EOF 结果。
- 多包结果集按有界完整结果交付，扫描游标在完整包边界恢复；粘在一起的下一结果保留
  独立事件与相同事务 ID。字段解析仍走 `mysql_fields.yaml`，不另造简化结果树。

首版不处理 prepared/binary 命令与行、压缩传输、QUERY_ATTRIBUTES、LOCAL INFILE、
结果集内部中途 ERR、缓存列元数据以及 0xffffff 续包。这些路径明确要求其他 profile，
不会被标记为已解码。结果中出现 `MORE_RESULTS` 必须已有协商能力。
每个结果最多 1 MiB、4096 列/累计行值，分帧最多 8194 个包；适用更小的调用者上限。

## 生命周期、所有权与延迟模式

一个连接上的帧/会话错误会使双方的状态失效；保留诊断，随后只计数，避免错误后猜测状态。
连接结束时，未达到消息边界的字节和未完成的请求/响应会报告 `incomplete`。

每条事件拥有 `Raw`、`Fields`、`Session`。`Decode()` 返回 `fields`、`metadata` 及独立的
`session` 快照。`ProtocolInspector` 的内存预算同时计入原始字节和会话快照；列表省略
快照，详情返回独立副本，不依赖已关闭连接或当前 HPACK 字典。

延迟模式仍需执行严格消息校验和会话推进（包含字段解码与 HPACK）。回调不交付字段树，
详情时重新执行无状态字段解析，并附上当时的会话快照；因此它不代表省去了全部解码成本。

## 样本与验证

固定真实捕获沿用已有 corpus，不修改或重打包：

| 捕获 | 本次实时结果 | 独立字段断言 |
|---|---|---|
| `ndpi/ndpi-http2.pcapng` | 12 条消息、591 字节，全部解码 | POST 请求与 201 响应头，stream 关联 |
| `ndpi/ndpi-mysql.pcapng` | 28 条消息、4271 字节，全部解码 | MariaDB 查询 `select @@version_comment limit 1`、结果 `Ubuntu 22.04`、第二连接的 SSLRequest/TLS |

来源为 nDPI commit `4cae778e7e8f846b34f11d4f8392504cdebd3db8`；原始 URL、许可和
SHA-256 保存在 [corpus sources.json](../../bin-parser/testdata/protocol-corpus/sources.json)。
本次测试也锁定这两份文件的 SHA-256。

[protocol-sessions](testdata/protocol-sessions/README.md) 另含四份明确标记为生成数据的 pcap，
用于多路流、HPACK 动态引用、CONTINUATION、认证切换、传统/新 EOF、多结果、错误和退出。
它们不是生产流量。生成器和真实捕获互相补充，不把生成器当作唯一正确性来源。

```sh
# 实时链路与对抗测试；包含 pcap/pcapng、IPv4/IPv6、1/2/4 worker、逐字节分片。
go test ./common/pcapx/pcaputil -run '^TestLive' -count=1
# 完整既有协议语料和回归。
go test ./common/bin-parser/... ./common/pcapx/pcaputil/... ./common/pcapx/cmd/... ./common/utils/embeddedfs/... -count=1 -timeout=10m
# 会话、查看器与既有协议回调的并发验证。
go test -race ./common/pcapx/pcaputil ./common/bin-parser/parser/stream_parser -run 'Test(Live|ProtocolAPI|BinParser|HTTP2|MySQL)' -count=1 -timeout=10m
# 有界随机消息 + 分片输入；完整连接关闭后必须释放共享容量。
go test ./common/pcapx/pcaputil -run '^$' -fuzz '^FuzzLiveProtocolSessions$' -fuzztime=45s -parallel=4 -timeout=3m
go test ./common/pcapx/pcaputil -run '^$' -bench '^BenchmarkLiveProtocolSessions$' -benchmem -benchtime=200ms -count=3
```

新增规则还逐事件通过 `ParseBinary` 与实时 `Fields` 的等价断言，截断数据必须失败。
覆盖错误序号、认证/命令方向、缺失握手上下文、非法/重复伪头、孤立 CONTINUATION、
HPACK 缺失字典、缩表、窗口溢出、流复用、EOF 边界、资源预算、查看器深拷贝与并发访问。

规范依据：[RFC 9113](https://www.rfc-editor.org/rfc/rfc9113.html)、
[RFC 7541](https://www.rfc-editor.org/rfc/rfc7541.html)、
[MySQL Client/Server Protocol](https://dev.mysql.com/doc/dev/mysql-server/latest/PAGE_PROTOCOL.html)、
[MariaDB Client/Server Protocol](https://mariadb.com/docs/server/reference/clientserver-protocol)。
