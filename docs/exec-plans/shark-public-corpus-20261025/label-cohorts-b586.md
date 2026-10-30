逐帧 TOP30 标签分布，实际生产者 `b586dda3c8eb3a3a12f6e2f4e9eaae0441481617`。

每行以 Wireshark 的原始列标签分组，列出同一批原始帧在 Shark 中的真实标签分布。没有合并别名；名称、加密可见性、载体和拒绝策略的差异需要逐帧核实，不能直接算成误识别或已支持。

参考采用新的隔离个人配置、无名称解析及显式 TCP 乱序重组，没有显式 keylog；Shark 保留原生产参数。字段、身份、流和会话的正确性不由这个计数表证明。原包、keys、完整逐帧矩阵均在仓库外。

## sharon

物理帧 328,127；原生消息 263,112；非物理结束发布 552。

| 排名 | Wireshark 原始标签 | 帧数 | 同一批帧的 Shark 原始标签 |
|---:|---|---:|---|
| 1 | IPv4 | 26,588 | Fragment 19,871；IPv4 6,662；GRE 25；UDP 20；IPv6Routing 5；IPv6Fragment 2；IGMP 1；SCTP 1；TCP 1 |
| 2 | TCP | 19,371 | Fragment 7,620；TCP 5,431；IPv4 3,478；UDP 2,242；RTSP 176；TLSv1.3 119；Syslog 91；IPP 65；TLSv1.2 28；GRE 23；HTTP/1.1 18；BGP 16；LDP 16；DNS 12；SMB2 11；IMAP 10；TLS 7；SSH 5；HTTP/1.0 3 |
| 3 | GSMTAP | 17,864 | UDP 17,864 |
| 4 | BACnet-APDU | 10,607 | IPv4 6,090；UDP 4,274；GRE 224；Fragment 18；MPLS 1 |
| 5 | 802.11 | 10,527 | IPv4 5,757；UDP 3,711；GRE 838；Fragment 220；MPLS 1 |
| 6 | NBAP | 7,875 | SCTP 3,406；UDP 2,609；IPv4 1,767；Fragment 93 |
| 7 | F1AP | 7,708 | SCTP 3,699；UDP 2,694；IPv4 1,286；Fragment 29 |
| 8 | RTP | 6,492 | RTP 6,462；UDP 30 |
| 9 | SCCP (Int. ITU) | 6,310 | Fragment 5,831；UDP 386；IPv4 41；TCP 39；SCTP 13 |
| 10 | UDP | 5,998 | UDP 5,975；TFTP 10；Fragment 5；SSDP 5；IPv4 3 |
| 11 | X2AP | 5,534 | SCTP 3,304；UDP 1,132；IPv4 1,070；Fragment 28 |
| 12 | ICMPv6 | 4,846 | ICMPv6 2,958；IPv4 1,069；UDP 692；MLDv2 91；Fragment 29；MLD 3；GRE 2；MLDv1 2 |
| 13 | S1AP | 4,735 | SCTP 2,418；IPv4 1,183；UDP 995；Fragment 139 |
| 14 | GTP | 4,352 | UDP 3,129；Fragment 924；IPv4 292；TCP 7 |
| 15 | XnAP | 4,300 | SCTP 2,030；UDP 1,667；IPv4 600；Fragment 3 |
| 16 | NGAP | 4,171 | UDP 1,620；SCTP 1,593；IPv4 937；Fragment 21 |
| 17 | ZigBee  | 4,113 | UDP 2,145；Fragment 1,963；IPv4 3；TCP 2 |
| 18 | GTPv2 | 3,302 | UDP 2,225；Fragment 679；IPv4 395；TCP 3 |
| 19 | UMA | 3,229 | Fragment 1,859；IPv4 1,336；UDP 29；TCP 5 |
| 20 | SCTP | 3,215 | UDP 1,635；SCTP 781；IPv4 775；Fragment 24 |
| 21 | WSP | 3,065 | UDP 3,065 |
| 22 | ICMP | 2,823 | ICMP 2,499；IPv4 192；UDP 110；Fragment 21；GRE 1 |
| 23 | STP | 2,720 | RPVST+ 2,205；RSTP 407；IPv4 50；GRE 31；UDP 27 |
| 24 | RSVP | 2,628 | IPv4 2,405；UDP 187；Fragment 30；GRE 5；IPv6Routing 1 |
| 25 | DNS | 2,574 | DNS 1,284；UDP 725；IPv4 391；Fragment 160；GRE 8；SCTP 3；TCP 3 |
| 26 | LTP | 2,504 | UDP 1,967；IPv4 535；Fragment 2 |
| 27 | S7COMM | 2,298 | IPv4 2,096；GRE 101；UDP 88；Fragment 13 |
| 28 | CoAP | 2,158 | UDP 1,391；Fragment 430；IPv4 335；TCP 2 |
| 29 | RNSAP | 2,109 | Fragment 2,007；UDP 96；TCP 6 |
| 30 | ieee1905 | 1,896 | IPv4 1,318；UDP 474；MPLS 76；GRE 25；Fragment 3 |

