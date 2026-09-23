# Windows 实验流量（叠在 #5013 上，未合并）

样本说明写在每个 zip 的 `README.md` 里，不在本页展开。每个 zip 都带 `captures/*.pcapng` 和 `go run .` 验证程序。验证结束的标准是：程序从 pcapng 解出 README 里 `BEGIN CRITICAL` 与 `END CRITICAL` 之间的字段，退出码为 0。删掉或截断 pcapng 必须失败。

| zip | 内容 |
|---|---|
| `zips/win-protocols-20.zip` | 20 组 #5013 语料里还没有的、Windows 上能做的综合会话 |
| `zips/topic-ics.zip` | 工控：Modbus 点值、BACnet present-value、EtherNet/IP CIP 标签 |
| `zips/topic-power.zip` | 电力：IEC 104 的 IOA、GOOSE 状态、C37.118 相量 |
| `zips/topic-blueteam.zip` | 蓝队：实验室 Stratum 矿池会话和 HTTP 信标 IOC |
| `zips/ctf-ftp-reassembly.zip` | 原创 CTF：FTP 被动模式重组出的 flag |
| `zips/ctf-dns-labels.zip` | 原创 CTF：按 DNS TXT 查询名拼出的 flag |

这些 pcapng 是本目录程序生成的实验字节，地址用 192.0.2.0/24，许可证 CC0-1.0。不是 #5013 已有捕获的改名，也不是外部 CTF 或工控仓库里的文件。不要把本变更当成协议路线图评分。
