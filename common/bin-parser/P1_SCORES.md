# P1 协议交付打分

对照 [PROTOCOL_DELIVERY.md](PROTOCOL_DELIVERY.md)。机器可读记录在 `p1_scores.go`，由 `TestP1ScorecardsCovered` 校验：每个 P1 名称都有计分卡；`Status: done` 必须是 **B**（总分 ≥ 75）。A 级（≥ 90）留给 Wireshark 级主 PDU。

维度（满分 100）：Schema 25 / 真实流量 25 / 测试 20 / 分支覆盖 20 / 栈集成 10。硬门槛 G1–G8 全过才计分。

别名与主规则共用同一张卡（见 `AliasOf`）。样本来源包括 gopacket 测试帧、RFC 完整 PDU，以及 Ethernet+IP+L4 整帧断言。

G5 要求 SampleClass ∈ {L1, L2, L3}；L4-only handmade PDU 不计分。L3 gopacket serialize 的 Traffic ≤ 8。

`TestP1ScorecardsCovered` 用 YAML/`p1FailCases`/mustChild 扫描卡死 Schema/Tests/Traffic 上限：声称分不得高于 `schemaCeiling` / `testsCeiling` / `trafficCeiling`。G1–G4/G6–G8 由测试从规则文件、失败路径和以太网封装推导，`p1card` 不得写死为 true。

