# 电力行业专题流量

SPDX-License-Identifier: CC0-1.0

来源：本目录里的 pcapng 全部是为 yaklang PR #5013 新写的 Windows 实验流量，不是从 #5013 已有样本改名，也不是第三方 CTF/工控仓库里的抓包。作者放弃这些实验字节的版权（CC0-1.0），可以随仓库再分发。

电力专题是新的实验帧，不复制 #5013 里 718 字节的 iec104-startdt-interrogation.pcap、621 字节的 goose-dataset.pcap，也不复制 c37118-cmd.pcap。

解完的标准是读出报文里真实存在的量：IEC 104 的 ASDU 类型、公共地址、IOA 和值；GOOSE 的 goID、gocbRef、数据集和状态变化；C37.118 的 IDCODE 与相量分量。只看到 2404 端口或 0x88B8 以太类型不算结束。用 Wireshark 打开时，下面写的过滤器必须能定位到这些字段。

## 怎么验证

在解压后的目录执行 `go run .`（只依赖 Go 标准库）。程序读取 `captures/*.pcapng`，按协议把 TCP/UDP 重组后再解字段，把结果打印到标准输出。退出码 0 且输出与下面的 CRITICAL 块逐字相同，才算这一组验证结束。

下面任何一种都不算结束：只在 Wireshark 里看到有包、只对上了端口、README 里写了答案但程序没有从 pcapng 读出来、或者把 pcapng 删掉/截断/改一个应用层字节后程序仍然打印同一份 CRITICAL。

BEGIN CRITICAL
file=power-01-iec104.pcapng
protocol=iec104
startdt=act-con
ca=1
sp_type=M_SP_NA_1
sp_ioa=2001
sp=1
me_type=M_ME_NC_1
me_ioa=1001
me_value=220.5
interrogation=actterm

file=power-02-goose.pcapng
protocol=goose
gocbRef=LABP1/LLN0$GO$gcb1
goID=LAB-GOOSE-1
datSet=LABP1/LLN0$ds1
states=5:0,6:1
appid=0x3000

file=power-03-c37118.pcapng
protocol=c37.118
idcode=7
format=0x000a
phasor=V1
real=110
imag=10.5
freq_off_hz=0.5
nominal_hz=50
END CRITICAL

## IEC 60870-5-104

- 文件：`captures/power-01-iec104.pcapng`
- SHA-256：`3f02af795ad1258cb3a5e717de5e9d730b4b945941978fab123570b019db7f1f`
- 相对 #5013 的缺口：已有样本没有同时给出 IOA 2001 的单点与 IOA 1001 的短浮点，也没有激活终止。
- Wireshark / tshark：过滤器 `iec104` 或 `104apci`，TCP 2404。U 帧 STARTDT act/con，然后 I 帧：C_IC_NA_1（100）总召唤、M_SP_NA_1（1）、M_ME_NC_1（13）、再次 C_IC_NA_1 且 COT=10，最后 S 帧确认。公共地址和 IOA 按 104 常见的 2 字节 COT、2 字节 CA、3 字节 IOA，小端。
- 核心指标：CA、单点 IOA 与 SPI、测量 IOA 与浮点、总召唤激活终止。
- 验证标准：`go run .` 打印的对应 `file=power-01-iec104.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有对时、遥控选择/执行和文件传输。

## IEC 61850 GOOSE

- 文件：`captures/power-02-goose.pcapng`
- SHA-256：`07d9da77d7ee338e23343934db23fc756f3f55aa4e6f32111715b99858ea8317`
- 相对 #5013 的缺口：已有 goose-dataset.pcap 不是 goID LAB-GOOSE-1 从 stNum 5 到 6 的状态变化。
- Wireshark / tshark：以太类型 0x88b8，目的 MAC 01:0c:cd:01:00:01。过滤器 `goose`。APPID 0x3000。两帧的 stNum/布尔量不同。gocbRef、datSet、goID 在 goosePDU 的 context 标记 0、2、3。
- 核心指标：gocbRef、goID、datSet、两帧的 stNum 和布尔状态。
- 验证标准：`go run .` 打印的对应 `file=power-02-goose.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 GOOSE 订阅超时、密钥和采样值 SV。

## IEEE C37.118 同步相量

- 文件：`captures/power-03-c37118.pcapng`
- SHA-256：`7f553e0ca67b37dc3d66b3881bd4bb394fbaf00c77d91d157de4acd4375937bd`
- 相对 #5013 的缺口：已有 c37118-cmd.pcap 是命令帧场景，没有这一帧里带名字的浮点相量。
- Wireshark / tshark：过滤器 `synphasor`，TCP 4712。先是命令帧（启动传输，CMD=2），再是 config2（站名 LAB-PMU，相量名 V1，50 Hz），然后数据帧。CRC 是 init 0xFFFF、多项式 0x1021、无最终异或，覆盖除 CHK 外的整帧。若 Wireshark 的 CRC 专家信息与实现差一个变体，字段仍应按 IDCODE 和相量解析；本验证以同一算法重算 CRC，不符则失败。
- 核心指标：IDCODE、相量名、直角坐标实部/虚部、频率偏差、额定频率。
- 验证标准：`go run .` 打印的对应 `file=power-03-c37118.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 config3、多 PMU 和分相模拟量。

