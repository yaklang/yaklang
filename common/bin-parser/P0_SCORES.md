# P0 协议交付打分

对照 [PROTOCOL_DELIVERY.md](PROTOCOL_DELIVERY.md)。机器可读记录在 `protocol_scores.go`，由 `TestP0ScorecardsCovered` 校验：每个 P0 名称都有计分卡；`Status: done` 必须是 A 或 B。

维度（满分 100）：Schema 25 / 真实流量 25 / 测试 20 / 分支覆盖 20 / 栈集成 10。硬门槛 G1–G8 全过才计分。

别名（CIFS、MSRPC、TNS、AS-REP / TGS、NTLM v1/v2）与主规则共用同一张卡。

2026-09-07：LDAP、MySQL、PostgreSQL、SMB3 按原规则及已验证能力重新核算，分别仍为 80/B、75/B、81/B、75/B；新字段入口不自动增加 Schema 或分支分。新增消息范围的验收证据独立执行，见 [P0 范围与评分依据](P0_SCOPE.md)。四项能力目录继续标记 `partial`。

历史 `WPAD proxy` 计分卡仅评价 `HTTP GET /wpad.dat` 的 HTTP 封装，不评价 WPAD 发现、代理使用或配置正文语义。专用 WPAD 入口必须保留 `partial` 状态；P0 范围回归同时验证旧 HTTP／Ethernet 路径、专用字段／原始字节，以及未观察发现流程、未证明代理使用、未解析正文的元数据，不能以状态改名绕过这一边界。

| 协议 | 等级 | 总分 | Schema | 流量 | 测试 | 分支 | 栈 | 样本 | 规则 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Ethernet II | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `ethernet.yaml` |
| IEEE 802.1Q | B | 78 | 20 | 8 | 20 | 20 | 10 | L3 | `ieee_802_1q.yaml` |
| ARP | B | 85 | 20 | 25 | 16 | 14 | 10 | L1 | `address_resolution_protocol.yaml` |
| IPv4 | B | 85 | 20 | 25 | 16 | 14 | 10 | L1 | `internet_protocol.yaml` |
| IPv6 | B | 85 | 20 | 25 | 16 | 14 | 10 | L1 | `internet_protocol_version_6.yaml` |
| ICMP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `internet_control_message_protocol.yaml` |
| ICMPv6 | B | 85 | 20 | 25 | 16 | 14 | 10 | L1 | `internet_control_message_protocol_v6.yaml` |
| TCP | A | 100 | 25 | 25 | 20 | 20 | 10 | L1 | `transmission_control_protocol.yaml` |
| UDP | B | 85 | 20 | 25 | 16 | 14 | 10 | L1 | `user_datagram_protocol.yaml` |
| QUIC | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/quic.yaml` |
| NetBIOS | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/nbss.yaml` |
| NBT NS | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/nbns.yaml` |
| NBT SS | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/nbss.yaml` |
| DNS | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `application-layer/dns.yaml` |
| NBNS | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/nbns.yaml` |
| DHCP | B | 78 | 20 | 8 | 20 | 20 | 10 | L3 | `application-layer/dhcp.yaml` |
| HTTP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `application-layer/http.yaml` |
| HTTP/2 | A | 90 | 20 | 20 | 20 | 20 | 10 | L2 | `application-layer/http2.yaml` |
| WebSocket | A | 95 | 25 | 20 | 20 | 20 | 10 | L2 | `application-layer/websocket.yaml` |
| TLS | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `application-layer/tls.yaml` |
| JA3/JA4 | B | 85 | 20 | 25 | 16 | 14 | 10 | L1 | `application-layer/tls_hello.yaml` |
| SMTP | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/smtp.yaml` |
| FTP | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/ftp.yaml` |
| SMB | B | 85 | 20 | 15 | 20 | 20 | 10 | L2 | `application-layer/smb.yaml` |
| SMB2 | B | 85 | 20 | 15 | 20 | 20 | 10 | L2 | `application-layer/smb2.yaml` |
| SMB3 | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/smb3.yaml` |
| CIFS | B | 85 | 20 | 15 | 20 | 20 | 10 | L2 | alias of SMB |
| LDAP | B | 80 | 20 | 20 | 16 | 14 | 10 | L2 | `application-layer/ldap.yaml` |
| Kerberos | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/kerberos.yaml` |
| NTLM | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/ntlm.yaml` |
| NTLMSSP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/ntlm.yaml` |
| SPNEGO | B | 80 | 15 | 25 | 16 | 14 | 10 | L2 | `application-layer/spnego.yaml` |
| RADIUS | A | 100 | 25 | 25 | 20 | 20 | 10 | L1 | `application-layer/radius.yaml` |
| SOCKS5 | B | 86 | 20 | 20 | 16 | 20 | 10 | L2 | `application-layer/socks5.yaml` |
| SSH | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/ssh.yaml` |
| RDP | B | 86 | 20 | 20 | 16 | 20 | 10 | L1 | `application-layer/msrdp.yaml` |
| PsExec/SMB-svcctl | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| MySQL | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/mysql.yaml` |
| PostgreSQL | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/postgresql.yaml` |
| MSSQL TDS | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/tds.yaml` |
| Oracle TNS | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/tns.yaml` |
| TNS | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | alias of Oracle TNS |
| Redis | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/redis.yaml` |
| MQTT | B | 86 | 20 | 20 | 16 | 20 | 10 | L2 | `application-layer/mqtt.yaml` |
| DCE/RPC | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| MSRPC | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | alias of DCE/RPC |
| SNMP | B | 80 | 20 | 20 | 16 | 14 | 10 | L2 | `application-layer/snmp.yaml` |
| MSRPC EPM | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| SAMR | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| LSARPC | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| NETLOGON | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| DRSUAPI | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| SRVSVC | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| SVCCTL | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| WINREG | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| WMI | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| DCOM | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| Yakit proxy framing | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `application-layer/http.yaml` |
| AJP | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/ajp.yaml` |
| Java serialization | B | 80 | 20 | 20 | 16 | 14 | 10 | L2 | `application-layer/java_ser.yaml` |
| Kerberos PAC | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/kerberos.yaml` |
| AS-REP / TGS | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | alias of Kerberos |
| NTLM v1/v2 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of NTLMSSP |
| NetNTLMv2 | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/ntlm.yaml` |
| LLMNR response | B | 75 | 20 | 15 | 16 | 14 | 10 | L2 | `application-layer/nbns.yaml` |
| NBT-NS response | B | 81 | 20 | 15 | 16 | 20 | 10 | L2 | `application-layer/nbns.yaml` |
| WPAD proxy | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `application-layer/http.yaml` |

