# pcapx + bin-parser：抓包、初步解析和回放

新脚本从 [pcapx 协议解析 API](PROTOCOL_PARSER.md) 和 [shark.yak](../examples/shark.yak)
开始。本文保留底层集成与旧 API 的兼容参考；新代码使用 `pcap_onProtocolMessage` 和 `Fields`。

这条链路提供类似 Wireshark 的基本工作流：选网卡 → 抓取原包 → TCP 重组 → 识别
已支持协议 → 按完整消息解析字段 → 查看/筛选会话 → 保存后回放。它是消息分析工具，
当前没有 Wireshark 的完整协议覆盖、逐包 GUI、TLS 解密或专家诊断功能。

## 五分钟开始使用

从仓库根目录构建；Windows 将输出名改为 `pcap-inspect.exe`：

```sh
go build -o pcap-inspect ./common/pcapx/cmd/pcap-inspect
./pcap-inspect -list
./pcap-inspect -interface 1 -bpf "tcp or udp" -duration 30s -workers 1 -follow -write session.pcap -report session.json
./pcap-inspect -read session.pcap -protocol http -rows 20
./pcap-inspect -read session.pcap -flow 1 -detail 123
```

把 `1` 换成 `-list` 显示的网卡序号；`-flow` / `-detail` 使用本次回放表中的 ID。
Windows 执行形式为 `.\pcap-inspect.exe ...`，实时捕获需要 Npcap；macOS 使用
libpcap 并需要抓包设备权限。普通 pcap/pcapng 文件回放不需要原生抓包驱动。

默认对每条已准入完整消息解析字段。查看表中的协议、方向、长度和状态，再用 detail
查看原始字节、结构化字段与 metadata。`-protocol` / `-flow` 只过滤显示，BPF 过滤
捕获输入。默认历史为最后 4,096 条 / 32 MiB 原始消息，历史淘汰不代表解析丢包。
保存文件用于后续重新回放；输出文件必须是新文件。Ctrl-C 或到时停止后会排空已接收任务。

始终检查报告中的 `Analysis` 和 `Reassembly`：`decoded` 是已完成字段；
`unrecognized` / `context-required` 表示协议或阶段暂不支持；`incomplete` / `malformed`
表示缺少边界或报文非法；`limited` 表示触及资源上限。原生 Dropped / InterfaceDropped
与重组缺口同样重要，不能只看 Mbps。TLS 密文不会变成解密后的 HTTP 内容。

## Yak 实时抓包

网卡名称取自 `-list`，Windows 例如 `\\Device\\NPF_...`，macOS 例如 `en0` / `lo0`。
下面通过 context 限制抓包时长，结束后查看，避免逐消息打印占用分析 CPU：

```javascript
view = pcapx.NewBinParserInspector(4096, 32 * 1024 * 1024)~
err = pcapx.StartSniff("en0",
    pcapx.pcap_context(context.Seconds(30)),
    pcapx.pcap_bpfFilter("tcp or udp"),
    pcapx.pcap_tcpReassemblyWorkers(1),
    pcapx.pcap_binParser(func(event) { view.OnEvent(event) }),
)
dump(err)
rows = view.Rows("", 0)
dump(rows)
if len(rows) > 0 {
    detail = view.Details(rows[0].ID)~
    dump(detail)
}
```

下面保留 Go/Yak API、消息所有权、状态码及精确协议范围；命令行压测用法见
[CLI 指南](../cmd/README.md)。后续协议实现集中在 [协议 TODO](../../bin-parser/PROTOCOL_TODO.md)。

`WithBinParser` connects pcapx capture, ordered TCP reassembly, bounded protocol
detection, exact-message framing and complete structured results. No JSON
serialization is involved. `ReplayPcapFile` reads pcap and pcapng without a native
capture driver. `Sniff` / `Start` use the existing live libpcap/Npcap backend.

## Run the viewer

```shell
go build -o pcap-inspect ./common/pcapx/cmd/pcap-inspect
pcap-inspect -read capture.pcapng -protocol http
pcap-inspect -read capture.pcapng -flow 1 -detail 2
pcap-inspect -read capture.pcapng -full -workers 1
pcap-inspect -list
pcap-inspect -interface eth0 -duration 10s -workers 1
pcap-inspect -interface eth0 -bpf "tcp or udp" -duration 30s -follow -write capture.pcap -report capture.json
```

On Windows use `pcap-inspect.exe` and the pcapx/Npcap device name. The live backend
needs its native driver; offline replay does not. `-workers` accepts 1, 2 or 4.
Ctrl-C stops capture and drains accepted jobs before displaying results.

The CLI defaults to **full structured decoding** of every admitted message.
`-deferred` explicitly selects capture-first operation, with `-detail ID`
decoding outside capture. These are different workloads; deferred mode reports
zero decoded Mbps. JSON details and the final report are encoded after timing.
Periodic rates print once per second; `-follow` displays bounded batches of new
rows. `-quiet` disables periodic output. Terminal formatting consumes CPU, so do
not use per-message printing when measuring core bandwidth.

