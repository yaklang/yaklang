# WCF net.tcp MC-NMF framing：2026-11-03

本批实现 PA-012 的首个有限合同：识别 WCF `net.tcp` 使用的 MC-NMF v1.0
record framing，并保留有界消息边界、方向、流和原始字节。Envelope 内容保持
opaque；不执行、不反序列化捕获中的 SOAP、MTOM、.NET Binary XML 或应用对象。

## 规范、样本和入口

- 规范使用 Microsoft [MC-NMF] 2.2 Record Types、2.2.2 小端 EncodedSize、
  2.2.3 Property Records、2.2.4 Envelope Records、2.2.5 Fault Records 和
  3.1.1.2 Protocol Grammar。
- record 布局由 Wireshark `packet-mc-nmf.c` 交叉核对。Wireshark 的 9389 默认
  端口提示不作为识别依据。
- 原样本为仓库外企业流量
  `redline-infostealer-2023-07/capture.pcap`，SHA-256
  `7349b0cab7fbc605f31dd7db95748a23b16b3fc2f5d370a7afb3e99cd9f17c8e`。
  只读取该训练样本，不访问其中基础设施，也不执行载荷。
- 新 native TCP profile 为 `Protocol == "wcf"`、
  `Profile == "wcf-mc-nmf"`。

## 有限合同

支持 MC-NMF v1.0 的 13 种 record：Version、Mode、Via、Known/Extensible
Encoding、Unsized/Sized Envelope、End、Fault、Upgrade Request/Response、
Preamble Ack/End。EncodedSize 最多 5 字节，最终值不得超过 `uint32`；实际内存、
record 和集合数量继续受 `ParserBudget` 限制。

完整 Preamble 只以以下证据接纳：Version 1.0 + 合法 Mode + `net.tcp://` Via。
中途捕获只以以下证据接纳：完整且边界自洽的 Fault framing URI，或 Sized
Envelope 的完整外层长度加上 SOAP/Binary XML root 候选。单个 0x0B Preamble Ack
不是协议身份，会短暂保留，由后续同流 Envelope 解析；不能借端口或弱长度前缀
直接宣称 WCF。

每个事件恰好包含一条完整 MC-NMF record。Envelope 事件保留 envelope 总长、
opaque body 长度和 SHA-256；字段范围只指向本 record。方向沿用首次观察顺序，
不根据端口猜测客户端。中途捕获不要求握手，也不虚构 Version、Mode、Via 或
Encoding。失败事件保留 `wcf`、方向、上下文和原始前缀，不降级为普通 TCP。

## 明确限制

- `Singleton-Sized` 的 Mode Record 后是无 record 边界的裸 Octets。缺少应用
  长度时返回 `context-required`，不猜测消息边界。
- SOAP 1.1/1.2、MTOM、Binary 和 Binary-with-dictionary 正文均保持 opaque。
- `application/negotiate` 后随的 NNS/SPNEGO 以及 TLS 内容不在此 profile；
  Upgrade Response 后恢复普通承载检测，升级成功不等同于认证或解密成功。
- 不推断 WCF operation、消息 schema、业务结果、端点身份或对象内容。

## 验证

- 合成覆盖 Preamble、双向 midstream、Sized/Unsized、逐字节/逐 record 分片、
  coalesced record、截断、零/未终止/超限 EncodedSize、未知 record、错误状态
  转换和误导性长度前缀。
- `redline-01` 实际回放得到同一 flow 上 12 条记录：
  `3/2/33/2/1/1/218/159/166/6899/541960/120` 字节，方向和二进制 XML opaque
  正文均可定位。
- `go test ./common/pcapx/pcaputil -count=1` 通过（258.506s）。
- `go test ./common/bin-parser -count=1` 通过（80.201s）。
- `go test -race ./common/pcapx/pcaputil -run '^TestWCF'` 通过；macOS linker
  输出既有 `LC_DYSYMTAB` warning，不影响测试结果。
- `FuzzWCFRecord` 运行 5 秒，共 23,301 次执行、31 个 interesting corpus，未崩溃。
- `go vet ./common/pcapx/pcaputil ./common/bin-parser` 和 `git diff --check` 通过。
