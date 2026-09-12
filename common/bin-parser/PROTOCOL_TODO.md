# 协议实现 TODO

后续工作以逐协议补全为主：已有抓包、TCP 重组、消息分帧、会话亲和、结构化输出、
CLI 和回放基础。每次交付应把一个明确协议阶段的字段、状态、实时入口和验证补齐。

状态以 [protocol_roadmap.go](protocol_roadmap.go) 和
[protocol_catalog.go](protocol_catalog.go) 为准。路线图的 `done` 仅表示其限定报文
合同通过验收，不等于完整协议、所有版本或实时抓包已支持；目录的 `stable` 也是
已有行为被测试锁定，不是 Wireshark 功能对等声明。

## 优先推进的实时协议工作

| 优先顺序 | 协议/家族 | 已有基础 | 待交付 |
|---|---|---|---|
| 1 | MySQL/MariaDB、PostgreSQL、LDAP | 明确方向/阶段的字段模型和结构化入口 | 握手/能力/认证状态、双向阶段迁移、完整消息分帧、pcapx binding；缺少上下文必须明确报出 |
| 1 | HTTP/2、WebSocket | 现有帧/消息规则及协议样本 | HTTP Upgrade/ALPN 选择、跨帧/多路流状态、头压缩或分片消息、完整实时链路回归 |
| 1 | SMTP、IMAP、POP3 | 显式请求/响应字段入口 | 行与多行响应、IMAP literal、状态/方向关联、STARTTLS 转换及自动准入 |
| 1 | TLS | 记录分帧、ClientHello，另有显式证书入口 | 更多握手类型、跨记录握手重组、证书入口接入和协商状态；密文保持边界，不伪称已解密 |
| 1 | DNS | UDP/TCP 实时入口、完整现有规则投影 | 扩展具名 RDATA 类型与复杂组合覆盖；DoH/DoT/DoQ 须先完成对应加密/传输层上下文 |
| 2 | MQTT | CONNECT 确认 3.1/3.1.1 后双向解析 | MQTT 5.0、更多协商属性与会话生命周期；缺少 CONNECT 不猜版本 |
| 2 | Cassandra、Memcached | 有限 OPTIONS/STARTUP/SUPPORTED、stats/binary GET 等范围 | 后续查询/命令/响应、压缩与协商、完整会话阶段准入 |
| 2 | TDS、TNS、SMB、Kerberos | 显式 PDU/版本/方向模型，Kerberos 已有实时入口 | 真实会话协商、方向/事务状态、后续 PDU、对应实时 binding；密文/未知能力仍显式保留 |
| 3 | 企业网、工控、存储与长尾 | 下方目录中的字段规则、样本和部分负例 | 按一个家族完成具名字段、规范约束、真实捕获、坏包验证，再接自动分发 |

当前 pcapx 自动准入仅覆盖 HTTP/1.x、TLS、MQTT 3.1/3.1.1、DNS、Kerberos 以及
Memcached/Cassandra 的上述限定范围。其他规则需要显式入口，不能仅按端口猜测
会话状态。用法与精确边界见 [抓包分析指南](../pcapx/pcaputil/BIN_PARSER.md)。

AnyDesk、DingTalk、DoH、DoQ、DoT、HTTP/3、IMAPS、SMTPS、T.38、WeChat/MicroMsg
现有外层/派生流不算完整业务协议；爱奇艺 P2P、腾讯游戏、网易游戏、米哈游/HoYoverse
的识别结果也不算具名业务字段交付。保留其正负例，补齐后再提升状态。

## 完整待办清单

从仓库根目录运行 `go generate ./common/bin-parser` 更新下方清单；生成器只读取
目录/路线图，不初始化解析器，不修改协议状态。

<!-- BEGIN GENERATED PROTOCOL INVENTORY -->

路线图共 **616** 项：限定范围 `done` **211**，`partial` **0**，`todo` **405**。目录共 **465** 个入口：`stable` **15**，`partial` **265**，`new` **185**。入口、规则文件和协议家族不是同一个计数。

### 路线图尚未完成的项目

同名目录只作定位；没有同名不等于没有实现，别名与专用 Fields 入口需继续核对下表及 `protocol_catalog.go`。`done` 项的剩余范围也在后面的非 stable 目录中保留。