## weber

物理帧 51,328；原生消息 42,095；非物理结束发布 26。

| 排名 | Wireshark 原始标签 | 帧数 | 同一批帧的 Shark 原始标签 |
|---:|---|---:|---|
| 1 | TCP | 8,509 | TCP 7,801；RTSP 176；TLSv1.3 156；Syslog 91；IPP 65；LDAP 39；TLSv1.2 37；TLS 27；DCE/RPC 19；HTTP/1.1 18；BGP 16；LDP 16；SMB2 14；DNS 12；IMAP 10；SSH 5；HTTP/1.0 3；Kerberos 2；POP3 1；RDP 1 |
| 2 | RTP | 6,462 | RTP 6,462 |
| 3 | DNS | 3,384 | DNS 3,372；UDP 8；TCP 4 |
| 4 | ICMPv6 | 2,967 | ICMPv6 2,826；MLDv2 141 |
| 5 | TLSv1.2 | 2,810 | TLSv1.2 2,486；RDPUDP2 228；TLS 96 |
| 6 | STP | 2,612 | RPVST+ 2,205；RSTP 407 |
| 7 | ICMP | 2,509 | ICMP 2,509 |
| 8 | UDP | 2,138 | UDP 2,123；TFTP 10；SSDP 5 |
| 9 | ESP | 1,600 | ESP 1,600 |
| 10 | NTP | 1,331 | NTP 1,331 |
| 11 | RMCP+ | 864 | IPMI 864 |
| 12 | TLSv1.3 | 854 | TLSv1.3 835；TLS 19 |
| 13 | RDPUDP2 | 830 | RDPUDP2 828；TLSv1.2 2 |
| 14 | VRRP | 818 | VRRP 818 |
| 15 | GLBP | 777 | GLBP 777 |
| 16 | HSRPv2 | 768 | HSRPv2 764；UDP 4 |
| 17 | ISAKMP | 719 | IKEv2 480；IKEv1 239 |
| 18 | L2TPv3 | 706 | L2TPv3 706 |
| 19 | OSPF | 695 | OSPFv3 417；OSPFv2 278 |
| 20 | SNMP | 678 | SNMP 678 |
| 21 | ISIS HELLO | 559 | IS-IS Hello 559 |
| 22 | SSHv2 | 537 | SSH 537 |
| 23 | BFD Echo | 472 | UDP 472 |
| 24 | MDNS | 396 | mDNS 396 |
| 25 | LOOP | 339 | EthernetCTPReply 339 |
| 26 | PIMv2 | 333 | PIMv2 333 |
| 27 | LDAP | 326 | LDAP 326 |
| 28 | LDP | 306 | LDP 306 |
| 29 | SMB2 | 242 | SMB2 122；TCP 107；SPNEGO 8；NTLMSSP 5 |
| 30 | Chargen | 233 | TCP 228；UDP 5 |
