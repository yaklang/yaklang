# 用 pcapx 编写协议抓包工具

协议解析是 pcapx 的内置能力。脚本只需注册结果回调，无需创建解析引擎、加载规则或
设置启用开关。没有消息或统计订阅者时，省去规则准备与协议解码，原有原包/流回调照常工作。

## 最小脚本

```javascript
ctx, stop = pcapx.CaptureContext(30)~
defer stop()
pcapx.StartSniff("en0",
    pcapx.pcap_context(ctx),
    pcapx.pcap_onProtocolMessage(func(message) {
        println(message.Protocol, message.Source, message.Destination)
        dump(message.Fields)
    }),
)~
```

`CaptureContext(30)` 支持 30 秒停止及 Ctrl-C；`0` 表示不设期限。`stop()` 也可用于
消息数达到目标时主动停止。结束前会排空已接收任务，再回调最终统计、关闭输出文件。

回放同一组选项，只需将最后的调用换成 `pcapx.OpenPcapFile("session.pcap", options...)`。
pcap/pcapng 默认使用无需原生驱动的文件读取器；BPF 或原生句柄回调才使用 libpcap/Npcap。
实时抓包需要相应驱动和设备权限。`pcapx.ListDevices()~` 返回可传给 StartSniff 的设备名称。

## 可运行的 shark.yak

[shark.yak](../examples/shark.yak) 完全使用 pcapx 和 CLI API：

```sh
yak common/pcapx/examples/shark.yak -D
yak common/pcapx/examples/shark.yak -i 1 --duration 30 -f "tcp or udp" -w session.pcap
yak common/pcapx/examples/shark.yak -r session.pcap -Y http -c 10
yak common/pcapx/examples/shark.yak -r session.pcap -Y dns -V
```

Windows 将 `yak` 换成对应的 `yak.exe` 路径。`-i` 支持网卡编号或名称。
`-f` 是输入 BPF；`-Y` 是小写协议名的显示过滤；`-c` 是显示消息数上限；`-V` 打印完整字段。
`-w` 保存原始包，文件必须尚不存在。过滤显示不会过滤保存内容。
这里的行是重组后的协议消息；一个 TCP 包可能包含多条消息，一条消息也可能跨越多个包。
`-Y` 当前不是 Wireshark 的完整字段过滤表达式语言。

## 消息与订阅

| 字段 | 含义 |
|---|---|
| `ID` / `FlowID` | 本次捕获的消息 ID / 会话 ID |
| `Timestamp` / `Transport` | 捕获时间 / TCP 或 UDP |
| `Protocol` / `Summary` | 协议名 / 消息摘要 |
| `Source` / `Destination` | 包含端口的端点 |
| `Direction` / `Offset` | 首次观察方向及该方向已交付字节的偏移，不推测客户端角色 |
| `Status` / `Error` | 解析状态 / 具体错误 |
| `Length` / `Raw` | 消息长度 / 自有原始字节；错误样本的 Raw 可能受上限裁剪 |
| `Fields` | 完整结构化字段，正常情况下 `Status == "decoded"` |
| `Metadata` | 与字段分开的解析附加信息 |

回调返回后可以保留消息，原始字节和字段不借用重组池。每次捕获内的新协议回调默认
串行执行，包括多个 TCP worker 与 UDP 投递，脚本可直接维护自己的计数或列表。
同一 TCP 会话内保持消息顺序；跨会话不保证 ID、时间戳和完成顺序相同。
回调较慢会形成背压；持续高流量时，应限制输出或将消息交给有界历史查看器。

```javascript
pcapx.pcap_onProtocolMessage(func(message) { /* 接收消息 */ })
pcapx.pcap_onProtocolStats(func(stats) { println(stats.Messages, stats.Decoded, stats.Unknown) })
pcapx.pcap_outputFile("session.pcap")
```

消息订阅以最后一次配置为准；传 `nil` 取消消息订阅。仅订阅统计也可以运行解析。
统计在捕获结束、已接收任务排空后提供。原生丢包及重组诊断沿用 `pcap_onTCPReassemblyStats`。
输出文件由 pcapx 创建和关闭；Go 调用者已有 writer 时仍可使用 `WithCaptureWriter`，自行负责关闭。

默认完整解码。需要有界历史时：

```javascript
history = pcapx.NewProtocolInspector()~ // 默认 4096 条、32 MiB 原始字节
pcapx.OpenPcapFile("session.pcap", pcapx.pcap_onProtocolMessage(history.OnEvent))~
rows = history.Rows("http", 0)
if len(rows) > 0 {
    message = history.Details(rows[0].ID)~
    dump(message.Fields)
}
```

列表只有摘要；详情返回独立字段。历史淘汰可以通过 `Evicted()` 观察，不等于抓包丢包。
显式设置 `pcap_protocolDeferred(true)` 才延迟解码；此时使用 `message.GetFields()~`
或历史详情取得字段，`Fields` 在消息回调时为空。

## Go API 与兼容性

对应入口为 `WithOnProtocolMessage(func(*ProtocolEvent))`、`WithOnProtocolStats`、
`WithOutputFile`、`NewProtocolInspector`、`CaptureContext`。
旧消息类型、订阅和查看器名称仍保留兼容；旧 `WithBinParser` 保持原来的并行回调语义。
`Structured` / `Decode()` 保留旧的 fields/metadata 包装；新脚本直接使用 `Fields` 和
`Metadata`。不会把字段转成 JSON 后再交给回调。

协议订阅使用有界 TCP 流式重组，与禁用重组、抓包缓存或旧的全流 HTTP/TLS helper
不能同时使用；只有原包/旧 helper 的脚本无需修改。
目前实时准入覆盖 HTTP/1.x、TLS、MQTT 3.1/3.1.1、DNS、Kerberos，以及 Memcached /
Cassandra 的限定阶段。未知、缺上下文、不完整、非法和资源受限样本都保留明确状态；
TLS 密文保持不透明。扩展范围见 [协议 TODO](../../bin-parser/PROTOCOL_TODO.md)。