| 优先级 | 协议 | 路线图状态 | 同名目录入口 |
|---|---|---|---|
| P2 | 3GPP NAS | todo | 需核对别名/Fields 入口 |
| P2 | 3GPP NGAP | todo | 需核对别名/Fields 入口 |
| P2 | 3GPP S1AP | todo | 需核对别名/Fields 入口 |
| P2 | ADWS | todo | 需核对别名/Fields 入口 |
| P2 | AFP | todo | [new / AFP](rules/application-layer/extended_protocols.yaml) |
| P2 | AMF0/AMF3 | todo | 需核对别名/Fields 入口 |
| P2 | ASN.1 | todo | 需核对别名/Fields 入口 |
| P2 | ActiveMQ OpenWire | todo | [new / ActiveMQOpenWire](rules/application-layer/extended_protocols.yaml) |
| P2 | Ansible | todo | 需核对别名/Fields 入口 |
| P2 | BACnet | todo | [new / BACnetIP](rules/application-layer/extended_protocols.yaml) |
| P2 | BFD | todo | [new / BFD](rules/application-layer/extended_protocols.yaml) |
| P2 | Beckhoff ADS | todo | [new / BeckhoffADS](rules/application-layer/extended_protocols.yaml) |
| P2 | Bonjour | todo | [partial / DNS](rules/application-layer/dns.yaml) |
| P2 | CAN/ISO-TP | todo | [new / CANEthernet](rules/application-layer/extended_protocols.yaml) |
| P2 | CODESYS | todo | 需核对别名/Fields 入口 |
| P2 | COTP | todo | [partial / COTP](rules/application-layer/isotp.yaml) |
| P2 | CWMP/TR-069 | todo | 需核对别名/Fields 入口 |
| P2 | Cassandra CQL | todo | [new / CassandraCQL](rules/application-layer/extended_protocols.yaml) |
| P2 | Ceph | todo | [new / CephConnect](rules/application-layer/extended_protocols.yaml) |
| P2 | Citrix ICA | todo | [new / CitrixICA](rules/application-layer/extended_protocols.yaml) |
| P2 | ClickHouse | todo | 需核对别名/Fields 入口 |
| P2 | Consul | todo | 需核对别名/Fields 入口 |
| P2 | Consul RPC | todo | 需核对别名/Fields 入口 |
| P2 | DB2 DRDA | todo | [new / DRDA](rules/application-layer/extended_protocols.yaml) |
| P2 | DER | todo | 需核对别名/Fields 入口 |
| P2 | DLMS/COSEM | todo | [new / DLMSHDLC](rules/application-layer/extended_protocols.yaml) |
| P2 | DMVPN | todo | 需核对别名/Fields 入口 |
| P2 | DNP3 | todo | [new / DNP3](rules/application-layer/extended_protocols.yaml) |
| P2 | Diameter | todo | [new / Diameter](rules/application-layer/extended_protocols.yaml) |
| P2 | Diameter Cx/Dx | todo | 需核对别名/Fields 入口 |
| P2 | DoIP | todo | [partial / DoIP](rules/application-layer/doip.yaml) |
| P2 | DoQ | todo | [partial / DoQQueryStream](rules/application-layer/doq.yaml) |
| P2 | ESXi hostd | todo | 需核对别名/Fields 入口 |
| P2 | ETCD | todo | [partial / EtcdVersionHTTP](rules/application-layer/etcd.yaml) |
| P2 | EtherCAT | todo | [partial / EtherCAT](rules/ethercat.yaml) |
| P2 | EtherNet/IP CIP | todo | [new / EtherNetIPCIPIO](rules/application-layer/extended_protocols.yaml) |
| P2 | Exchange EWS | todo | 需核对别名/Fields 入口 |
| P2 | FCP | todo | 需核对别名/Fields 入口 |
| P2 | FCoE | todo | [partial / FCoE](rules/fcoe.yaml) |
| P2 | FIX | todo | [new / FIX](rules/application-layer/extended_protocols.yaml) |
| P2 | FTPS | todo | [new / FTPAuthTLS](rules/application-layer/ftp.yaml) |
| P2 | Fibre Channel | todo | [partial / FibreChannel](rules/fibre_channel.yaml) |
| P2 | GB28181 | todo | 需核对别名/Fields 入口 |
| P2 | GE SRTP | todo | 需核对别名/Fields 入口 |
| P2 | GLBP | todo | [new / GLBP](rules/application-layer/extended_protocols.yaml) |
| P2 | GTP-C | todo | [new / GTPv2](rules/application-layer/extended_protocols.yaml) |
| P2 | GTP-U | todo | [new / GTPv1](rules/application-layer/extended_protocols.yaml) |
| P2 | GTPv2 | todo | [partial / GTPv2](rules/application-layer/extended_protocols.yaml) |
| P2 | Geneve | todo | [partial / Geneve](rules/geneve.yaml) |
| P2 | Git daemon | todo | [new / GitDaemon](rules/application-layer/extended_protocols.yaml) |
| P2 | H.225 | todo | [partial / H225RAS](rules/application-layer/h225.yaml) |
| P2 | H.245 | todo | 需核对别名/Fields 入口 |
| P2 | H.323 | todo | [partial / H225Call](rules/application-layer/h225.yaml) |
| P2 | HBase | todo | 需核对别名/Fields 入口 |
| P2 | Hart-IP | todo | [new / HartIP](rules/application-layer/extended_protocols.yaml) |
| P2 | IBM MQ | todo | 需核对别名/Fields 入口 |
| P2 | ICE | todo | 需核对别名/Fields 入口 |
| P2 | ICMP Timestamp | todo | [partial / ICMP](rules/internet_control_message_protocol.yaml) |
| P2 | ICMPv6 MLD | todo | [partial / ICMPV6](rules/internet_control_message_protocol_v6.yaml) |
| P2 | IEC 60870-5-101 | todo | 需核对别名/Fields 入口 |
| P2 | IEC 60870-5-104 | todo | [new / IEC104](rules/application-layer/extended_protocols.yaml) |
| P2 | IEC 61850 GOOSE | todo | [partial / GOOSE](rules/iec61850.yaml) |
| P2 | IEC 61850 MMS | todo | [partial / MMSPDU](rules/application-layer/mms.yaml) |
| P2 | IEC 61850 SV | todo | [partial / SampledValues](rules/iec61850.yaml) |
| P2 | IEC 62056 | todo | 需核对别名/Fields 入口 |
| P2 | IEEE 802.11 Radiotap | todo | [new / RadioTapHeader](rules/application-layer/extended_protocols.yaml) |
| P2 | IIOP over SSL | todo | 需核对别名/Fields 入口 |
| P2 | IMAPS | todo | [partial / ](rules/application-layer/tls.yaml) |
| P2 | IPFIX | todo | [partial / IPFIX](rules/application-layer/ipfix.yaml) |
| P2 | IPIP | todo | [partial / Internet Protocol](rules/internet_protocol.yaml) |
| P2 | IPP | todo | [new / IPP](rules/application-layer/extended_protocols.yaml) |
| P2 | IPv6 Destination Options | todo | [partial / Internet Protocol Version 6](rules/internet_protocol_version_6.yaml) |
| P2 | IPv6 Fragment | todo | [partial / Internet Protocol Version 6](rules/internet_protocol_version_6.yaml) |
| P2 | IPv6 Hop-by-Hop | todo | [partial / Internet Protocol Version 6](rules/internet_protocol_version_6.yaml) |
| P2 | IPv6 Routing Header | todo | [partial / Internet Protocol Version 6](rules/internet_protocol_version_6.yaml) |
| P2 | IRC | todo | [new / IRC](rules/application-layer/extended_protocols.yaml) |
| P2 | IS-IS | todo | [partial / ISIS](rules/isis.yaml) |
| P2 | ISO 8583 | todo | 需核对别名/Fields 入口 |
| P2 | J1939 | todo | [partial / J1939](rules/j1939.yaml) |
| P2 | KNX/IP | todo | [new / KNXIP](rules/application-layer/extended_protocols.yaml) |
| P2 | Kryo | todo | 需核对别名/Fields 入口 |
| P2 | LACP | todo | [partial / LACP](rules/link_aggregation.yaml) |
| P2 | LDP | todo | [new / LDP](rules/application-layer/extended_protocols.yaml) |
| P2 | LLC | todo | [new / ](rules/llc.yaml) |
| P2 | Libvirt | todo | 需核对别名/Fields 入口 |
| P2 | Linux SLL2 | todo | [partial / LinuxSLL2](rules/linux_sll2.yaml) |
| P2 | LoRaWAN | todo | 需核对别名/Fields 入口 |
| P2 | MACSec | todo | [partial / MACSec](rules/macsec.yaml) |
| P2 | MAPI | todo | 需核对别名/Fields 入口 |
| P2 | MGCP | todo | [new / MGCP](rules/application-layer/extended_protocols.yaml) |
| P2 | MPEG-TS | todo | [new / MPEGTransportStream](rules/application-layer/extended_protocols.yaml) |
| P2 | MPLS PW | todo | [partial / MPLSEthernetPW](rules/network_samples.yaml) |
| P2 | MQTT-SN | todo | 需核对别名/Fields 入口 |
| P2 | MSTP | todo | 需核对别名/Fields 入口 |
| P2 | Modbus RTU/ASCII | todo | 需核对别名/Fields 入口 |
| P2 | Modbus TCP | todo | [new / ModbusTCP](rules/application-layer/extended_protocols.yaml) |
| P2 | Mount | todo | [partial / ONCRPC](rules/onc_rpc.yaml) |
| P2 | NATS | todo | [new / NATS](rules/application-layer/extended_protocols.yaml) |
| P2 | NETCONF | todo | 需核对别名/Fields 入口 |
| P2 | NFS4.1/4.2 | todo | 需核对别名/Fields 入口 |
| P2 | NHRP | todo | [partial / NHRP](rules/nhrp.yaml) |
| P2 | NVGRE | todo | [partial / NVGRE](rules/network_samples.yaml) |
| P2 | NVMe-oF | todo | [partial / NVMeTCP](rules/application-layer/nvme_tcp.yaml) |
| P2 | Nagios NRPE | todo | 需核对别名/Fields 入口 |
| P2 | NetFlow v5 | todo | [partial / NetFlowV5](rules/application-layer/netflow_v5.yaml) |
| P2 | NetFlow v9 | todo | [new / NetFlowV9](rules/application-layer/extended_protocols.yaml) |
| P2 | OAuth/OIDC wire | todo | 需核对别名/Fields 入口 |
| P2 | OCI distribution | todo | 需核对别名/Fields 入口 |
| P2 | OCSP | todo | [new / OCSPRequest](rules/application-layer/ocsp.yaml) |
| P2 | ONVIF | todo | 需核对别名/Fields 入口 |
| P2 | OPC DA | todo | 需核对别名/Fields 入口 |
| P2 | OPC UA | todo | [new / OPCUAHello](rules/application-layer/extended_protocols.yaml) |
| P2 | OSPFv3 | todo | [partial / OSPFv3](rules/ospfv3.yaml) |
| P2 | OXABREF/NSPI | todo | 需核对别名/Fields 入口 |
| P2 | Omron FINS | todo | [new / OmronFINS](rules/application-layer/extended_protocols.yaml) |
| P2 | PFCP | todo | [new / PFCP](rules/application-layer/extended_protocols.yaml) |
| P2 | PIM | todo | [new / PIM](rules/application-layer/extended_protocols.yaml) |
| P2 | POP3S | todo | 需核对别名/Fields 入口 |
| P2 | PPPoE+BRAS | todo | 需核对别名/Fields 入口 |
| P2 | PROFIBUS | todo | 需核对别名/Fields 入口 |
| P2 | Profinet DCP | todo | [partial / ProfinetDCP](rules/profinet_dcp.yaml) |
| P2 | Profinet IO | todo | [new / ProfinetIOReadImplicit](rules/application-layer/profinet_io.yaml) |
| P2 | Prometheus exposition | todo | [partial / PrometheusExposition](rules/application-layer/prometheus.yaml) |
| P2 | Puppet | todo | 需核对别名/Fields 入口 |
| P2 | Q.931 | todo | 需核对别名/Fields 入口 |
| P2 | QEMU QMP | todo | 需核对别名/Fields 入口 |
| P2 | RARP | todo | [partial / RARPFrame](rules/rarp.yaml) |
| P2 | RESTCONF | todo | 需核对别名/Fields 入口 |
| P2 | RIPng | todo | [partial / RIPng](rules/ripng.yaml) |
| P2 | RPRN | todo | 需核对别名/Fields 入口 |
| P2 | RSVP | todo | [partial / RSVP](rules/application-layer/observed_protocols.yaml) |
| P2 | Redfish | todo | [partial / RedfishServiceRootRequest](rules/application-layer/redfish.yaml) |
| P2 | Redfish SSDP | todo | [partial / RedfishSSDP](rules/application-layer/redfish_ssdp.yaml) |
| P2 | Rsync | todo | [partial / Rsync](rules/rsync.yaml) |
| P2 | Ruby Marshal | todo | 需核对别名/Fields 入口 |
| P2 | S7comm | todo | [new / S7comm](rules/application-layer/extended_protocols.yaml) |
| P2 | S7comm-plus | todo | [new / S7commPlus](rules/application-layer/extended_protocols.yaml) |
| P2 | SAML | todo | 需核对别名/Fields 入口 |
| P2 | SCCP/Skinny | todo | [new / Skinny](rules/application-layer/extended_protocols.yaml) |
| P2 | SCP | todo | 需核对别名/Fields 入口 |
| P2 | SIP IMS | todo | 需核对别名/Fields 入口 |
| P2 | SMB-Direct | todo | 需核对别名/Fields 入口 |
| P2 | SMB3 multichannel | todo | 需核对别名/Fields 入口 |
| P2 | SMPP | todo | [new / SMPPBind](rules/application-layer/extended_protocols.yaml) |
| P2 | SNAP | todo | [partial / LLC](rules/llc.yaml) |
| P2 | SNTP | todo | [partial / NTP](rules/application-layer/ntp.yaml) |
| P2 | SRTP | todo | 需核对别名/Fields 入口 |
| P2 | SRVLOC/SLP | todo | [new / SLPv2](rules/application-layer/extended_protocols.yaml) |
| P2 | SSTP | todo | 需核对别名/Fields 入口 |
| P2 | STOMP | todo | [new / STOMP](rules/application-layer/extended_protocols.yaml) |
| P2 | SWIFT | todo | 需核对别名/Fields 入口 |
| P2 | Solr | todo | 需核对别名/Fields 入口 |
| P2 | Submission | todo | [partial / SMTPCommand](rules/application-layer/smtp.yaml) |
| P2 | Sybase TDS | todo | 需核对别名/Fields 入口 |
| P2 | UDS | todo | 需核对别名/Fields 入口 |
| P2 | VMware AFD | todo | 需核对别名/Fields 入口 |
| P2 | Vault | todo | 需核对别名/Fields 入口 |
| P2 | WCF | todo | 需核对别名/Fields 入口 |
| P2 | WEP | todo | [partial / WEP](rules/wep.yaml) |
| P2 | WINS | todo | 需核对别名/Fields 入口 |
| P2 | WS-Discovery | todo | [partial / WSDiscovery](rules/application-layer/ws_discovery.yaml) |
| P2 | WebDAV | todo | [partial / HTTP](rules/application-layer/http.yaml) |
| P2 | XML-RPC | todo | [partial / XMLRPC](rules/application-layer/xmlrpc.yaml) |
| P2 | XMPP | todo | [partial / XMPP](rules/application-layer/xmpp.yaml) |
| P2 | Z-Wave | todo | 需核对别名/Fields 入口 |
| P2 | Zabbix | todo | [partial / Zabbix](rules/zabbix.yaml) |
| P2 | Zigbee | todo | [partial / Zigbee](rules/zigbee.yaml) |
| P2 | ZooKeeper | todo | 需核对别名/Fields 入口 |
| P2 | etcd raft | todo | 需核对别名/Fields 入口 |
| P2 | gRPC-Web | todo | 需核对别名/Fields 入口 |
| P2 | iSCSI | todo | [partial / ISCSIBasicHeader](rules/application-layer/sample_protocols.yaml) |
| P2 | sFlow | todo | [new / SFlowV5](rules/application-layer/extended_protocols.yaml) |
| P2 | vSphere NBDSSL | todo | 需核对别名/Fields 入口 |
| P3 | 360 Safe | todo | 需核对别名/Fields 入口 |
| P3 | 360 Tianji | todo | 需核对别名/Fields 入口 |
| P3 | AliWangWang | todo | 需核对别名/Fields 入口 |
| P3 | Alipay | todo | 需核对别名/Fields 入口 |
| P3 | Alipay mtop | todo | 需核对别名/Fields 入口 |
| P3 | AliyunDrive | todo | 需核对别名/Fields 入口 |
| P3 | AnyDesk | todo | [partial / ](rules/application-layer/tls.yaml) |
| P3 | BaiduNetdisk | todo | 需核对别名/Fields 入口 |
| P3 | Bilibili P2P | todo | 需核对别名/Fields 入口 |
| P3 | CC-Link IE | todo | 需核对别名/Fields 入口 |
| P3 | CMPP | todo | [partial / CMPPConnect](rules/application-layer/sample_protocols.yaml) |
| P3 | DPTech | todo | 需核对别名/Fields 入口 |
| P3 | Dahua | todo | 需核对别名/Fields 入口 |
| P3 | DiDi | todo | 需核对别名/Fields 入口 |
| P3 | DingTalk | todo | [partial / ](rules/application-layer/tls.yaml) |
| P3 | Douyin/TikTok | todo | 需核对别名/Fields 入口 |
| P3 | Dubbo | todo | 需核对别名/Fields 入口 |
| P3 | Feishu/Lark | todo | 需核对别名/Fields 入口 |
| P3 | Fiberhome | todo | 需核对别名/Fields 入口 |
| P3 | H3C | todo | 需核对别名/Fields 入口 |
| P3 | H3C IRF | todo | 需核对别名/Fields 入口 |
| P3 | Hessian | todo | 需核对别名/Fields 入口 |
| P3 | Hikvision ISAPI | todo | 需核对别名/Fields 入口 |
| P3 | Hikvision SDK | todo | 需核对别名/Fields 入口 |
| P3 | Hillstone | todo | 需核对别名/Fields 入口 |
| P3 | Huawei NCA | todo | 需核对别名/Fields 入口 |
| P3 | Huawei NHRP+ | todo | 需核对别名/Fields 入口 |
| P3 | Huawei SSL VPN | todo | 需核对别名/Fields 入口 |
| P3 | JD | todo | 需核对别名/Fields 入口 |
| P3 | Kuaishou | todo | 需核对别名/Fields 入口 |
| P3 | Kugou | todo | 需核对别名/Fields 入口 |
| P3 | Kuwo | todo | 需核对别名/Fields 入口 |
| P3 | Leadsec | todo | 需核对别名/Fields 入口 |
| P3 | MELSEC | todo | [new / MELSEC](rules/application-layer/extended_protocols.yaml) |
| P3 | Maipu | todo | 需核对别名/Fields 入口 |
| P3 | Meituan | todo | 需核对别名/Fields 入口 |
| P3 | NSFOCUS | todo | 需核对别名/Fields 入口 |
| P3 | NetEase Games | todo | 需核对别名/Fields 入口 |
| P3 | NetEase POPO | todo | 需核对别名/Fields 入口 |
| P3 | Netease Cloud Music | todo | 需核对别名/Fields 入口 |
| P3 | PBOC | todo | 需核对别名/Fields 入口 |
| P3 | PPLive | todo | 需核对别名/Fields 入口 |
| P3 | PPStream | todo | 需核对别名/Fields 入口 |
| P3 | Pinduoduo | todo | 需核对别名/Fields 入口 |
| P3 | Portal/WebAuth | todo | 需核对别名/Fields 入口 |
| P3 | Pulsar | todo | 需核对别名/Fields 入口 |
| P3 | QQ | todo | 需核对别名/Fields 入口 |
| P3 | QQ KeepAlive | todo | 需核对别名/Fields 入口 |
| P3 | QQ Login | todo | 需核对别名/Fields 入口 |
| P3 | QQ Message | todo | 需核对别名/Fields 入口 |
| P3 | QQ Music | todo | 需核对别名/Fields 入口 |
| P3 | QiAnXin NGFW | todo | 需核对别名/Fields 入口 |
| P3 | QiAnXin Tianqing | todo | 需核对别名/Fields 入口 |
| P3 | Quark | todo | 需核对别名/Fields 入口 |
| P3 | RADIUS accounting CN | todo | 需核对别名/Fields 入口 |
| P3 | RTMPE | todo | 需核对别名/Fields 入口 |
| P3 | RTSP-over-GB | todo | 需核对别名/Fields 入口 |
| P3 | RocketMQ | todo | 需核对别名/Fields 入口 |
| P3 | Ruijie | todo | 需核对别名/Fields 入口 |
| P3 | RustDesk | todo | 需核对别名/Fields 入口 |
| P3 | SGIP | todo | 需核对别名/Fields 入口 |
| P3 | SMGP | todo | 需核对别名/Fields 入口 |
| P3 | Sangfor AF | todo | 需核对别名/Fields 入口 |
| P3 | Sangfor SSL VPN | todo | 需核对别名/Fields 入口 |
| P3 | Sangfor aTrust | todo | 需核对别名/Fields 入口 |
| P3 | SofaRPC | todo | 需核对别名/Fields 入口 |
| P3 | Sunlogin/Oray | todo | 需核对别名/Fields 入口 |
| P3 | TIM | todo | 需核对别名/Fields 入口 |
| P3 | Taobao | todo | 需核对别名/Fields 入口 |
| P3 | TeamViewer | todo | [partial / TeamViewerRecord](rules/application-layer/teamviewer.yaml) |
| P3 | Tencent Games | todo | 需核对别名/Fields 入口 |
| P3 | Thunder/Xunlei | todo | 需核对别名/Fields 入口 |
| P3 | ToDesk | todo | 需核对别名/Fields 入口 |
| P3 | Topsec | todo | 需核对别名/Fields 入口 |
| P3 | UnionPay | todo | 需核对别名/Fields 入口 |
| P3 | UnionPay CUPS | todo | 需核对别名/Fields 入口 |
| P3 | Uniview | todo | 需核对别名/Fields 入口 |
| P3 | Venus NTA | todo | 需核对别名/Fields 入口 |
| P3 | Venustech | todo | 需核对别名/Fields 入口 |
| P3 | WAI | todo | 需核对别名/Fields 入口 |
| P3 | WAPI | todo | 需核对别名/Fields 入口 |
| P3 | WeChat MMTLS | todo | 需核对别名/Fields 入口 |
| P3 | WeChat Pay | todo | 需核对别名/Fields 入口 |
| P3 | WeChat/MicroMsg | todo | [partial / ](rules/application-layer/tls.yaml) |
| P3 | WeCom | todo | 需核对别名/Fields 入口 |
| P3 | YY | todo | 需核对别名/Fields 入口 |
| P3 | Youku P2P | todo | 需核对别名/Fields 入口 |
| P3 | ZTE | todo | 需核对别名/Fields 入口 |
| P3 | eMule/ED2K | todo | [partial / EDonkey](rules/application-layer/edonkey.yaml) |
| P3 | iQIYI P2P | todo | 需核对别名/Fields 入口 |
| P3 | miHoYo/HoYoverse | todo | 需核对别名/Fields 入口 |
| P4 | 3GPP M2AP | todo | 需核对别名/Fields 入口 |
| P4 | 3GPP M3AP | todo | 需核对别名/Fields 入口 |
| P4 | 6in4 | todo | [new / Internet Protocol](rules/internet_protocol.yaml) |
| P4 | 6to4 | todo | [partial / Internet Protocol](rules/internet_protocol.yaml) |
| P4 | 9P | todo | [partial / NinePVersion](rules/application-layer/sample_protocols.yaml) |
| P4 | AARP | todo | [partial / AARP](rules/aarp.yaml) |
| P4 | ACMEv2 | todo | [partial / ACMEJWSRequest](rules/application-layer/acme.yaml) |
| P4 | AES50 | todo | 需核对别名/Fields 入口 |
| P4 | AllJoyn | todo | [partial / AllJoynNS](rules/application-layer/alljoyn.yaml) |
| P4 | AoE | todo | [partial / AoE](rules/aoe.yaml) |
| P4 | AppleTalk | todo | [partial / DDP](rules/appletalk.yaml) |
| P4 | Babel | todo | 需核对别名/Fields 入口 |
| P4 | BaiduHi | todo | 需核对别名/Fields 入口 |
| P4 | Burlap | todo | 需核对别名/Fields 入口 |
| P4 | CARP | todo | 需核对别名/Fields 入口 |
| P4 | CFM | todo | [partial / CFM](rules/cfm.yaml) |
| P4 | CHAOS | todo | 需核对别名/Fields 入口 |
| P4 | Cap'n Proto | todo | 需核对别名/Fields 入口 |
| P4 | Checkmk | todo | [partial / CheckmkReport](rules/application-layer/checkmk.yaml) |
| P4 | Chef | todo | 需核对别名/Fields 入口 |
| P4 | Collectd | todo | [new / Collectd](rules/application-layer/extended_protocols.yaml) |
| P4 | CouchDB | todo | 需核对别名/Fields 入口 |
| P4 | DALI | todo | 需核对别名/Fields 入口 |
| P4 | DALI-2 | todo | 需核对别名/Fields 入口 |
| P4 | DCCP | todo | [partial / DCCP](rules/dccp.yaml) |
| P4 | DECnet | todo | [partial / DECnet](rules/decnet.yaml) |
| P4 | DOCSIS | todo | 需核对别名/Fields 入口 |
| P4 | DVMRP | todo | [partial / DVMRP](rules/dvmrp.yaml) |
| P4 | E-LMI | todo | [partial / ELMI](rules/management_samples.yaml) |
| P4 | EGP | todo | 需核对别名/Fields 入口 |
| P4 | EOAM | todo | [partial / EOAM](rules/management_samples.yaml) |
| P4 | FF HSE | todo | 需核对别名/Fields 入口 |
| P4 | Fetion | todo | 需核对别名/Fields 入口 |
| P4 | Funshion | todo | 需核对别名/Fields 入口 |
| P4 | GTP Prime | todo | [new / GTPPrime](rules/application-layer/extended_protocols.yaml) |
| P4 | Gluster | todo | 需核对别名/Fields 入口 |
| P4 | Gnutella | todo | [new / Gnutella](rules/application-layer/extended_protocols.yaml) |
| P4 | Graphite | todo | 需核对别名/Fields 入口 |
| P4 | HLS | todo | 需核对别名/Fields 入口 |
| P4 | HomePlug AV | todo | [partial / HomePlugAV](rules/link_samples.yaml) |
| P4 | IAX2 | todo | [partial / IAX2](rules/application-layer/iax2.yaml) |
| P4 | IEEE 802.1AB | todo | 需核对别名/Fields 入口 |
| P4 | IEEE 802.1ah PBB | todo | [partial / ProviderBackboneBridge](rules/link_samples.yaml) |
| P4 | IEEE 802.3 Slow Protocols | todo | [partial / SlowProtocols](rules/link_samples.yaml) |
| P4 | IGRP | todo | [partial / IGRP](rules/igrp.yaml) |
| P4 | IPX | todo | [partial / IPX](rules/ipx.yaml) |
| P4 | IPcomp | todo | [partial / IPComp](rules/network_samples.yaml) |
| P4 | ISATAP | todo | 需核对别名/Fields 入口 |
| P4 | ISL | todo | 需核对别名/Fields 入口 |
| P4 | InfluxDB | todo | 需核对别名/Fields 入口 |
| P4 | Informix | todo | 需核对别名/Fields 入口 |
| P4 | Kingsoft | todo | 需核对别名/Fields 入口 |
| P4 | L2F | todo | 需核对别名/Fields 入口 |
| P4 | LACP-marker | todo | [partial / LACPMarker](rules/link_samples.yaml) |
| P4 | LAT | todo | [partial / LAT](rules/lat.yaml) |
| P4 | LMTP | todo | 需核对别名/Fields 入口 |
| P4 | LPD | todo | [partial / LPD](rules/application-layer/lpd.yaml) |
| P4 | LonTalk | todo | 需核对别名/Fields 入口 |
| P4 | M-Bus | todo | 需核对别名/Fields 入口 |
| P4 | MIPv6 | todo | [partial / MIPv6Packet](rules/mobility_samples.yaml) |
| P4 | MOP | todo | 需核对别名/Fields 入口 |
| P4 | MRP-MMRP | todo | [partial / MMRP](rules/management_samples.yaml) |
| P4 | MRP-MSRP | todo | [partial / MSRP](rules/management_samples.yaml) |
| P4 | MRP-MVRP | todo | [partial / MVRP](rules/management_samples.yaml) |
| P4 | MS-FASP | todo | 需核对别名/Fields 入口 |
| P4 | MS-WSP | todo | 需核对别名/Fields 入口 |
| P4 | MSDP | todo | [partial / MSDPMessage](rules/application-layer/sample_protocols.yaml) |
| P4 | MSNMS | todo | 需核对别名/Fields 入口 |
| P4 | Megaco/H.248 | todo | [partial / Megaco](rules/application-layer/megaco.yaml) |
| P4 | Mercurial | todo | 需核对别名/Fields 入口 |
| P4 | Mobile IP | todo | [partial / MobileIP](rules/mobility_samples.yaml) |
| P4 | NBD | todo | 需核对别名/Fields 入口 |
| P4 | NCP | todo | [partial / NCP](rules/application-layer/ncp.yaml) |
| P4 | NNTP | todo | [new / NNTP](rules/application-layer/extended_protocols.yaml) |
| P4 | NSQ | todo | 需核对别名/Fields 入口 |
| P4 | Nagios NSCA | todo | 需核对别名/Fields 入口 |
| P4 | NetBEUI | todo | [partial / NetBIOSFrame](rules/netbios_frame.yaml) |
| P4 | Nomad | todo | 需核对别名/Fields 入口 |
| P4 | OLSR | todo | [partial / OLSRPacket](rules/application-layer/sample_protocols.yaml) |
| P4 | PCAnywhere | todo | 需核对别名/Fields 入口 |
| P4 | PUP | todo | 需核对别名/Fields 入口 |
| P4 | PacketCable | todo | 需核对别名/Fields 入口 |
| P4 | PlayStation | todo | 需核对别名/Fields 入口 |
| P4 | Powerlink | todo | [partial / Powerlink](rules/powerlink.yaml) |
| P4 | Quake | todo | [partial / QuakeControl](rules/application-layer/sample_protocols.yaml) |
| P4 | Quake2 | todo | [partial / QuakeConnectionless](rules/application-layer/sample_protocols.yaml) |
| P4 | Quake3 | todo | [partial / QuakeConnectionless](rules/application-layer/sample_protocols.yaml) |
| P4 | Qvod | todo | 需核对别名/Fields 入口 |
| P4 | ROHC | todo | 需核对别名/Fields 入口 |
| P4 | RSH | todo | [new / RSH](rules/application-layer/extended_protocols.yaml) |
| P4 | Rlogin | todo | [partial / RloginRequest](rules/application-layer/sample_protocols.yaml) |
| P4 | Roofnet | todo | 需核对别名/Fields 入口 |
| P4 | SAMETIME | todo | 需核对别名/Fields 入口 |
| P4 | SEBEK | todo | [partial / SEBEK](rules/application-layer/sebek.yaml) |
| P4 | SERCOS III | todo | 需核对别名/Fields 入口 |
| P4 | SIGCOMP | todo | [partial / SIGCOMP](rules/application-layer/sigcomp.yaml) |
| P4 | SMUX | todo | 需核对别名/Fields 入口 |
| P4 | SNA | todo | [partial / SNAEthernet](rules/sna.yaml) |
| P4 | SPB | todo | 需核对别名/Fields 入口 |
| P4 | SPDY | todo | 需核对别名/Fields 入口 |
| P4 | SPX | todo | 需核对别名/Fields 入口 |
| P4 | SQLite over net | todo | 需核对别名/Fields 入口 |
| P4 | STT | todo | 需核对别名/Fields 入口 |
| P4 | SVN | todo | 需核对别名/Fields 入口 |
| P4 | SliMP3 | todo | [partial / SliMP3](rules/application-layer/slimp3.yaml) |
| P4 | SoulSeek | todo | 需核对别名/Fields 入口 |
| P4 | StatsD | todo | 需核对别名/Fields 入口 |
| P4 | Steam | todo | [partial / SteamDiscovery](rules/application-layer/steam_discovery.yaml) |
| P4 | T.38 | todo | [partial / T38H248SDPAdvertisement](rules/application-layer/t38_sdp.yaml) |
| P4 | TDMoE | todo | [partial / TDMoE](rules/tdmoe.yaml) |
| P4 | TELKONET | todo | 需核对别名/Fields 入口 |
| P4 | TIBCO Rendezvous | todo | 需核对别名/Fields 入口 |
| P4 | TIPC | todo | [partial / TIPC](rules/tipc.yaml) |
| P4 | TPNCP | todo | 需核对别名/Fields 入口 |
| P4 | TRILL | todo | [partial / TRILL](rules/trill.yaml) |
| P4 | TZSP | todo | [partial / TZSP](rules/application-layer/sample_protocols.yaml) |
| P4 | Teredo | todo | [new / TeredoAuthentication](rules/application-layer/extended_protocols.yaml) |
| P4 | UCP/EMI | todo | [partial / UCPMessage](rules/application-layer/sample_protocols.yaml) |
| P4 | UDP-Lite | todo | [partial / Internet Protocol](rules/internet_protocol.yaml) |
| P4 | ULP | todo | 需核对别名/Fields 入口 |
| P4 | VARAN | todo | 需核对别名/Fields 入口 |
| P4 | VINES | todo | [partial / VINESVIP](rules/vines.yaml) |
| P4 | VNTAG | todo | [partial / VNTag](rules/link_samples.yaml) |
| P4 | WASSP | todo | 需核对别名/Fields 入口 |
| P4 | WINS-Replication | todo | 需核对别名/Fields 入口 |
| P4 | WPS Office sync | todo | 需核对别名/Fields 入口 |
| P4 | WSMP | todo | [partial / WSMP](rules/wsmp.yaml) |
| P4 | WTLS | todo | 需核对别名/Fields 入口 |
| P4 | WoW | todo | 需核对别名/Fields 入口 |
| P4 | X11 | todo | [partial / X11SetupLittle](rules/application-layer/sample_protocols.yaml) |
| P4 | XDMCP | todo | [new / XDMCP](rules/application-layer/extended_protocols.yaml) |
| P4 | XNS | todo | [partial / XNS](rules/xns.yaml) |
| P4 | XOT | todo | 需核对别名/Fields 入口 |
| P4 | XTP | todo | [partial / XTP](rules/transport-layer/xtp.yaml) |
| P4 | XYPLEX | todo | [partial / XYPLEXRequest](rules/xyplex.yaml) |
| P4 | Xbox Live | todo | 需核对别名/Fields 入口 |
| P4 | Xen API | todo | 需核对别名/Fields 入口 |
| P4 | Yahoo Messenger | todo | 需核对别名/Fields 入口 |
| P4 | ZEP | todo | 需核对别名/Fields 入口 |
| P4 | gNMI | todo | 需核对别名/Fields 入口 |
| P4 | swIPe | todo | [partial / SwIPe](rules/swipe.yaml) |

