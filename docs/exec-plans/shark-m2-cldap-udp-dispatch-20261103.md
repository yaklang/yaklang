# CLDAP UDP/389 分发与消息边界：2026-11-03

本批关闭 PA-014 的首个有限合同：把 UDP/389 的 CLDAP 数据报接入 native
datagram registry，复用已有 `application-layer/cldap.yaml` 解析 SearchRequest，
并对暂未支持的响应操作保留明确边界。不会把 TCP LDAP 的会话状态、认证或结果
投影复用于 UDP。

## 问题与真实来源

原证据来自 `host-user-identification` 的第 3 个 PCAP：同一运行中 UDP/389 有
34 个数据报全部为 `unrecognized`，TCP/389 的 LDAP 则已有独立结果。

真实文件：

- `Wireshark-tutorial-identifying-hosts-and-users-3-of-5.pcap`
- SHA-256 `7ce9a37aa6d8f0121d23f9370d11f17d4de7111cf19629d92ff630abd74064af`
- 12,329 包，训练流量；只回放解析，不访问捕获中的端点。

## 实现

- UDP/389 加入 `nativeDatagrams`；只有端口提示仍要求 BER `SEQUENCE`
  后才进入 CLDAP profile，不能仅凭端口创造协议记录。
- 一个 UDP 数据报可以包含一个或多个有序 LDAPMessage。实现逐条校验 BER 长度、
  Message ID、application operation 和 datagram 剩余边界。
- SearchRequest 复用现有 CLDAP YAML，并暴露 Message ID、base、scope、alias、
  size/time limits、types-only、filter 和 requested attributes。
- SearchResultEntry/SearchResultDone/SearchReference 仍形成 `cldap` 事件，
  保留 Message ID、operation、result code、消息数量和不透明正文，状态为
  `context-required`。不借用 TCP LDAP 的结果关联或认证状态。
- 支持 `WithProtocolDecodeAs("udp", port, "cldap")`；显式选择仍执行同一 BER
  和消息边界校验。

## 验证

- 原样本回放：34 个 UDP/389 数据报全部成为 `cldap`，其中 17 个
  SearchRequest 为 decoded，17 个响应数据报为 context-required，
  UDP/389 的 unrecognized 为 0；同一捕获保留 60 个 TCP LDAP 事件。
- 合成覆盖 SearchRequest 字段/摘要、组合 SearchResultEntry+Done、
  非标准端口 DecodeAs、响应操作矩阵、截断 BER 和 fuzz。
- `go test ./common/pcapx/pcaputil -count=1` 通过（255.692s）。
- `go test ./common/bin-parser -count=1` 通过（80.716s）。
- `go test -race ./common/pcapx/pcaputil -run '^TestCLDAP'` 通过；macOS
  linker 输出既有 `LC_DYSYMTAB` warning，不影响测试结果。
- `FuzzCLDAPDatagram` 运行 5 秒，共 16,658 次执行、16 个 interesting corpus，
  未崩溃。
- `go vet ./common/pcapx/pcaputil ./common/bin-parser` 与 `git diff --check`
  通过。