`-history` and `-memory-mib` bound retained history; eviction is printed. `-protocol`
and `-flow` are display filters, not capture/BPF filters. Message IDs are specific
to one capture; across worker counts/concurrent flows their ordering can differ.
`-history 0` disables history (and therefore follow/details). `RowsAfter` uses an
arrival cursor so out-of-order concurrent message IDs do not hide new rows.
Omitted rows are history/display limits, not dropped analysis.

`-write` records original packets before analysis without enabling public packet
callbacks. It creates a new nanosecond classic pcap and refuses to overwrite an
existing file. `-report` similarly creates a new final JSON report; inspect both
analysis and native drop counters. `DecodedBytes` excludes failed decodes.
`CapturedMbps` includes link/IP/TCP headers; `DeliveredMbps` counts bytes passed
to analysis. Overall rates and CPU include idle capture time and final drain.
Very short replay can fall below clock resolution (`RateAvailable=false`).

Live capture defaults to a 32 MiB native buffer, configurable with
`-capture-buffer-mib 0..256` (0 selects the backend default). Buffers absorb bursts;
they cannot fix sustained overload or slow storage. `-cpu-profile FILE` records
a Go CPU profile for diagnosis and changes the measured workload. See
[the CLI guide](../cmd/README.md) for a localhost HTTP/TLS/MQTT load generator.

## Yak

```javascript
view = pcapx.NewBinParserInspector(4096, 32 * 1024 * 1024)~
pcapx.ReplayPcapFile("capture.pcapng",
    pcapx.pcap_tcpReassemblyWorkers(1),
    pcapx.pcap_binParser(func(event) { view.OnEvent(event) }),
    pcapx.pcap_binParserStats(func(stats) { dump(stats) }),
)~
dump(view.Rows("", 0))
// Choose an ID from Rows; the object includes owned Raw, Structured and metadata.
// detail = view.Details(2)~
// dump(detail)
```

Use `pcapx.StartSniff(device, ...same options...)` for live capture. The default
of `pcap_binParser` is **full structured parsing**, as in the CLI. Use
`pcapx.pcap_captureBufferSize(32 * 1024 * 1024)` to configure the native buffer.
`pcapx.pcap_captureWriter(writer)` records packets; the caller must flush/close
the writer after capture returns. These options require an exclusive handle.
Do not print or JSON-marshal every message in a high-rate
capture callback. Yak callbacks from multiple workers may execute concurrently;
the inspector handles synchronization internally.

## Go

```go
view, err := pcaputil.NewBinParserInspector(4096, 32<<20)
if err != nil { return err }
err = pcaputil.ReplayPcapFile("capture.pcapng",
    pcaputil.WithTCPReassemblyWorkers(1),
    pcaputil.WithBinParser(view.OnEvent), // complete fields, no JSON
    pcaputil.WithBinParserStats(func(stats pcaputil.BinParserStats) {
        // Record counts and uncovered bytes, separately from reassembly loss.
    }),
)
```

`WithBinParserConfig` sets message/buffer/probe limits and explicit additional
`BinParserBinding` entries. Each binding supplies a port hint, wire probe, exact
framer, rule and entry. Probes run on a bounded initial prefix; bindings are
indexed by port and cached per flow. Global (port 0) bindings still require a
linear candidate scan. Adding 600 distinct-port bindings does not add 600 probes
to every message. This does not guarantee constant cost for 600 ambiguous
protocols sharing a port or for high rates of new, unrecognized sessions.

A binding must apply to both directions and the entire admitted session phase.
Do not guess MySQL capabilities, PostgreSQL phase, TDS version or MQTT version
from a port. Use distinct, explicit profiles where these assumptions are known;
the adapter does not automatically promote all 82 native entries into detectors.

## Automatic coverage

| Protocol | Framing and output |
| --- | --- |
| HTTP/1.0, HTTP/1.1 | Observed request methods, lengths, chunks, trailers, HEAD, interim responses, CONNECT/101 upgrade; complete existing `HTTPExact` fields |
| TLS | Exact records; existing record/ClientHello rule. Ciphertext remains opaque; no decryption or new handshake semantics |
| MQTT | CONNECT establishes 3.1 or 3.1.1 profile for both directions; complete native fields; 5.0 / missing CONNECT needs context |
| DNS | UDP datagrams or length-prefixed TCP, port hint plus header checks; existing DNS rule |
| Kerberos | Port 88 plus DER application tag; UDP messages or TCP records; existing native fields |
| Memcached | Existing stats request/response and binary GET request profiles |
| Cassandra | Existing CQL v4 OPTIONS / SUPPORTED / STARTUP profiles; later phases require further admission |

