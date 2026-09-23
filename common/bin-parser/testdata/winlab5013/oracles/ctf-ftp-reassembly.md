# CTF 专题一：FTP 被动模式重组

SPDX-License-Identifier: CC0-1.0

来源：本目录里的 pcapng 全部是为 yaklang PR #5013 新写的 Windows 实验流量，不是从 #5013 已有样本改名，也不是第三方 CTF/工控仓库里的抓包。作者放弃这些实验字节的版权（CC0-1.0），可以随仓库再分发。

原创题目，CC0-1.0。#5013 里已经有 FTP 样本，本题不是那份样本，也不从外部 CTF 包拷贝。旗标拆在数据连接的多个 TCP 段里；横幅里的 flag{not-the-ftp-flag} 是干扰项。

完成标准：从 PASV 给出的数据端口把 RETR 的文件重组出来，打印真正的 flag。只在控制连接里搜到 flag{ 不算。Wireshark 里控制连接和数据连接要分开看。

## 怎么验证

在解压后的目录执行 `go run .`（只依赖 Go 标准库）。程序读取 `captures/*.pcapng`，按协议把 TCP/UDP 重组后再解字段，把结果打印到标准输出。退出码 0 且输出与下面的 CRITICAL 块逐字相同，才算这一组验证结束。

下面任何一种都不算结束：只在 Wireshark 里看到有包、只对上了端口、README 里写了答案但程序没有从 pcapng 读出来、或者把 pcapng 删掉/截断/改一个应用层字节后程序仍然打印同一份 CRITICAL。

BEGIN CRITICAL
file=ctf-01-ftp.pcapng
protocol=ftp
file=flag.txt
data_port=1025
flag=flag{winlab-ftp-reassembly-5013}
END CRITICAL

## FTP 重组题

- 文件：`captures/ctf-01-ftp.pcapng`
- SHA-256：`ab7cedf32d17a93a68533a9088f63d504a9dbb2f720cb9eadd41828a7ca11600`
- 相对 #5013 的缺口：原创挑战流量，不是已有 FTP 夹具。
- Wireshark / tshark：过滤器 `ftp` 看控制连接（21），`ftp.request.command == "RETR"`。227 应答给出数据端口。数据连接本身是裸文件字节，协议列是 TCP；要对 PASV 端口做 Follow TCP Stream 才能看到完整旗标。
- 核心指标：文件名、数据端口、重组后的 flag。
- 验证标准：`go run .` 打印的对应 `file=ctf-01-ftp.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 TLS（FTPS）和多文件。干扰旗标不是答案。

