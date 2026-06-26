`pcapx` 库提供底层网络抓包与造包能力：监听网卡流量、解析 TCP/HTTP/TLS 流，以及逐层构造并注入以太网/IP/TCP/UDP/ICMP/ARP 数据包。属于原始套接字操作，通常需要管理员/root 权限。

典型使用场景：

- 抓包嗅探：`pcapx.StartSniff(iface, opts...)` 监听网卡，`pcapx.OpenPcapFile` 解析 pcap 文件，配合 `pcapx.pcap_bpfFilter` 过滤、`pcapx.pcap_onHTTPFlow` / `pcapx.pcap_onTLSClientHello` / `pcapx.pcap_everyPacket` 等回调处理流量。
- 造包注入：`pcapx.PacketBuilder` 逐层构造数据包（`pcapx.ethernet_*` / `pcapx.ipv4_*` / `pcapx.tcp_*` / `pcapx.udp_*` / `pcapx.icmp_*` / `pcapx.arp_*` 选项），`pcapx.InjectRaw` / `pcapx.InjectTCP` / `pcapx.InjectIP` / `pcapx.InjectHTTPRequest` 注入到网络。
- 权限：`pcapx.FixPermission` / `pcapx.WithdrawPermission` 处理抓包权限。

与相邻库的关系：`pcapx` 是底层数据包能力，`synscan`/`finscan` 在其之上做端口扫描，`netstack`/`netutils` 处理路由；分析 TLS 指纹可结合 `ja3`。
