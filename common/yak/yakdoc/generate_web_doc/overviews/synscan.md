synscan 是 SYN 半开放端口扫描模块：自行构造并发送 TCP 三次握手的第一个 SYN 包，只要收到对端的 SYN-ACK 即判定端口开放，随后发 RST 中断握手，不建立完整 TCP 连接。它绕开了操作系统对连接状态与文件描述符的维护，原理类似 masscan——短时间内把 SYN 包批量发出，再统一等待一段时间收集回包，因此速度极快、资源消耗极低，适合大范围端口快速探活。

核心接口是 synscan.Scan(targets, ports, opts...)，targets 支持 IP、CIDR、域名，ports 支持 22,80,443、1-65535、1-100,200-300 等写法；返回结果 channel 流式产出 *SynScanResult，每个结果含 Host 与 Port，可调用 Show() 打印。由于是批量发包后等待，提供 wait(秒) 控制收包等待时长，并有 rateLimit/concurrent 控速、excludeHosts/excludePorts 排除、outputFile/outputPrefix 落盘、context 取消等可选项。

注意：SYN 扫描使用原始套接字，需要 root/管理员权限，可先用 synscan.FixPermission() 修复权限；高速发包可能造成短暂网络拥塞或被防火墙拦截，且丢包会带来误差，需谨慎使用。synscan 常作为扫描链路的第一步，配合 servicescan.ScanFromSynResult 做"先探活、再识别指纹"的高效组合。
