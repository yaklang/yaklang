# 工控流量专题

SPDX-License-Identifier: CC0-1.0

来源：本目录里的 pcapng 全部是为 yaklang PR #5013 新写的 Windows 实验流量，不是从 #5013 已有样本改名，也不是第三方 CTF/工控仓库里的抓包。作者放弃这些实验字节的版权（CC0-1.0），可以随仓库再分发。

这是新做的工控实验会话，不是把 #5013 里已有的 modbus-read-holding.pcap（645 字节，只有读保持寄存器）、ndpi 里的 bacnet，或已有的 enip/cip 文件重新打包。

要算“解完”，必须从 pcapng 里读出命令、点号和数值：Modbus 的功能码、地址、寄存器/线圈/异常码；BACnet 的设备实例、对象、present-value；EtherNet/IP 的会话号和 CIP 标签值。只认出 TCP 502 或 UDP 47808 不算结束。用 Wireshark 打开时应按下面每个文件的过滤器能看到这些字段。

## 怎么验证

在解压后的目录执行 `go run .`（只依赖 Go 标准库）。程序读取 `captures/*.pcapng`，按协议把 TCP/UDP 重组后再解字段，把结果打印到标准输出。退出码 0 且输出与下面的 CRITICAL 块逐字相同，才算这一组验证结束。

下面任何一种都不算结束：只在 Wireshark 里看到有包、只对上了端口、README 里写了答案但程序没有从 pcapng 读出来、或者把 pcapng 删掉/截断/改一个应用层字节后程序仍然打印同一份 CRITICAL。

BEGIN CRITICAL
file=ics-01-modbus.pcapng
protocol=modbus
read_fc=3
read_unit=1
read_addr=100
read_values=4660,7
write_fc=6
write_addr=100
write_value=42
coil_addr=3
coil=1
exception_unit=2
exception=2

file=ics-02-bacnet.pcapng
protocol=bacnet
device=device,5013
read_service=12
read_invoke=5
read_object=analog-value,7
read_property_id=85
read_property=present-value
read_value=21.5
write_service=15
write_invoke=6
write_object=analog-value,7
write_property_id=85
write_property=present-value
write_value=22

file=ics-03-enip-cip.pcapng
protocol=ethernet-ip-cip
session=0x5013
tag=Speed
read_type=DINT
read_value=1500
write_type=DINT
write_value=1510
END CRITICAL

## Modbus TCP

- 文件：`captures/ics-01-modbus.pcapng`
- SHA-256：`f1c00a25747b055de45fd26e9a9f204ac1f5b6fc9b84689d56e732e4bcc1de94`
- 相对 #5013 的缺口：已有样本只覆盖读保持寄存器。这里补了写单个寄存器、读线圈和非法地址异常。
- Wireshark / tshark：过滤器 `modbus` 或 `mbtcp`，TCP 502。事务 1 是功能码 3，事务 2 是功能码 6，事务 3 是功能码 1，事务 4 的功能码是 0x83（异常）。
- 核心指标：单元、功能码、地址、寄存器值、线圈、异常码。
- 验证标准：`go run .` 打印的对应 `file=ics-01-modbus.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有文件记录、设备标识（0x2B）和串口 RTU。

## BACnet/IP

- 文件：`captures/ics-02-bacnet.pcapng`
- SHA-256：`bbde8345b97c353083f8e1dae75674b1990f81565006dc9d0242882ed6ad24b1`
- 相对 #5013 的缺口：已有 bacnet 文件名不是这一组 Who-Is / I-Am / ReadProperty / WriteProperty。
- Wireshark / tshark：过滤器 `bacnet` 或 `bacapp`，UDP 47808。BVLC 0x81。Who-Is 限定设备 5013，I-Am 带 device 对象，随后是 analog-value,7 的 present-value 读和写。
- 核心指标：设备实例、对象标识、读出的 present-value、写入的 present-value。
- 验证标准：`go run .` 打印的对应 `file=ics-02-bacnet.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 COV、分段确认和 BACnet/SC。

## EtherNet/IP CIP

- 文件：`captures/ics-03-enip-cip.pcapng`
- SHA-256：`c4f595b56a95025549cc94533bac9f8b0f1a0f2a936297d6b5e05106062faf00`
- 相对 #5013 的缺口：语料里虽有 enip/cip 文件，但不是会话 0x5013、标签 Speed 的这次读 1500 / 写 1510。本文件的 SHA-256 与那些文件不同。
- Wireshark / tshark：过滤器 `enip` 和 `cip`，TCP 44818。RegisterSession 命令 0x0065，SendRRData 命令 0x006F。CIP 服务 0x4C 读标签、0x4D 写标签，路径是 ANSI 符号段 0x91，类型 DINT（0x00C4）。
- 核心指标：会话号、标签名、读出的 DINT、写入的 DINT。
- 验证标准：`go run .` 打印的对应 `file=ics-03-enip-cip.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有连接管理器的隐式 I/O、PCCC 和加密 CIP Security。

