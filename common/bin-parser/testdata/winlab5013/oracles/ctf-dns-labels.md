# CTF 专题二：DNS TXT 分段

SPDX-License-Identifier: CC0-1.0

来源：本目录里的 pcapng 全部是为 yaklang PR #5013 新写的 Windows 实验流量，不是从 #5013 已有样本改名，也不是第三方 CTF/工控仓库里的抓包。作者放弃这些实验字节的版权（CC0-1.0），可以随仓库再分发。

原创题目，CC0-1.0。不是第三方 CTF 流量包。旗标拆在三个 TXT 回答里，decoy.flag.lab-invalid 的 flag{not-this} 要丢掉。完整旗标在文件里不是连续字节。

完成标准：按 c1、c2、c3 的查询名顺序拼接 TXT，得到 flag。对整份 pcap 搜 flag{ 会同时看到干扰项，那不算解出来。Wireshark 的 DNS 解析树里，三条 TXT 是分开的资源记录。

## 怎么验证

在解压后的目录执行 `go run .`（只依赖 Go 标准库）。程序读取 `captures/*.pcapng`，按协议把 TCP/UDP 重组后再解字段，把结果打印到标准输出。退出码 0 且输出与下面的 CRITICAL 块逐字相同，才算这一组验证结束。

下面任何一种都不算结束：只在 Wireshark 里看到有包、只对上了端口、README 里写了答案但程序没有从 pcapng 读出来、或者把 pcapng 删掉/截断/改一个应用层字节后程序仍然打印同一份 CRITICAL。

BEGIN CRITICAL
file=ctf-01-dns.pcapng
protocol=dns
qnames=c1.flag.lab-invalid,c2.flag.lab-invalid,c3.flag.lab-invalid
ignored=flag{not-this}
flag=flag{dns-label-join-5013}
END CRITICAL

## DNS TXT 拼接题

- 文件：`captures/ctf-01-dns.pcapng`
- SHA-256：`7e5e7c649dfbc4f3cb53e570b5c40068fd7f7ff70e644ad8bc839db28610fb18`
- 相对 #5013 的缺口：原创挑战；已有 DNS 样本不含这四条 TXT。
- Wireshark / tshark：过滤器 `dns`，UDP 53。`dns.qry.name contains "flag.lab-invalid"`。每个回答是一条 TXT。c1/c2/c3 才参与拼接，decoy 只作为负例出现在包里。
- 核心指标：三条查询名、被忽略的 TXT、拼接后的 flag。
- 验证标准：`go run .` 打印的对应 `file=ctf-01-dns.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 DNSSEC 校验和 TCP DNS。干扰 TXT 不是答案。

