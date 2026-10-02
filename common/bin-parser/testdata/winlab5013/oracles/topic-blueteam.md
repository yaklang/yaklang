# 蓝队应急流量处置（含挖矿会话）

SPDX-License-Identifier: CC0-1.0

来源：本目录里的 pcapng 全部是为 yaklang PR #5013 新写的 Windows 实验流量，不是从 #5013 已有样本改名，也不是第三方 CTF/工控仓库里的抓包。作者放弃这些实验字节的版权（CC0-1.0），可以随仓库再分发。

这是实验用的应急分析样本，不是可用的矿工程序，也没有连接任何真实矿池。Stratum 只在 192.0.2.20:3333 上交换 JSON 行；域名 lab-pool.invalid 和 update.lab-invalid.test 都是保留在文档里的名字。授权参数里的口令是单个字母 x，验证输出不打印它。

处置这些流量时，解完的标准是从报文里抽出矿池、矿工、任务号，以及实验室信标的 Host、URI、User-Agent。不包含恶意家族归因、磁盘取证或遏制步骤；那些不算本包的完成条件。Wireshark 里用下面的过滤器和 Follow TCP Stream 能看到同一批字段。

## 怎么验证

在解压后的目录执行 `go run .`（只依赖 Go 标准库）。程序读取 `captures/*.pcapng`，按协议把 TCP/UDP 重组后再解字段，把结果打印到标准输出。退出码 0 且输出与下面的 CRITICAL 块逐字相同，才算这一组验证结束。

下面任何一种都不算结束：只在 Wireshark 里看到有包、只对上了端口、README 里写了答案但程序没有从 pcapng 读出来、或者把 pcapng 删掉/截断/改一个应用层字节后程序仍然打印同一份 CRITICAL。

BEGIN CRITICAL
file=blue-01-stratum.pcapng
protocol=stratum
pool_name=lab-pool.invalid
pool_ip=192.0.2.20
pool_port=3333
agent=lab-stratum/0
worker=lab.win01
job=7f3a9c

file=blue-02-beacon.pcapng
protocol=http-beacon
host=update.lab-invalid.test
uri=/beacon?host=WINLAB&id=5013
user_agent=lab-beacon/5013
body=ok job=7f3a
END CRITICAL

## Stratum 矿池会话

- 文件：`captures/blue-01-stratum.pcapng`
- SHA-256：`8e24d6895849600f69701c1ab5ab6f997f52eb873e2820d650d64a54d9b67d8b`
- 相对 #5013 的缺口：语料里没有 stratum 或 mining 捕获。
- Wireshark / tshark：先是 DNS `lab-pool.invalid` A 记录，过滤器 `dns.qry.name == "lab-pool.invalid"`。随后 TCP 3333，Follow TCP Stream 是一行一个 JSON：mining.subscribe、mining.authorize、mining.notify、mining.submit。Wireshark 通常把 3333 显示成 TCP；用 `tcp.port == 3333 && tcp contains "mining.notify"` 能定位任务。
- 核心指标：池域名、A 记录、端口、矿机 agent、矿工名、notify 的 job id。
- 验证标准：`go run .` 打印的对应 `file=blue-01-stratum.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有真实 share 校验、TLS 矿池和矿机进程树。

## 实验室 HTTP 信标

- 文件：`captures/blue-02-beacon.pcapng`
- SHA-256：`9efc1e3613114bd70495b229192a66c64c5df2640d7d268b542e7bf983956bd7`
- 相对 #5013 的缺口：不是已有 HTTP 语料的改名；Host 和 URI 是这组 IOC。
- Wireshark / tshark：过滤器 `http`，端口 80。`http.host == "update.lab-invalid.test"`，请求行含 `/beacon`，User-Agent 为 lab-beacon/5013。
- 核心指标：Host、URI、User-Agent、应答正文里的 job 标记。
- 验证标准：`go run .` 打印的对应 `file=blue-02-beacon.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 JA3、加密信标和主机侧进程关联。