This is a useful capture/analysis foundation, **not Wireshark-equivalent protocol
coverage**. Other TCP/UDP traffic yields bounded samples and counters. L2-only
protocols, IP fragment reassembly, TLS decryption, general protocol negotiation,
and packet dissection across hundreds of protocols are outside this adapter.
Existing pcapx packet callbacks are available when individual link-layer packets
are needed; enabling them uses the general packet path.

## Results, limits and errors

Events own `Raw` and `Structured`; retaining an event never retains pooled flow
pointers. `Direction` refers to first-observed endpoint order, not a client/server
role inferred from a port. `Offset` counts delivered ordered bytes within that
direction. TCP retransmissions are not counted as new application bytes. UDP
events use FlowID 0 and have no persistent session identity.

`Structured.metadata` preserves the rule's original metadata exactly. For
example, a native rule's `TCP Reassembly Performed: false` describes that rule's
own responsibilities; pcapx reassembly is reported by the event and capture
statistics, rather than rewriting parser metadata. Gap events have no `Raw`
message; their `Length` is the number of buffered bytes behind the gap.

- `decoded`: complete structured fields and metadata.
- `deferred`: complete, framed raw message, available through `Decode()` / `Details()`.
- `unrecognized`: bounded initial sample; following bytes are counted without repeated VM trials.
- `context-required`: recognized family but unsupported phase/missing negotiation.
- `incomplete`: the exact boundary was not observed before FIN/timeout/EOF/cancellation.
- `malformed`: invalid framing or full decode failed; affinity is invalidated for that direction.
- `limited`: configured message/capture buffering limit was reached.

Default analysis limits: 1 MiB/message, 32 MiB total retained partial-message
capacity, 64 initial probe bytes. Unknown samples are capped at the probe size.
HTTP headers/trailers are limited to 32 KiB and outstanding requests to 128.
Close-delimited HTTP bodies finish only on a clean TCP flow FIN, not file EOF,
RST or timeout. CONNECT/101 re-enters bounded detection at the confirmed boundary.
Capture EOF with TCP gaps never invents missing bytes.

The inspector drops eager field trees and keeps raw messages/metadata within its
history limits; requesting details re-runs decoding. Consumer-owned histories
outside the inspector remain the consumer's memory responsibility. Callbacks
backpressure the worker; there is no hidden unbounded analysis queue. Callback
panics are counted and returned as capture errors, including with one worker.

`BinParserStats` describes **analysis**, while `TCPReassemblyStats` describes
reassembly/queues/native drops. Check both and the returned error. Reassembly
queue accounting is only available with multiple workers in the merged backend;
single-worker analysis, capture byte/packet counts, native drops and diagnostic
counters are available. Gaps return
an error with every worker count and produce an incomplete event for that flow.
UDP analysis currently runs on the
reader path, so adding TCP workers does not scale a DNS-heavy workload.

`ReplayPcapFile` preserves file timestamp precision, supports mixed-link pcapng,
caps pcapng blocks at 16 MiB and interfaces at 1024/section, and validates classic
pcap lengths before allocation. It rejects BPF/native-handle/cache/device options;
use `OpenPcapFile` for native BPF. It never silently ignores a filter. A blocking
custom `ReplayPcap(io.Reader)` needs its own reader cancellation.

`WithCaptureWriter(io.Writer)` / `WithCaptureBufferSize(bytes)` expose the same
recording and buffering controls to Go. The buffer option applies only to live
devices. Recording supports one link type per file; mixed-link pcapng/device
input returns an error on a link change, rather than writing a corrupt pcap.
Writes provide backpressure and errors propagate to the caller.

The private decoder reuses Ethernet, loopback/raw IP, TCP and UDP layers. Public
packet callbacks still receive owned packet bytes. Fragments, extension headers
and invalid/ambiguous network layers retain the general decoder's behavior.
The native buffer API uses libpcap activation on Windows and macOS; no new CGO
or OS-specific native code is introduced.

Passive capture can reorder RST before preceding data or FIN. One bounded reset
sequence per direction waits for the observed contiguous stream to reach it
exactly, without advancing over missing bytes. A mismatch or a gap that remains
at disposal is still an error. An in-sequence reset after a half-close terminates
the whole flow, allowing a new SYN to reuse the four-tuple.

DNS, HTTP and TLS (including complete ClientHello) structured adapters are gated by complete embedded
rule fingerprints (including the relevant imported TLS rule) and the default
parser registration. Rule changes/custom parsers fall back to the interpreter.
Field names, concrete Go types and metadata are compared with the original
Node projection. Failed validation retains the original diagnostic path.

The DNS adapter preserves the existing label lists, unresolved RR pointers,
DNSA/DNSPTR children, raw RData and omitted empty sections. It does not add
DNS name resolution or additional RDATA types.

The ClientHello adapter preserves cipher suites, compression methods, SNI and
unknown extension bytes with the same concrete types. Ambiguous lengths retain
the interpreter path. Current capture/replay results and capacity limits are
recorded in [PERFORMANCE.md](../../bin-parser/PERFORMANCE.md).