### 已有模型中仍非 stable 的入口

以下每行按规则文件合并入口，全部保持现有 `partial` / `new` 状态，不将有限样本支持升级为完整协议。每个入口的具体未解析字段、上下文和样本依据见 `protocol_catalog.go` 的 `Notes` / `SampleFrom`。这些都是后续协议扩展的起点。

| 规则 | 状态与入口 |
|---|---|
| [aarp.yaml](rules/aarp.yaml) | AARP (`AARP`, partial) |
| [amqp.yaml](rules/amqp.yaml) | AMQP (``, new) |
| [aoe.yaml](rules/aoe.yaml) | AoE (`AoE`, partial) |
| [appletalk.yaml](rules/appletalk.yaml) | AppleTalk (`DDP`, partial) |
| [application-layer/acme.yaml](rules/application-layer/acme.yaml) | ACMEv2 (`ACMEJWSRequest`, partial) |
| [application-layer/ajp.yaml](rules/application-layer/ajp.yaml) | AJP (``, new) |
| [application-layer/alljoyn.yaml](rules/application-layer/alljoyn.yaml) | AllJoyn (`AllJoynNS`, partial) |
| [application-layer/ber.yaml](rules/application-layer/ber.yaml) | BER (``, new) |
| [application-layer/bootp.yaml](rules/application-layer/bootp.yaml) | BOOTP (`BOOTP`, new) |
| [application-layer/browser_mailslot.yaml](rules/application-layer/browser_mailslot.yaml) | Browser Mailslot Datagram (`BrowserMailslotDatagram`, partial) |
| [application-layer/c37118.yaml](rules/application-layer/c37118.yaml) | IEEE C37.118 synchrophasor (`C37118`, partial) |
| [application-layer/cassandra_fields.yaml](rules/application-layer/cassandra_fields.yaml) | CQL Options v4 Fields (`CQLOptions4Fields`, partial)<br>CQL Options v5 Initial Fields (`CQLOptions5InitialFields`, partial)<br>CQL Startup v4 Fields (`CQLStartup4Fields`, partial)<br>CQL Startup v5 Initial Fields (`CQLStartup5InitialFields`, partial)<br>CQL Supported v4 Fields (`CQLSupported4Fields`, partial)<br>CQL Supported v5 Initial Fields (`CQLSupported5InitialFields`, partial)<br>Cassandra Internode Initiate Fields (`CassandraInternodeInitiateFields`, partial) |
| [application-layer/checkmk.yaml](rules/application-layer/checkmk.yaml) | Checkmk (`CheckmkReport`, partial) |
| [application-layer/cifs.yaml](rules/application-layer/cifs.yaml) | CIFS Negotiate Request (`CIFSDirectTCP`, partial) |
| [application-layer/cldap.yaml](rules/application-layer/cldap.yaml) | CLDAP (`CLDAP`, partial) |
| [application-layer/dcerpc.yaml](rules/application-layer/dcerpc.yaml) | DCE/RPC (``, new)<br>DCOM (``, new)<br>DRSUAPI (``, new)<br>LSARPC (``, new)<br>MSRPC (``, new)<br>MSRPC EPM (``, new)<br>NETLOGON (``, new)<br>PsExec/SMB-svcctl (``, new)<br>SAMR (``, new)<br>SRVSVC (``, new)<br>SVCCTL (``, new)<br>WINREG (``, new)<br>WMI (``, new) |
| [application-layer/dhcp.yaml](rules/application-layer/dhcp.yaml) | DHCP (``, new) |
| [application-layer/dicom.yaml](rules/application-layer/dicom.yaml) | DICOM Upper Layer (`DICOM`, partial) |
| [application-layer/dns.yaml](rules/application-layer/dns.yaml) | Bonjour (`DNS`, partial)<br>mDNS (`DNS`, new) |
| [application-layer/docker_api.yaml](rules/application-layer/docker_api.yaml) | Docker API (`DockerContainerListRequest`, partial) |
| [application-layer/doip.yaml](rules/application-layer/doip.yaml) | DoIP (`DoIP`, partial) |
| [application-layer/doq.yaml](rules/application-layer/doq.yaml) | DoQ (`DoQQueryStream`, partial) |
| [application-layer/edonkey.yaml](rules/application-layer/edonkey.yaml) | eMule/ED2K (`EDonkey`, partial) |
| [application-layer/egd.yaml](rules/application-layer/egd.yaml) | GE Ethernet Global Data (`EGD`, partial) |
| [application-layer/elasticsearch.yaml](rules/application-layer/elasticsearch.yaml) | Elasticsearch (`Elasticsearch`, new) |
| [application-layer/etcd.yaml](rules/application-layer/etcd.yaml) | ETCD (`EtcdVersionHTTP`, partial) |
| [application-layer/ether_sbus.yaml](rules/application-layer/ether_sbus.yaml) | Ether-S-Bus (`EtherSBus`, partial) |
| [application-layer/ether_sio.yaml](rules/application-layer/ether_sio.yaml) | Ether-S-I/O (`EtherSIO`, partial) |
| [application-layer/extended_protocols.yaml](rules/application-layer/extended_protocols.yaml) | AFP (`AFP`, new)<br>ANSI C12.22 (`C1222Message`, new)<br>ActiveMQ OpenWire (`ActiveMQOpenWire`, new)<br>BACnet (`BACnetIP`, new)<br>BFD (`BFD`, new)<br>Beckhoff ADS (`BeckhoffADS`, new)<br>CAN/ISO-TP (`CANEthernet`, new)<br>Cassandra CQL (`CassandraCQL`, new)<br>Ceph (`CephConnect`, new)<br>Citrix ICA (`CitrixICA`, new)<br>CoAP (`CoAP`, new)<br>Collectd (`Collectd`, new)<br>DB2 DRDA (`DRDA`, new)<br>DLEP (`DLEPSignal`, new)<br>DLMS/COSEM (`DLMSHDLC`, new)<br>DNP3 (`DNP3`, new)<br>Diameter (`Diameter`, new)<br>EtherNet/IP CIP (`EtherNetIPCIPIO`, new)<br>FIX (`FIX`, new)<br>GLBP (`GLBP`, new)<br>GTP Prime (`GTPPrime`, new)<br>GTP-C (`GTPv2`, new)<br>GTP-U (`GTPv1`, new)<br>GTPv2 (`GTPv2`, partial)<br>Git daemon (`GitDaemon`, new)<br>Gnutella (`Gnutella`, new)<br>Hart-IP (`HartIP`, new)<br>IEC 60870-5-104 (`IEC104`, new)<br>IEEE 802.11 Radiotap (`RadioTapHeader`, new)<br>IPP (`IPP`, new)<br>IRC (`IRC`, new)<br>KNX/IP (`KNXIP`, new)<br>LDP (`LDP`, new)<br>MELSEC (`MELSEC`, new)<br>MGCP (`MGCP`, new)<br>MPEG-TS (`MPEGTransportStream`, new)<br>Modbus TCP (`ModbusTCP`, new)<br>NAT-PMP (`NATPMPPublicAddressRequest`, new)<br>NATS (`NATS`, new)<br>NNTP (`NNTP`, new)<br>NetFlow v9 (`NetFlowV9`, new)<br>OPC UA (`OPCUAHello`, new)<br>Omron FINS (`OmronFINS`, new)<br>PFCP (`PFCP`, new)<br>PIM (`PIM`, new)<br>RSH (`RSH`, new)<br>S7comm (`S7comm`, new)<br>S7comm-plus (`S7commPlus`, new)<br>SCCP/Skinny (`Skinny`, new)<br>SMPP (`SMPPBind`, new)<br>SOME/IP (`SOMEIP`, new)<br>SRVLOC/SLP (`SLPv2`, new)<br>STOMP (`STOMP`, new)<br>Teredo (`TeredoAuthentication`, new)<br>USB HID report (`USBHIDReport`, partial)<br>XDMCP (`XDMCP`, new)<br>sFlow (`SFlowV5`, new) |
| [application-layer/ftp.yaml](rules/application-layer/ftp.yaml) | FTP (``, new)<br>FTPS (`FTPAuthTLS`, new) |
| [application-layer/gquic.yaml](rules/application-layer/gquic.yaml) | GQUIC Q035 (`GQUIC35ClientPacket`, partial) |
| [application-layer/gssapi.yaml](rules/application-layer/gssapi.yaml) | GSS-API (`GSSAPIHTTP`, partial) |
| [application-layer/h225.yaml](rules/application-layer/h225.yaml) | H.225 (`H225RAS`, partial)<br>H.323 (`H225Call`, partial) |
| [application-layer/hislip.yaml](rules/application-layer/hislip.yaml) | HiSLIP (`HiSLIP`, partial) |
| [application-layer/hl7.yaml](rules/application-layer/hl7.yaml) | HL7 v2 MLLP (`HL7Message`, partial) |
| [application-layer/http.yaml](rules/application-layer/http.yaml) | HTTP (``, new)<br>HTTP Proxy CONNECT (`HTTP`, new)<br>SOAP (`HTTP`, partial)<br>WebDAV (`HTTP`, partial)<br>Yakit proxy framing (``, new) |
| [application-layer/http2.yaml](rules/application-layer/http2.yaml) | HTTP/2 (``, new) |
| [application-layer/http2_fields.yaml](rules/application-layer/http2_fields.yaml) | HTTP/2 Frame Fields (`HTTP2FrameFields`, partial)<br>HTTP/2 Initial Client Direction (`HTTP2InitialClientStream`, partial)<br>HTTP/2 Initial Server Direction (`HTTP2InitialServerStream`, partial) |
| [application-layer/http3.yaml](rules/application-layer/http3.yaml) | HTTP/3 (`HTTP3RequestStream`, partial) |
| [application-layer/iax2.yaml](rules/application-layer/iax2.yaml) | IAX2 (`IAX2`, partial) |
| [application-layer/iiop.yaml](rules/application-layer/iiop.yaml) | IIOP (`GIOP`, partial)<br>IIOP Locate (`GIOP`, partial)<br>IIOP/GIOP (`GIOP`, partial) |
| [application-layer/imap_fields.yaml](rules/application-layer/imap_fields.yaml) | IMAP Command Fields (`IMAPCommandFields`, partial)<br>IMAP Response Block Fields (`IMAPResponseBlockFields`, partial)<br>IMAP Response Fields (`IMAPResponseFields`, partial) |
| [application-layer/ipfix.yaml](rules/application-layer/ipfix.yaml) | IPFIX (`IPFIX`, partial) |
| [application-layer/isotp.yaml](rules/application-layer/isotp.yaml) | COTP (`COTP`, partial)<br>TPKT (`TPKT`, partial) |
| [application-layer/java_ser.yaml](rules/application-layer/java_ser.yaml) | Java serialization (``, new) |
| [application-layer/jsonrpc_v2.yaml](rules/application-layer/jsonrpc_v2.yaml) | JSON-RPC 2.0 (`JSONRPC2`, partial) |
| [application-layer/kcp.yaml](rules/application-layer/kcp.yaml) | KCP (`KCPDatagram`, partial) |
| [application-layer/kerberos.yaml](rules/application-layer/kerberos.yaml) | Kerberos (``, new)<br>Kerberos PAC (``, new) |
| [application-layer/kerberos_fields.yaml](rules/application-layer/kerberos_fields.yaml) | Kerberos Message Fields (`KerberosMessageFields`, partial)<br>Kerberos TCP Fields (`KerberosTCPFields`, partial) |
| [application-layer/kubernetes_api.yaml](rules/application-layer/kubernetes_api.yaml) | Kubernetes API (`KubernetesAPIRequest`, partial) |
| [application-layer/ldap.yaml](rules/application-layer/ldap.yaml) | LDAP (``, partial) |
| [application-layer/ldap_fields.yaml](rules/application-layer/ldap_fields.yaml) | LDAP Bind Request Fields (`LDAPBindRequestFields`, partial) |
| [application-layer/lpd.yaml](rules/application-layer/lpd.yaml) | LPD (`LPD`, partial) |
| [application-layer/megaco.yaml](rules/application-layer/megaco.yaml) | Megaco/H.248 (`Megaco`, partial) |
| [application-layer/memcached_fields.yaml](rules/application-layer/memcached_fields.yaml) | Memcached Binary GET Request Fields (`MemcachedBinaryGetRequestFields`, partial)<br>Memcached Stats Request Fields (`MemcachedStatsRequestFields`, partial)<br>Memcached Stats Response Fields (`MemcachedStatsResponseFields`, partial) |
| [application-layer/minio_s3.yaml](rules/application-layer/minio_s3.yaml) | MinIO/S3 (`S3SignatureV4Request`, partial) |
| [application-layer/mms.yaml](rules/application-layer/mms.yaml) | IEC 61850 MMS (`MMSPDU`, partial) |
| [application-layer/mqtt.yaml](rules/application-layer/mqtt.yaml) | MQTT (``, new) |
| [application-layer/mqtt_fields.yaml](rules/application-layer/mqtt_fields.yaml) | MQTT 3.1 Packet Fields (`MQTT31PacketFields`, partial)<br>MQTT 3.1.1 Packet Fields (`MQTT311PacketFields`, partial) |
| [application-layer/msrdp.yaml](rules/application-layer/msrdp.yaml) | MSRdp (``, new)<br>RDP (`RDPConnectionRequest`, new) |
| [application-layer/mysql.yaml](rules/application-layer/mysql.yaml) | MariaDB (`MySQLPacket`, partial)<br>MySQL (``, partial) |
| [application-layer/mysql_fields.yaml](rules/application-layer/mysql_fields.yaml) | MariaDB Greeting Fields (`MySQLGreetingFields`, partial)<br>MariaDB Handshake Response Fields (`MariaDBHandshakeResponse41Fields`, partial)<br>MariaDB Text Result Set Fields (`MariaDBTextResultSetFields`, partial)<br>MySQL Command Fields (`MySQLCommandFields`, partial)<br>MySQL Greeting Fields (`MySQLGreetingFields`, partial)<br>MySQL OK Session Track Fields (`MySQLOKSessionTrackFields`, partial)<br>MySQL SSL Request Fields (`MySQLSSLRequestFields`, partial) |
| [application-layer/nbns.yaml](rules/application-layer/nbns.yaml) | LLMNR (`LLMNR`, partial)<br>LLMNR response (``, new)<br>LLMNR-MDNS collision (`LLMNR`, partial)<br>NBNS (``, new)<br>NBT NS (``, new)<br>NBT-NS response (``, new) |
| [application-layer/nbss.yaml](rules/application-layer/nbss.yaml) | NBT SS (``, new)<br>NetBIOS (``, new) |
| [application-layer/ncp.yaml](rules/application-layer/ncp.yaml) | NCP (`NCP`, partial) |
| [application-layer/netflow_v5.yaml](rules/application-layer/netflow_v5.yaml) | NetFlow v5 (`NetFlowV5`, partial) |
| [application-layer/ntlm.yaml](rules/application-layer/ntlm.yaml) | NTLM (``, new)<br>NTLM v1/v2 (`NTLMSSP`, new)<br>NTLMSSP (`NTLMSSP`, new)<br>NetNTLMv2 (``, new) |
| [application-layer/ntp.yaml](rules/application-layer/ntp.yaml) | NTP (``, new)<br>SNTP (`NTP`, partial) |
| [application-layer/nvme_tcp.yaml](rules/application-layer/nvme_tcp.yaml) | NVMe-oF (`NVMeTCP`, partial) |
| [application-layer/observed_protocols.yaml](rules/application-layer/observed_protocols.yaml) | RSVP (`RSVP`, partial) |
| [application-layer/ocsp.yaml](rules/application-layer/ocsp.yaml) | OCSP (`OCSPRequest`, new) |
| [application-layer/pop3_fields.yaml](rules/application-layer/pop3_fields.yaml) | POP3 Capabilities Fields (`POP3CapabilitiesFields`, partial)<br>POP3 Challenge Fields (`POP3ChallengeFields`, partial)<br>POP3 Client Continuation Fields (`POP3ClientContinuationFields`, partial)<br>POP3 Command Fields (`POP3CommandFields`, partial)<br>POP3 List Fields (`POP3ListFields`, partial)<br>POP3 Stat Fields (`POP3StatFields`, partial)<br>POP3 Status Fields (`POP3StatusFields`, partial)<br>POP3 UIDL Fields (`POP3UIDLFields`, partial) |
| [application-layer/postgresql.yaml](rules/application-layer/postgresql.yaml) | PostgreSQL (``, partial) |
| [application-layer/postgresql_fields.yaml](rules/application-layer/postgresql_fields.yaml) | PostgreSQL Backend Block Fields (`PostgreSQLBackendBlockFields`, partial)<br>PostgreSQL Backend SCRAM Fields (`PostgreSQLBackendSCRAMFields`, partial)<br>PostgreSQL Frontend Fields (`PostgreSQLFrontendFields`, partial)<br>PostgreSQL GSS Request Fields (`PostgreSQLGSSRequestFields`, partial)<br>PostgreSQL GSS Response Fields (`PostgreSQLGSSResponseFields`, partial)<br>PostgreSQL Password Fields (`PostgreSQLPasswordFields`, partial)<br>PostgreSQL SASL Initial Fields (`PostgreSQLSASLInitialFields`, partial)<br>PostgreSQL SCRAM Response Fields (`PostgreSQLSASLSCRAMResponseFields`, partial)<br>PostgreSQL SSL Request Fields (`PostgreSQLSSLRequestFields`, partial)<br>PostgreSQL SSL Response Fields (`PostgreSQLSSLResponseFields`, partial)<br>PostgreSQL Startup Fields (`PostgreSQLStartupFields`, partial) |
| [application-layer/pptp.yaml](rules/application-layer/pptp.yaml) | PPTP (``, new) |
| [application-layer/profinet_io.yaml](rules/application-layer/profinet_io.yaml) | Profinet IO (`ProfinetIOReadImplicit`, new) |
| [application-layer/prometheus.yaml](rules/application-layer/prometheus.yaml) | Prometheus exposition (`PrometheusExposition`, partial) |
| [application-layer/quic.yaml](rules/application-layer/quic.yaml) | QUIC (``, new) |
| [application-layer/radius.yaml](rules/application-layer/radius.yaml) | RADIUS (``, new) |
| [application-layer/redfish.yaml](rules/application-layer/redfish.yaml) | Redfish (`RedfishServiceRootRequest`, partial) |
| [application-layer/redfish_ssdp.yaml](rules/application-layer/redfish_ssdp.yaml) | Redfish SSDP (`RedfishSSDP`, partial) |
| [application-layer/redis.yaml](rules/application-layer/redis.yaml) | Redis (``, new) |
| [application-layer/sample_protocols.yaml](rules/application-layer/sample_protocols.yaml) | 9P (`NinePVersion`, partial)<br>CMPP (`CMPPConnect`, partial)<br>MSDP (`MSDPMessage`, partial)<br>OLSR (`OLSRPacket`, partial)<br>Quake (`QuakeControl`, partial)<br>Quake2 (`QuakeConnectionless`, partial)<br>Quake3 (`QuakeConnectionless`, partial)<br>Rlogin (`RloginRequest`, partial)<br>TZSP (`TZSP`, partial)<br>UCP/EMI (`UCPMessage`, partial)<br>X11 (`X11SetupLittle`, partial)<br>iSCSI (`ISCSIBasicHeader`, partial) |
| [application-layer/sebek.yaml](rules/application-layer/sebek.yaml) | SEBEK (`SEBEK`, partial) |
| [application-layer/sigcomp.yaml](rules/application-layer/sigcomp.yaml) | SIGCOMP (`SIGCOMP`, partial) |
| [application-layer/slimp3.yaml](rules/application-layer/slimp3.yaml) | SliMP3 (`SliMP3`, partial) |
| [application-layer/smb.yaml](rules/application-layer/smb.yaml) | CIFS (`SMB`, new)<br>SMB (`SMB`, new) |
| [application-layer/smb2.yaml](rules/application-layer/smb2.yaml) | SMB2 (``, new) |
| [application-layer/smb3.yaml](rules/application-layer/smb3.yaml) | SMB3 (`SMB3Negotiate`, partial) |
| [application-layer/smtp.yaml](rules/application-layer/smtp.yaml) | SMTP (``, new)<br>Submission (`SMTPCommand`, partial) |
| [application-layer/smtp_fields.yaml](rules/application-layer/smtp_fields.yaml) | SMTP Command Fields (`SMTPCommandFields`, partial) |
| [application-layer/smtp_reply.yaml](rules/application-layer/smtp_reply.yaml) | SMTP Reply (`SMTPReply`, partial) |
| [application-layer/snmp.yaml](rules/application-layer/snmp.yaml) | SNMP (``, new) |
| [application-layer/snmpv3.yaml](rules/application-layer/snmpv3.yaml) | SNMPv3 (`SNMPv3`, partial) |
| [application-layer/spnego.yaml](rules/application-layer/spnego.yaml) | SPNEGO (``, new) |
| [application-layer/ssh.yaml](rules/application-layer/ssh.yaml) | SSH (``, new) |
| [application-layer/ssh_plaintext.yaml](rules/application-layer/ssh_plaintext.yaml) | SSH DH Group Exchange (`SSHPlaintextDHGEX`, partial)<br>SSH Fixed Group DH (`SSHPlaintextDH`, partial)<br>SSH Initial Plaintext Packet (`SSHPlaintextPacket`, partial)<br>SSH P-256 ECDH (`SSHPlaintextECDHP256`, partial) |
| [application-layer/steam_discovery.yaml](rules/application-layer/steam_discovery.yaml) | Steam (`SteamDiscovery`, partial) |
| [application-layer/t3.yaml](rules/application-layer/t3.yaml) | T3 (``, new) |
| [application-layer/t38_sdp.yaml](rules/application-layer/t38_sdp.yaml) | T.38 (`T38H248SDPAdvertisement`, partial) |
| [application-layer/tds.yaml](rules/application-layer/tds.yaml) | MSSQL TDS (``, new) |
| [application-layer/tds_fields.yaml](rules/application-layer/tds_fields.yaml) | TDS RPC 7.1 Fields (`TDSRPC71Fields`, partial)<br>TDS RPC 7.2 Fields (`TDSRPC72Fields`, partial)<br>TDS Response 7.1 Fields (`TDSResponse71Fields`, partial)<br>TDS Response 7.2 Fields (`TDSResponse72Fields`, partial)<br>TDS SQL Batch 7.1 Fields (`TDSBatch71Fields`, partial)<br>TDS SQL Batch 7.2 Fields (`TDSBatch72Fields`, partial) |
| [application-layer/teamviewer.yaml](rules/application-layer/teamviewer.yaml) | TeamViewer (`TeamViewerRecord`, partial) |
| [application-layer/tls.yaml](rules/application-layer/tls.yaml) | AnyDesk (``, partial)<br>DingTalk (``, partial)<br>DoH (``, partial)<br>DoT (``, partial)<br>IMAPS (``, partial)<br>SMTPS (``, partial)<br>SSL (``, partial)<br>TLS (``, new)<br>WeChat/MicroMsg (``, partial) |
| [application-layer/tls_certificate.yaml](rules/application-layer/tls_certificate.yaml) | TLS 1.2 Certificate (`TLSHandshakeCertificate`, partial) |
| [application-layer/tls_certificate_auth.yaml](rules/application-layer/tls_certificate_auth.yaml) | TLS 1.2 CertificateRequest (`TLS12CertificateRequest`, partial)<br>TLS 1.2 CertificateVerify (`TLS12CertificateVerify`, partial) |
| [application-layer/tls_change_cipher_spec.yaml](rules/application-layer/tls_change_cipher_spec.yaml) | TLS ChangeCipherSpec (`TLSChangeCipherSpecRecord`, partial) |
| [application-layer/tls_control_handshake.yaml](rules/application-layer/tls_control_handshake.yaml) | TLS 1.2 NewSessionTicket (`TLS12NewSessionTicket`, partial)<br>TLS 1.2 ServerHelloDone (`TLS12ServerHelloDone`, partial) |
| [application-layer/tls_hello.yaml](rules/application-layer/tls_hello.yaml) | JA3/JA4 (``, new) |
| [application-layer/tls_key_exchange.yaml](rules/application-layer/tls_key_exchange.yaml) | TLS 1.2 ClientKeyExchange (`TLS12ECDHEClientKeyExchange`, partial)<br>TLS 1.2 ServerKeyExchange (`TLS12ECDHEServerKeyExchange`, partial) |
| [application-layer/tls_server_hello.yaml](rules/application-layer/tls_server_hello.yaml) | TLS ServerHello (`TLSServerHelloRecord`, partial) |
| [application-layer/tns.yaml](rules/application-layer/tns.yaml) | Oracle TNS (``, new)<br>TNS (``, new) |
| [application-layer/tns_fields.yaml](rules/application-layer/tns_fields.yaml) | TNS Accept 315 Fields (`TNSAccept315Fields`, partial)<br>TNS Connect 315 Fields (`TNSConnect315Fields`, partial)<br>TNS Parameters Native LE64 32 Fields (`TNSParametersNativeLE6432Fields`, partial)<br>TNS Protocol Request 32 Fields (`TNSProtocolRequest32Fields`, partial)<br>TNS Protocol Response 32 Fields (`TNSProtocolResponse32Fields`, partial)<br>TNS Resend 16 Fields (`TNSResend16Fields`, partial)<br>TNS Services 32 Fields (`TNSServices32Fields`, partial)<br>TNS Types Request Native 32 Fields (`TNSTypesRequestNative32Fields`, partial)<br>TNS Types Response Native 32 Fields (`TNSTypesResponseNative32Fields`, partial) |
| [application-layer/trdp.yaml](rules/application-layer/trdp.yaml) | TRDP (`TRDPMessage`, partial) |
| [application-layer/upnp.yaml](rules/application-layer/upnp.yaml) | SSDP (`UPnPSSDPNotify`, partial)<br>UPnP (`UPnPSSDPNotify`, new) |
| [application-layer/websocket.yaml](rules/application-layer/websocket.yaml) | WebSocket (``, new) |
| [application-layer/winrm.yaml](rules/application-layer/winrm.yaml) | WinRM HTTP (`WinRMHTTP`, partial) |
| [application-layer/wpad.yaml](rules/application-layer/wpad.yaml) | WPAD (`WPADRequest`, partial)<br>WPAD proxy (`WPADRequest`, partial) |
| [application-layer/ws_discovery.yaml](rules/application-layer/ws_discovery.yaml) | WS-Discovery (`WSDiscovery`, partial) |
| [application-layer/x509_certificate.yaml](rules/application-layer/x509_certificate.yaml) | X.509 Certificate DER (`X509CertificateDER`, partial)<br>X.509 Certificate DER with Extensions (`X509CertificateDERWithExtensions`, partial)<br>X.509 Certificate DER with Public Key (`X509CertificateDERWithPublicKey`, partial) |
| [application-layer/xmlrpc.yaml](rules/application-layer/xmlrpc.yaml) | XML-RPC (`XMLRPC`, partial) |
| [application-layer/xmpp.yaml](rules/application-layer/xmpp.yaml) | XMPP (`XMPP`, partial) |
| [application-layer/zlib_json.yaml](rules/application-layer/zlib_json.yaml) | Length-prefixed zlib JSON (`ZlibJSONRecord`, partial) |
| [bgp.yaml](rules/bgp.yaml) | BGP (``, new) |
| [bittorrent.yaml](rules/bittorrent.yaml) | BitTorrent (``, new) |
| [cdp.yaml](rules/cdp.yaml) | CDP (``, new) |
| [cfm.yaml](rules/cfm.yaml) | CFM (`CFM`, partial) |
| [dccp.yaml](rules/dccp.yaml) | DCCP (`DCCP`, partial) |
| [decnet.yaml](rules/decnet.yaml) | DECnet (`DECnet`, partial) |
| [dhcpv6.yaml](rules/dhcpv6.yaml) | DHCPv6 (``, new)<br>DHCPv6 server exchange (`DHCPv6`, partial) |
| [dtls.yaml](rules/dtls.yaml) | DTLS (``, new) |
| [dvmrp.yaml](rules/dvmrp.yaml) | DVMRP (`DVMRP`, partial) |
| [eapol.yaml](rules/eapol.yaml) | EAP (`EAPOL`, partial)<br>EAPOL (``, new)<br>IEEE 802.1X (`EAPOL`, partial) |
| [eigrp.yaml](rules/eigrp.yaml) | EIGRP (``, new) |
| [ethercat.yaml](rules/ethercat.yaml) | EtherCAT (`EtherCAT`, partial) |
| [ethernet.yaml](rules/ethernet.yaml) | Ethernet 802.3 (`Ethernet`, partial)<br>Ethernet SNAP (`Ethernet`, partial) |
| [fastcgi.yaml](rules/fastcgi.yaml) | FastCGI (``, new) |
| [fcoe.yaml](rules/fcoe.yaml) | FCoE (`FCoE`, partial) |
| [fibre_channel.yaml](rules/fibre_channel.yaml) | Fibre Channel (`FibreChannel`, partial) |
| [ftp_data.yaml](rules/ftp_data.yaml) | FTP-DATA (``, new) |
| [geneve.yaml](rules/geneve.yaml) | Geneve (`Geneve`, partial) |
| [hessian.yaml](rules/hessian.yaml) | Hessian2 (``, new) |
| [hsrp.yaml](rules/hsrp.yaml) | HSRP (``, new) |
| [iec61850.yaml](rules/iec61850.yaml) | IEC 61850 GOOSE (`GOOSE`, partial)<br>IEC 61850 SV (`SampledValues`, partial) |
| [ieee_802_11.yaml](rules/ieee_802_11.yaml) | IEEE 802.11 (``, new) |
| [ieee_802_1ad.yaml](rules/ieee_802_1ad.yaml) | IEEE 802.1ad QinQ (``, new) |
| [ieee_802_1q.yaml](rules/ieee_802_1q.yaml) | IEEE 802.1Q (``, new) |
| [igmp.yaml](rules/igmp.yaml) | IGMP (``, new) |
| [igrp.yaml](rules/igrp.yaml) | IGRP (`IGRP`, partial) |
| [ike.yaml](rules/ike.yaml) | IKEv1 (`IKE`, new)<br>IKEv2 (``, new) |
| [imap.yaml](rules/imap.yaml) | IMAP (``, new) |
| [internet_control_message_protocol.yaml](rules/internet_control_message_protocol.yaml) | ICMP (``, new)<br>ICMP Timestamp (`ICMP`, partial) |
| [internet_control_message_protocol_v6.yaml](rules/internet_control_message_protocol_v6.yaml) | ICMPv6 (``, new)<br>ICMPv6 MLD (`ICMPV6`, partial)<br>ICMPv6 NDP (`ICMPV6`, partial)<br>IPv6 RA (`ICMPV6`, partial) |
| [internet_protocol.yaml](rules/internet_protocol.yaml) | 6in4 (`Internet Protocol`, new)<br>6to4 (`Internet Protocol`, partial)<br>IPIP (`Internet Protocol`, partial)<br>UDP-Lite (`Internet Protocol`, partial) |
| [internet_protocol_version_6.yaml](rules/internet_protocol_version_6.yaml) | IPv6 Destination Options (`Internet Protocol Version 6`, partial)<br>IPv6 Fragment (`Internet Protocol Version 6`, partial)<br>IPv6 Hop-by-Hop (`Internet Protocol Version 6`, partial)<br>IPv6 Routing Header (`Internet Protocol Version 6`, partial) |
| [ipmi.yaml](rules/ipmi.yaml) | IPMI (``, new)<br>IPMI RMCP+ (`IPMI`, partial) |
| [ipsec.yaml](rules/ipsec.yaml) | IPsec AH (``, new) |
| [ipx.yaml](rules/ipx.yaml) | IPX (`IPX`, partial) |
| [isis.yaml](rules/isis.yaml) | IS-IS (`ISIS`, partial) |
| [j1939.yaml](rules/j1939.yaml) | J1939 (`J1939`, partial) |
| [jdwp.yaml](rules/jdwp.yaml) | JDWP (`JDWP`, partial)<br>JDWP handshake (`JDWP`, partial) |
| [jenkins.yaml](rules/jenkins.yaml) | Jenkins remoting (``, new) |
| [jsonrpc.yaml](rules/jsonrpc.yaml) | JSON-RPC (``, partial) |
| [kafka.yaml](rules/kafka.yaml) | Kafka (``, new) |
| [l2tp.yaml](rules/l2tp.yaml) | L2TP (``, new) |
| [lat.yaml](rules/lat.yaml) | LAT (`LAT`, partial) |
| [link_aggregation.yaml](rules/link_aggregation.yaml) | LACP (`LACP`, partial) |
| [link_samples.yaml](rules/link_samples.yaml) | HomePlug AV (`HomePlugAV`, partial)<br>IEEE 802.1ah PBB (`ProviderBackboneBridge`, partial)<br>IEEE 802.3 Slow Protocols (`SlowProtocols`, partial)<br>LACP-marker (`LACPMarker`, partial)<br>VNTAG (`VNTag`, partial) |
| [linux_sll.yaml](rules/linux_sll.yaml) | Linux SLL (``, new) |
| [linux_sll2.yaml](rules/linux_sll2.yaml) | Linux SLL2 (`LinuxSLL2`, partial) |
| [llc.yaml](rules/llc.yaml) | Ethernet 802.2 (`LLC`, partial)<br>LLC (``, new)<br>SNAP (`LLC`, partial) |
| [lldp.yaml](rules/lldp.yaml) | LLDP (``, new) |
| [loopback.yaml](rules/loopback.yaml) | Loopback (``, new) |
| [macsec.yaml](rules/macsec.yaml) | MACSec (`MACSec`, partial) |
| [management_samples.yaml](rules/management_samples.yaml) | E-LMI (`ELMI`, partial)<br>EOAM (`EOAM`, partial)<br>MRP-MMRP (`MMRP`, partial)<br>MRP-MSRP (`MSRP`, partial)<br>MRP-MVRP (`MVRP`, partial) |
| [memcached.yaml](rules/memcached.yaml) | Memcache binary (`Memcached`, partial)<br>Memcached (``, new) |
| [mobility_samples.yaml](rules/mobility_samples.yaml) | MIPv6 (`MIPv6Packet`, partial)<br>Mobile IP (`MobileIP`, partial) |
| [mongodb.yaml](rules/mongodb.yaml) | MongoDB (``, new) |
| [mpls.yaml](rules/mpls.yaml) | MPLS (``, new) |
| [nat_t.yaml](rules/nat_t.yaml) | NAT-T (`NATT`, partial) |
| [nbt_dg.yaml](rules/nbt_dg.yaml) | NBT DG (``, new) |
| [net_remoting.yaml](rules/net_remoting.yaml) | .NET Remoting (``, new) |
| [netbios_frame.yaml](rules/netbios_frame.yaml) | NetBEUI (`NetBIOSFrame`, partial) |
| [network_samples.yaml](rules/network_samples.yaml) | IPcomp (`IPComp`, partial)<br>MPLS PW (`MPLSEthernetPW`, partial)<br>NVGRE (`NVGRE`, partial) |
| [nhrp.yaml](rules/nhrp.yaml) | NHRP (`NHRP`, partial) |
| [onc_rpc.yaml](rules/onc_rpc.yaml) | Mount (`ONCRPC`, partial)<br>NFS (`ONCRPC`, new)<br>ONC RPC (``, new)<br>Portmap/Rpcbind (`ONCRPC`, partial)<br>RPC (`ONCRPC`, partial) |
| [openvpn.yaml](rules/openvpn.yaml) | OpenVPN (``, new) |
| [ospf.yaml](rules/ospf.yaml) | OSPF (``, new) |
| [ospfv3.yaml](rules/ospfv3.yaml) | OSPFv3 (`OSPFv3`, partial) |
| [php_ser.yaml](rules/php_ser.yaml) | PHP serialize (``, new) |
| [pickle.yaml](rules/pickle.yaml) | Python pickle (``, new) |
| [pop3.yaml](rules/pop3.yaml) | POP3 (``, new) |
| [powerlink.yaml](rules/powerlink.yaml) | Powerlink (`Powerlink`, partial) |
| [pppoe.yaml](rules/pppoe.yaml) | PPPoE Discovery (``, new)<br>PPPoE Session (`PPPoE`, partial) |
| [profinet_dcp.yaml](rules/profinet_dcp.yaml) | Profinet DCP (`ProfinetDCP`, partial) |
| [protobuf.yaml](rules/protobuf.yaml) | Protobuf (``, new) |
| [ptp.yaml](rules/ptp.yaml) | PTP (`PTP`, new) |
| [rarp.yaml](rules/rarp.yaml) | RARP (`RARPFrame`, partial) |
| [rip.yaml](rules/rip.yaml) | RIP (``, new) |
| [ripng.yaml](rules/ripng.yaml) | RIPng (`RIPng`, partial) |
| [rmi.yaml](rules/rmi.yaml) | RMI (`RMIHeader`, partial)<br>RMI/JRMP (`RMIHeader`, partial) |
| [rsync.yaml](rules/rsync.yaml) | Rsync (`Rsync`, partial)<br>Rsync daemon (``, new) |
| [rtmp.yaml](rules/rtmp.yaml) | RTMP (``, new) |
| [rtp.yaml](rules/rtp.yaml) | RTCP (`RTCP`, new)<br>RTP (``, new) |
| [rtsp.yaml](rules/rtsp.yaml) | RTSP (``, new) |
| [salt.yaml](rules/salt.yaml) | SaltStack (``, new) |
| [sctp.yaml](rules/sctp.yaml) | SCTP (``, new) |
| [sdp.yaml](rules/sdp.yaml) | SDP (`SDP`, partial) |
| [sip.yaml](rules/sip.yaml) | SIP (``, new) |
| [sna.yaml](rules/sna.yaml) | SNA (`SNAEthernet`, partial) |
| [socks4.yaml](rules/socks4.yaml) | SOCKS4 (``, new) |
| [stp.yaml](rules/stp.yaml) | RSTP (`STP`, partial)<br>STP (``, new) |
| [stun.yaml](rules/stun.yaml) | STUN (``, new) |
| [swipe.yaml](rules/swipe.yaml) | swIPe (`SwIPe`, partial) |
| [syslog.yaml](rules/syslog.yaml) | Syslog (``, new) |
| [tacacs.yaml](rules/tacacs.yaml) | TACACS+ (`TACACS`, partial) |
| [tdmoe.yaml](rules/tdmoe.yaml) | TDMoE (`TDMoE`, partial) |
| [telnet.yaml](rules/telnet.yaml) | Telnet (``, new) |
| [tftp.yaml](rules/tftp.yaml) | TFTP (``, new) |
| [thrift.yaml](rules/thrift.yaml) | Thrift (``, new) |
| [tipc.yaml](rules/tipc.yaml) | TIPC (`TIPC`, partial) |
| [transport-layer/xtp.yaml](rules/transport-layer/xtp.yaml) | XTP (`XTP`, partial) |
| [trill.yaml](rules/trill.yaml) | TRILL (`TRILL`, partial) |
| [usb_pcap.yaml](rules/usb_pcap.yaml) | USBPcap (`USBPcap`, partial) |
| [vines.yaml](rules/vines.yaml) | VINES (`VINESVIP`, partial) |
| [vnc.yaml](rules/vnc.yaml) | VNC/RFB (``, new) |
| [vrrp.yaml](rules/vrrp.yaml) | VRRP (``, new) |
| [vxlan.yaml](rules/vxlan.yaml) | VXLAN (``, new) |
| [wep.yaml](rules/wep.yaml) | WEP (`WEP`, partial) |
| [wireguard.yaml](rules/wireguard.yaml) | WireGuard (``, new) |
| [wsmp.yaml](rules/wsmp.yaml) | WSMP (`WSMP`, partial) |
| [xns.yaml](rules/xns.yaml) | XNS (`XNS`, partial) |
| [xyplex.yaml](rules/xyplex.yaml) | XYPLEX (`XYPLEXRequest`, partial) |
| [zabbix.yaml](rules/zabbix.yaml) | Zabbix (`Zabbix`, partial)<br>Zabbix agent (``, new) |
| [zigbee.yaml](rules/zigbee.yaml) | Zigbee (`Zigbee`, partial) |

