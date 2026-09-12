# Capture, inspect and replay

Build from the repository root (Windows: add `.exe` to output names):

```sh
go build -o pcap-inspect ./common/pcapx/cmd/pcap-inspect
go build -o pcap-load ./common/pcapx/cmd/pcap-load
```

Windows live capture needs [Npcap](https://npcap.com/). macOS uses the existing
libpcap backend and requires permission to access capture devices. Pure-Go file
replay does not open a native capture handle; file BPF does require libpcap/Npcap.

```sh
pcap-inspect -list
pcap-inspect -interface 1 -duration 30s -workers 1 -bpf "tcp or udp" -follow -write session.pcap -report session.json
pcap-inspect -read session.pcap -workers 1 -protocol http -rows 20
pcap-inspect -read session.pcap -workers 1 -protocol http -detail 12345
```

Pick the interface index from `-list`, and the detail ID from the replay table.
History defaults to the last 4,096 messages within 32 MiB of raw data; an older
detail can be evicted. Increase `-history`/`-memory-mib` or narrow replay with BPF.
Protocol/flow filters affect the display only. The inspector retains raw messages
and metadata; details re-create owned structured fields. TLS ciphertext remains
opaque; this tool does not decrypt TLS.

The default is full structured parsing. `-deferred` explicitly postpones field
decoding until a detail is requested. Final JSON serialization is outside capture
timing. `-quiet -history 0` is useful for measuring the analysis path without a
viewer. `-follow` emits at most `-rows` rows per `-interval`, rather than printing
every packet. Ctrl-C or `-duration` stops capture and drains accepted worker jobs.
Only worker counts 1, 2 and 4 are supported by this CLI; GOMAXPROCS follows that
setting. UDP analysis remains on the reader.

Use separate terminals for a reproducible local test. On Windows:

```powershell
.\pcap-inspect.exe -interface '\Device\NPF_Loopback' -bpf 'tcp portrange 18080-18082' -duration 24s -workers 1 -write local.pcap -report local.json
```

```powershell
.\pcap-load.exe -duration 20s -delay 2s -rate 500 -body 4096 -flows 24 -reconnect 128
```

On macOS select the listed loopback device (usually `lo0`). The generator binds
only `127.0.0.1` and starts its own HTTP server, HTTPS server with a trusted local
test certificate, and a minimal MQTT 3.1.1 receiver. It does not contact external
hosts. `-port` changes all three consecutive ports; adjust the BPF accordingly.
Its rate counts application bodies (HTTP request plus response, MQTT publish),
not Ethernet/IP/TCP/TLS bytes. `-reconnect` controls connection churn. Receiver
counts verify MQTT payload delivery. This is constructed localhost traffic,
not an Internet traffic distribution or a two-host NIC line-rate test.

Check the report rather than just the displayed Mbps:

- `Analysis`: decoded messages, unknown/context-required/incomplete/malformed
  events, buffered/unclassified bytes. Only successful full fields count toward
  `DecodedBytes` and `DecodedMbps`.
- `Reassembly`: read packet/byte counts, sequence gaps, resource limits, invalid
  segments, native `Devices[].Dropped` and `InterfaceDropped`. Native statistics
  have backend-specific meanings; unavailable is different from zero drops.
- `CPUSeconds`: process user + system time. `CPUCores` divides by total elapsed
  capture time, including idle start/end time. One-second live rate windows better
  show active throughput; neither rate is a latency percentile.
- History eviction or omitted console rows means bounded viewing, not skipped
  parsing. A nonzero error is retained even when later messages decode successfully.

`-capture-buffer-mib` requests a bounded native buffer (default 32, maximum 256).
Increasing it may absorb bursts, but cannot guarantee losslessness. Recording
writes synchronously and can slow capture on busy storage. Every output file must
be new; existing files are never silently truncated. A classic pcap recording
cannot mix link types.

For hotspot analysis, add `-cpu-profile capture.cpu` and inspect with
`go tool pprof pcap-inspect.exe capture.cpu`. Profiled runs have additional CPU
cost and should be reported separately from unprofiled throughput tests.

Go/Yak options, protocol coverage and ownership contracts are documented in
[BIN_PARSER.md](../pcaputil/BIN_PARSER.md).
