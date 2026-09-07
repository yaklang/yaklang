# P1 协议交付打分

对照 [PROTOCOL_DELIVERY.md](PROTOCOL_DELIVERY.md)。机器可读记录在 `p1_scores.go`，由 `TestP1ScorecardsCovered` 校验：每个 P1 名称都有计分卡；`Status: done` 必须是 **B**（总分 ≥ 75）。A 级（≥ 90）留给 Wireshark 级主 PDU。

维度（满分 100）：Schema 25 / 真实流量 25 / 测试 20 / 分支覆盖 20 / 栈集成 10。硬门槛 G1–G8 全过才计分。

别名与主规则共用同一张卡（见 `AliasOf`）。样本来源包括 gopacket 测试帧、RFC 完整 PDU，以及 Ethernet+IP+L4 整帧断言。

G5 要求 SampleClass ∈ {L1, L2, L3}；L4-only handmade PDU 不计分。L3 gopacket serialize 的 Traffic ≤ 8。

`TestP1ScorecardsCovered` 用 YAML/`p1FailCases`/mustChild 扫描卡死 Schema/Tests/Traffic 上限：声称分不得高于 `schemaCeiling` / `testsCeiling` / `trafficCeiling`。G1–G4/G6–G8 由测试从规则文件、失败路径和以太网封装推导，`p1card` 不得写死为 true。

IIOP/GIOP 与 IIOP Locate 的原生 Go 字段树尚不在现有 YAML Schema 审计范围，Schema 暂不计分（0），总分为 75/B；这不是没有字段的声明，也不豁免静态上限。完整的原始捕获及新增 companion 有独立字段、位区间与字节边界测试。IDL 参数、Profile/Context 内容仍不展开，分片与 TCP 重组由调用方提供，ZIOP 尚不支持；评分不代表完整协议实现。

RMI/JRMP 及 JNDI、RMI、JMX 评分别名采用相同的保守口径：原生 Go 字段树不在 YAML Schema 审计范围，Schema 0，总分 75/B，不设置上限豁免。SingleOp Ping 与完整 Call 有 Ethernet+TCP/1099 字段断言；原始捕获逐阶段解析并检查 ObjID、operation、hash、序列化字段及边界。调用方仍负责阶段选择与重组，primitive arguments/custom blocks、Multiplex virtual data 不展开，TC_EXCEPTION 和 externalizable-v1 内容不支持；评分别名不表示已实现 JNDI/JMX 的全部语义。

GSS-API 与 P0 SPNEGO 的评分仅计 `spnego.yaml` 共同的初始 token 子集（80/B）：Schema 15；该规则的 `Optional Fields`（reqFlags/mechToken/mechListMIC 的 TLV 结构）和非 init `Octets`（含 NegTokenResp）未展开，长格式 BER 不支持，不作规范不透明豁免。`TestP1WiresharkAndRFCSamples/spnego/ntlm` 与 `/spnego/krb5` 各自在 Ethernet→IP→TCP/445→SMB2→Session Setup→SPNEGOInit 上断言 `MechOID`，支持流量 25；未覆盖每条显式错误分支，测试/分支为 16/14。独立的 corpus `gssapi.yaml/GSSAPIHTTP` 合同已解析 token 字段，保留缺少 NegotiationToken 的原样本为负例，以 `gen-gssapi-valid` 验证完整 token；tokenless 401 challenge 的 outer-only 响应须标记 `Token Fields Decoded=false`。此卡不计入新原生规则的额外字段，不表示所有 GSS-API 机制或协商结果已经实现和验证。

