`finscan` 库实现 FIN 扫描——一种隐蔽的端口扫描技术：向目标端口发送仅置 FIN 标志的 TCP 包，依据是否回 RST 判断端口状态。相比 SYN 扫描更隐蔽，可绕过部分简单防火墙/日志，但对现代系统准确性有限。

典型使用场景：

- 扫描：`finscan.Scan(target, port, opts...)` 对目标端口做 FIN 扫描，返回结果 channel。
- 控制：`finscan.concurrent` / `finscan.rateLimit` 控速，`finscan.excludeHosts` / `finscan.excludePorts` 排除，`finscan.wait` 控制收包等待，`finscan.outputFile` / `finscan.outputPrefix` 落盘。

与相邻库的关系：`finscan` 与 `synscan`（SYN 半开放扫描）同属底层端口探测，需要原始套接字权限；发现开放端口后可交给 `servicescan` 做指纹识别。