67 个 P0：A 10 / B 57，最低 75（B）。IEEE 802.1Q / DHCP 为 L3 样本，流量维按标准记 8 分。802.1Q 的 Payload switch（IP / IPv6 / ARP / EAPOL / default）由 `TestVLANPayloadTypeArms` + ARP 内层测试覆盖。证据见 `protocol_scores.go` 的 `Evidence`；G6 失败路径在 `p0_fail_paths_test.go`。

SPNEGO 与 P1 GSS-API 的评分共用 `spnego.yaml` 初始 token 子集的证据与得分（80/B）：Schema 15，不将该规则的 `Optional Fields`（reqFlags/mechToken/mechListMIC 的 TLV 结构）、非 init `Octets`（含 NegTokenResp）算作已解析，且不支持长格式 BER。`TestP1WiresharkAndRFCSamples/spnego/ntlm` 和 `/spnego/krb5` 分别在 Ethernet→IP→TCP/445→SMB2→Session Setup→SPNEGOInit 路径断言各自的 `MechOID`，因此流量维按既有整帧具名字段标准记 25；不是仅有 Type/Length 或孤立 payload。边界、额外 OID 与可选字段有回归，但尚未覆盖每条显式错误分支，测试/分支维保守记 16/14。独立的 corpus `gssapi.yaml/GSSAPIHTTP` 合同已解析 token 字段，缺少 NegotiationToken 的原始样本仍是负例，完整 token 由 `gen-gssapi-valid` 验证；tokenless 401 challenge 的 outer-only 响应不计为已解析 token 字段。评分不计入新原生规则的额外字段，也不表示所有 GSS-API 机制或协商结果已经实现和验证。

## 本轮扩展的非 P0

| 协议 | 原状态 | 现状态 | 样本 | 说明 |
| --- | --- | --- | --- | --- |
| EAPOL | P1 partial | P1 done | L1 Wireshark `wpa-Induction.pcap` frame 87 | Start/Logoff/EAP-Packet/Key；`TestEAPOL*` + 截断失败；EtherType 0x888e |