| 协议 | 等级 | 总分 | Schema | 流量 | 测试 | 分支 | 栈 | 样本 | 规则 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Ethernet 802.2 | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | `llc.yaml` |
| Ethernet 802.3 | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | `llc.yaml` |
| Ethernet SNAP | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | `llc.yaml` |
| IEEE 802.1ad QinQ | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `ieee_802_1ad.yaml` |
| PPP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `ppp.yaml` |
| PPPoE Discovery | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `pppoe.yaml` |
| PPPoE Session | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `pppoe.yaml` |
| LCP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `link_control_protocol.yaml` |
| PAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `password_authentication_protocol.yaml` |
| CHAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `challenge_handshake_authentication_protocol.yaml` |
| EAPOL | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `eapol.yaml` |
| EAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `eapol.yaml` |
| STP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `stp.yaml` |
| RSTP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of STP |
| LLDP | A | 100 | 25 | 25 | 20 | 20 | 10 | L1 | `lldp.yaml` |
| CDP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `cdp.yaml` |
| Loopback | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `loopback.yaml` |
| Linux SLL | B | 85 | 20 | 15 | 20 | 20 | 10 | L2 | `linux_sll.yaml` |
| IEEE 802.11 | A | 90 | 25 | 15 | 20 | 20 | 10 | L2 | `ieee_802_11.yaml` |
| WPA/RSN | A | 90 | 25 | 15 | 20 | 20 | 10 | L2 | `ieee_802_11.yaml` |
| IEEE 802.1X | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of EAPOL |
| ICMPv6 NDP | A | 100 | 25 | 25 | 20 | 20 | 10 | L1 | `internet_control_message_protocol_v6.yaml` |
| IGMP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `igmp.yaml` |
| GRE | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `generic_routing_encapsulation.yaml` |
| IPsec AH | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `ipsec.yaml` |
| IPsec ESP | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `ipsec.yaml` |
| MPLS | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `mpls.yaml` |
| VXLAN | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | `vxlan.yaml` |
| L2TP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `l2tp.yaml` |
| PPTP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/pptp.yaml` |
| OpenVPN | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `openvpn.yaml` |
| WireGuard | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `wireguard.yaml` |
| IKEv1 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `ike.yaml` |
| IKEv2 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `ike.yaml` |
| NAT-T | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of IKEv2 |
| OSPF | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `ospf.yaml` |
| BGP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `bgp.yaml` |
| RIP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `rip.yaml` |
| EIGRP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `eigrp.yaml` |
| VRRP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | `vrrp.yaml` |
| HSRP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `hsrp.yaml` |
| SCTP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `sctp.yaml` |
| NBT DG | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `nbt_dg.yaml` |
| mDNS | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `application-layer/dns.yaml` |
| LLMNR | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of mDNS |
| DoT | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| DoH | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| DHCPv6 | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `dhcpv6.yaml` |
| BOOTP | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `application-layer/bootp.yaml` |
| NTP | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `application-layer/ntp.yaml` |
| PTP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `ptp.yaml` |
| SSDP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| UPnP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| HTTP/3 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/quic.yaml` |
| SSL | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| DTLS | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `dtls.yaml` |
| gRPC | A | 90 | 20 | 20 | 20 | 20 | 10 | L2 | alias of HTTP/2 |
| SOAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| JSON-RPC | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `jsonrpc.yaml` |
| SMTPS | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| POP3 | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `pop3.yaml` |
| IMAP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `imap.yaml` |
| FTP-DATA | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `ftp_data.yaml` |
| TFTP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `tftp.yaml` |
| SFTP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/ssh.yaml` |
| NFS | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `onc_rpc.yaml` |
| RPC | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of NFS |
| Portmap/Rpcbind | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of NFS |
| LDAPS | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| CLDAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/ldap.yaml` |
| GSS-API | B | 80 | 15 | 25 | 16 | 14 | 10 | L2 | `application-layer/spnego.yaml` |
| TACACS+ | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `tacacs.yaml` |
| SOCKS4 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `socks4.yaml` |
| HTTP Proxy CONNECT | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| Telnet | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `telnet.yaml` |
| CREDSSP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| VNC/RFB | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `vnc.yaml` |
| WinRM | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| MariaDB | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/mysql.yaml` |
| MongoDB | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `mongodb.yaml` |
| Memcached | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `memcached.yaml` |
| Elasticsearch | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| AMQP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `amqp.yaml` |
| Kafka | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `kafka.yaml` |
| RabbitMQ | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of AMQP |
| Thrift | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `thrift.yaml` |
| Protobuf | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `protobuf.yaml` |
| JSON-RPC 2.0 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of JSON-RPC |
| ONC RPC | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `onc_rpc.yaml` |
| IIOP/GIOP | B | 75 | 0 | 25 | 20 | 20 | 10 | L2 | `application-layer/iiop.yaml` |
| T3 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/t3.yaml` |
| RMI/JRMP | B | 75 | 0 | 25 | 20 | 20 | 10 | L2 | `rmi.yaml` |
| JNDI | B | 75 | 0 | 25 | 20 | 20 | 10 | L2 | alias of RMI/JRMP |
| BER | A | 90 | 25 | 15 | 20 | 20 | 10 | L2 | `application-layer/ber.yaml` |
| SNMPv3 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/snmp.yaml` |
| Syslog | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `syslog.yaml` |
| IPMI | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `ipmi.yaml` |
| SIP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `sip.yaml` |
| SDP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `sdp.yaml` |
| RTP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `rtp.yaml` |
| RTCP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `rtp.yaml` |
| RTSP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `rtsp.yaml` |
| WebRTC | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of STUN |
| STUN | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `stun.yaml` |
| TURN | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of STUN |
| RTMP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `rtmp.yaml` |
| WKSSVC | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| SPOOLSS | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| ATSVC | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| IObjectExporter | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| LLMNR-MDNS collision | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of LLMNR |
| WPAD | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| TPKT | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/msrdp.yaml` |
| BitTorrent | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `bittorrent.yaml` |
| MinIO/S3 | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| RMI | B | 75 | 0 | 25 | 20 | 20 | 10 | L2 | alias of RMI/JRMP |
| JMX | B | 75 | 0 | 25 | 20 | 20 | 10 | L2 | alias of RMI/JRMP |
| JDWP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `jdwp.yaml` |
| FastCGI | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `fastcgi.yaml` |
| IIOP Locate | B | 75 | 0 | 25 | 20 | 20 | 10 | L2 | alias of IIOP/GIOP |
| Memcache binary | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of Memcached |
| Docker API | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| Kubernetes API | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| Elasticsearch transport | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| Jenkins remoting | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `jenkins.yaml` |
| Redis Sentinel/Cluster | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/redis.yaml` |
| Zabbix agent | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `zabbix.yaml` |
| VMware SOAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| IPMI RMCP+ | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | alias of IPMI |
| WS-Man | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| WinRM HTTP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| PowerShell PSRP | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| .NET Remoting | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `net_remoting.yaml` |
| Hessian2 | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `hessian.yaml` |
| PHP serialize | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `php_ser.yaml` |
| Python pickle | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `pickle.yaml` |
| JDWP handshake | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of JDWP |
| Rsync daemon | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `rsync.yaml` |
| Docker Registry | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of HTTP |
| gRPC reflection | A | 90 | 20 | 20 | 20 | 20 | 10 | L2 | alias of HTTP/2 |
| SaltStack | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `salt.yaml` |
| LDAP paged/SASL | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/ldap.yaml` |
| DHCPv6 server exchange | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of DHCPv6 |
| IPv6 RA | A | 100 | 25 | 25 | 20 | 20 | 10 | L1 | alias of ICMPv6 NDP |