| 协议 | 等级 | 总分 | Schema | 流量 | 测试 | 分支 | 栈 | 样本 | 规则 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Ethernet 802.2 | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `llc.yaml` |
| Ethernet 802.3 | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `llc.yaml` |
| Ethernet SNAP | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `llc.yaml` |
| IEEE 802.1ad QinQ | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `ieee_802_1ad.yaml` |
| PPP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `ppp.yaml` |
| PPPoE Discovery | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `pppoe.yaml` |
| PPPoE Session | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `pppoe.yaml` |
| LCP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `link_control_protocol.yaml` |
| PAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `password_authentication_protocol.yaml` |
| CHAP | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `challenge_handshake_authentication_protocol.yaml` |
| EAPOL | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | `eapol.yaml` |
| EAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `eapol.yaml` |
| STP | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `stp.yaml` |
| RSTP | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | alias of STP |
| LLDP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | `lldp.yaml` |
| CDP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `cdp.yaml` |
| Loopback | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `loopback.yaml` |
| Linux SLL | B | 80 | 15 | 15 | 20 | 20 | 10 | L2 | `linux_sll.yaml` |
| IEEE 802.11 | A | 90 | 25 | 15 | 20 | 20 | 10 | L2 | `ieee_802_11.yaml` |
| WPA/RSN | A | 90 | 25 | 15 | 20 | 20 | 10 | L2 | `ieee_802_11.yaml` |
| IEEE 802.1X | A | 95 | 20 | 25 | 20 | 20 | 10 | L1 | alias of EAPOL |
| ICMPv6 NDP | A | 100 | 25 | 25 | 20 | 20 | 10 | L1 | `internet_control_message_protocol_v6.yaml` |
| IGMP | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `igmp.yaml` |
| GRE | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `generic_routing_encapsulation.yaml` |
| IPsec AH | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `ipsec.yaml` |
| IPsec ESP | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `ipsec.yaml` |
| MPLS | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `mpls.yaml` |
| VXLAN | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `vxlan.yaml` |
| L2TP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `l2tp.yaml` |
| PPTP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/pptp.yaml` |
| OpenVPN | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `openvpn.yaml` |
| WireGuard | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `wireguard.yaml` |
| IKEv1 | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `ike.yaml` |
| IKEv2 | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `ike.yaml` |
| NAT-T | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of IKEv2 |
| OSPF | A | 100 | 25 | 25 | 20 | 20 | 10 | L1 | `ospf.yaml` |
| BGP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `bgp.yaml` |
| RIP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `rip.yaml` |
| EIGRP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `eigrp.yaml` |
| VRRP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | `vrrp.yaml` |
| HSRP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `hsrp.yaml` |
| SCTP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `sctp.yaml` |
| NBT DG | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `nbt_dg.yaml` |
| mDNS | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `application-layer/dns.yaml` |
| LLMNR | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | alias of mDNS |
| DoT | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| DoH | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| DHCPv6 | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `dhcpv6.yaml` |
| BOOTP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `application-layer/dhcp.yaml` |
| NTP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | `application-layer/ntp.yaml` |
| SSDP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| UPnP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| HTTP/3 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/quic.yaml` |
| SSL | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| DTLS | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `dtls.yaml` |
| gRPC | B | 86 | 20 | 20 | 16 | 20 | 10 | L2 | alias of HTTP/2 |
| SOAP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| JSON-RPC | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `jsonrpc.yaml` |
| SMTPS | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| POP3 | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `pop3.yaml` |
| IMAP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `imap.yaml` |
| FTP-DATA | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `ftp_data.yaml` |
| TFTP | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `tftp.yaml` |
| SFTP | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `application-layer/ssh.yaml` |
| NFS | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `onc_rpc.yaml` |
| RPC | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of NFS |
| Portmap/Rpcbind | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of NFS |
| LDAPS | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| CLDAP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/ldap.yaml` |
| GSS-API | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/spnego.yaml` |
| TACACS+ | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `tacacs.yaml` |
| SOCKS4 | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `socks4.yaml` |
| HTTP Proxy CONNECT | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| Telnet | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `telnet.yaml` |
| CREDSSP | A | 90 | 15 | 25 | 20 | 20 | 10 | L1 | alias of TLS |
| VNC/RFB | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `vnc.yaml` |
| WinRM | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| MariaDB | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `application-layer/mysql.yaml` |
| MongoDB | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `mongodb.yaml` |
| Memcached | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `memcached.yaml` |
| Elasticsearch | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| AMQP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `amqp.yaml` |
| Kafka | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `kafka.yaml` |
| RabbitMQ | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of AMQP |
| Thrift | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `thrift.yaml` |
| Protobuf | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `protobuf.yaml` |
| JSON-RPC 2.0 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of JSON-RPC |
| ONC RPC | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `onc_rpc.yaml` |
| IIOP/GIOP | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/iiop.yaml` |
| T3 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/t3.yaml` |
| RMI/JRMP | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `rmi.yaml` |
| JNDI | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | alias of RMI/JRMP |
| BER | B | 85 | 20 | 15 | 20 | 20 | 10 | L2 | `application-layer/ber.yaml` |
| SNMPv3 | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/snmp.yaml` |
| Syslog | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `syslog.yaml` |
| IPMI | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `ipmi.yaml` |
| SIP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `sip.yaml` |
| SDP | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `sdp.yaml` |
| RTP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `rtp.yaml` |
| RTCP | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `rtp.yaml` |
| RTSP | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `rtsp.yaml` |
| WebRTC | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of STUN |
| STUN | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `stun.yaml` |
| TURN | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of STUN |
| RTMP | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `rtmp.yaml` |
| WKSSVC | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| SPOOLSS | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| ATSVC | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| IObjectExporter | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `application-layer/dcerpc.yaml` |
| LLMNR-MDNS collision | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | alias of LLMNR |
| WPAD | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| TPKT | B | 86 | 15 | 25 | 16 | 20 | 10 | L1 | `application-layer/msrdp.yaml` |
| BitTorrent | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `bittorrent.yaml` |
| MinIO/S3 | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| RMI | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | alias of RMI/JRMP |
| JMX | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | alias of RMI/JRMP |
| JDWP | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `jdwp.yaml` |
| FastCGI | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `fastcgi.yaml` |
| IIOP Locate | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of IIOP/GIOP |
| Memcache binary | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | alias of Memcached |
| Docker API | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| Kubernetes API | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| Elasticsearch transport | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| Jenkins remoting | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `jenkins.yaml` |
| Redis Sentinel/Cluster | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `application-layer/redis.yaml` |
| Zabbix agent | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `zabbix.yaml` |
| VMware SOAP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| IPMI RMCP+ | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | alias of IPMI |
| WS-Man | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| WinRM HTTP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| PowerShell PSRP | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| .NET Remoting | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `net_remoting.yaml` |
| Hessian2 | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `hessian.yaml` |
| PHP serialize | B | 86 | 15 | 25 | 16 | 20 | 10 | L2 | `php_ser.yaml` |
| Python pickle | A | 91 | 20 | 25 | 16 | 20 | 10 | L2 | `pickle.yaml` |
| JDWP handshake | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | alias of JDWP |
| Rsync daemon | A | 90 | 15 | 25 | 20 | 20 | 10 | L2 | `rsync.yaml` |
| Docker Registry | A | 91 | 20 | 25 | 16 | 20 | 10 | L1 | alias of HTTP |
| gRPC reflection | B | 86 | 20 | 20 | 16 | 20 | 10 | L2 | alias of HTTP/2 |
| SaltStack | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | `salt.yaml` |
| LDAP paged/SASL | A | 95 | 20 | 25 | 20 | 20 | 10 | L2 | `application-layer/ldap.yaml` |
| DHCPv6 spoof | A | 100 | 25 | 25 | 20 | 20 | 10 | L2 | alias of DHCPv6 |
| IPv6 RA | A | 100 | 25 | 25 | 20 | 20 | 10 | L1 | alias of ICMPv6 NDP |