<!-- END GENERATED PROTOCOL INVENTORY -->

## 每个协议的完成条件

1. 写清版本、方向、PDU 和必要的协商上下文；新增 YAML 或复用已有明确入口。
2. 用真实/可追溯样本断言具名字段；覆盖截断、非法长度、未知类型、可选字段和资源上限。
3. 验证原 Node 路径与结构化计划的完整字段、类型、元数据和错误行为；结果可独立持有。
4. 接入实时 binding 时同时交付 probe、framer、双向状态，以及 TCP 拆包/粘包、重传/缺口、
   端口复用、UDP 边界与未知流量测试。仅提供 YAML 不能宣称实时入口已完成。
5. 同输入衡量 CPU 时间、完整解码字节与错误计数，仅测 1/2/4 worker；不把跳过的字段、
   未识别数据或 JSON 开销混入核心带宽收益。依照 [交付标准](PROTOCOL_DELIVERY.md)
   更新目录、路线图、评分和此 TODO。
6. 更新规则后执行 `go generate ./common/bin-parser/rules`，保证压缩归档与 YAML 同步。

基础设施仍需后续验证：持续高负载下写盘与分析隔离、DNS-heavy 的 UDP 调度、长时间
物理网卡稳定性和捕获延迟统计。这些不替代上面的协议实现任务，也不阻止逐协议交付。
