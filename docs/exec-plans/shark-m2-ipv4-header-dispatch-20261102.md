# IPv4 options、分片和子协议分发：2026-11-02

M2-69有限关闭N14.U02的通用IPv4头边界/分发缺口。代码和干净SDK、flow及Windows构建为
`83f4df508e0605175d895ba34d120c69e26d9cdd`，仍只进入M2 PR #5371；SMB继续搁置。
N14整体、完整TLS/QUIC恢复及TCP连接政策仍开放。

## 故障和实现

旧IP规则仅为少数协议先消费options，TCP、UDP、ICMP及嵌套IP可能从选项起点读取子头。
规则也可能直接把物理分片内容当成完整子协议。1067的展示控制暴露故障；独立1086矩阵
保留原故障。修复按本层Version/IHL/Total Length确认边界，先读取并校验有界options TLV，
保留NOP/EOL后的原padding，再分发子协议。物理分片仅保留原片段字节，重组由既有owner负责；
原DCCP专门的分片拒绝及source-route checksum上下文合同保持。

依据：[RFC791 §3.1](https://www.rfc-editor.org/rfc/rfc791.html#section-3.1)。本项不把
原始零长度当作有效线上长度，不声称验证全部options语义、checksum或整个IPv4实现。

同时同步原生operator plan：完整匹配新的头部程序，仅在已观察的Version4、IHL5、足够总长、
无分片且默认投影时执行普通分发；options/错误/片段/自定义输出在任何副作用前回到原VM。
新增资格拒绝、完整grammar变异和fresh VM对照。原测试只将已移除的`protocol == 9`变异位置
更新为实际的`header < 20`；原断言保持并增加控制，未绕过检查。

YAML与运行时`rules.tar.zst`一起提交。279个YAML源只有此规则变化；既有embedded-source测试通过，
1115重复运行既有稳定生成器后所有源与归档完全不变。归档215324B，SHA256
`d72bc2e2e33795359369d5ba2b89341e3d7baa372d20111b0aeb3780d14045c3`。
两个捕获fixture ZIP保持原哈希，无样本解压到源码目录。

## 新Windows实抓及参考

1090使用本机Winsock IP_OPTIONS创建plain、NOP/EOL和Router Alert三条localhost TCP连接，
每条执行两次HTTP请求/响应；Npcap仅捕获自有临时端口。得到45个包，其中16个带options，
4964B PCAPNG，SHA256 `0612a0367aa561ce79197368e4f5235106633715d9a408ef69868506cef58c8d`。
这是新Windows本机真实协议栈流量，不是外部Internet流量或仅构造PCAP。

1091新跑TShark4.6.9双遍、空配置、显式defrag/out-of-order，关闭TSO推断；1093对全部45帧
完成906个字面头检查，按原始引用、tuple、接口和完整HTTP字节哈希匹配12个原生消息/6对请求响应，
核对response_to及三个可观察完整握手和连接分区，不用数字ID相等代替关联。
1117将12个原生HTTP对象的全部起始行、头行及响应正文重建为原消息，方法/状态语义alias亦一致；
summary SHA256 `2ba1c2592801d27031e981e4a1796b905fa55bd5435c483905a2f77c7350b52a`。
新旧三种完整导出逐字节相同：原生HTTP/会话原本正确，本项修复物理字段树；不能推广为所有HTTP或TCP政策正确。

新版实际Windows加载全部45包，过滤原frame34；140B、接口0，层级为
`Loopback → IPv4 → TCP → HTTP/1.1`。20个实际console状态核对Source0 [24,28)的94040000、
正确端口53962→53959、完整140B读回、80×30/180×48、键鼠/Tab、CP936恢复与自有进程关闭。
主线程实际查看`Windows-live-IP-options-native1113-01/console/01-selected-field.png`；
为实际console cell渲染，不冒称所有Terminal/DPI/IME都已验收。

## 完整捕获、残包与fuzz

证据均在仓库外`C:\tmp\yak-shark-ESP-SSH-UI-20261028`；PCAP、keylog/密钥、完整导出、
PDML、原始帧证据、fuzz临时文件和截图没有入库或加入ZIP。

| 项目 | 实际验收 |
| --- | --- |
| 完整本地回归1101 | 289623 pass、24历史optional skip、0 fail；304新增精确ID全部pass；stdout SHA256 `e5e826eb7f382668d365e832d584417184ea6cdccd7ecc6ed8abdcffdf55b0a8` |
| 六完整公共导出1105 | Weber/Sharon共379455物理帧，SDK/flow/CLI六导出全部逐字节保持；summary `15b06154dbea8f6871315f4650481b41b8e288a2dc8c54bc62b982f5993d4c88`。未改变输出不等于全部字段/会话与Wireshark一致 |
| 公开头来源1095/1106/1114 | 完整遍历两原捕获后事前固定76有界见证，覆盖合法/非法options、分片、版本/IHL/总长和残头；重新双遍参考829个字面检查，缺失的部分参考字段与诊断保留。14个片段均无虚构完整子协议，53个公开树改变；不是全协议字段验收 |
| 按需worker1107 | 76公开见证加45新实抓，共242次旧/新实际执行；所有范围属于原帧，29个普通实抓树完全保持，16个options包的叶字段及TCP端口范围正确 |
| 插桩fuzz1111 | 明确`go test -c -fuzz=FuzzIPv4OptionsEnvelope`构建，6种子完成，35050执行；独立包络/TLV/消息边界模型，1CPU、总768MiB、最多2进程，第三进程拒绝正控通过；不代表全部IP状态或应用fuzz已完成 |
| 综合1116 | summary SHA256 `aa70a481d88c5ecd045e68ebe07fe3ce4899f803f32ff3918d730accabcba1e0`；空闲40770109440B，高于8GiB底线 |

1087运行时归档未重生成，明确不是新YAML的有效验收；随后同步归档。1089完整回归保留唯一
operator-plan合同失败，修复其新头部资格后1101全过。1097是新增测试夹具错误地继承raw,0
上限的失败，改为明确8字节子区间后与fresh VM均一致。1094的任意协议号见证超过原界限，
1095保留完整盘点，只对事前规定的常见号码/other桶做有界来源抽样；未丢弃完整计数。

## CI及未关闭边界

父`b603d21e02939b2fd874ecbbc93828c735008b31`已独立47约定检查、91执行、2状态全部成功；
314520原始ID/动作与事前清单一致，145新增和131084 required全pass/零required skip。
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41402609456)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41402614250)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41402748076)。官方产物13122223030，
16803881B，SHA256 `16852ec512ec4a5b12a97a39d09ceb9b4cd6a6dc7487005db78909b037df78b9`；
综合1104 SHA256 `05564339aee13653987567b7adba3f08243ae27da1986d308fafe0f6771f70f7`。
本批最终head必须另验全部CI，不借用父成功；所有既有原始ID/动作、304新增及required均保留。

当前69父项59有限完成，7进行中/3 SMB搁置，10未关闭。非SMB17包13有限完成/4开放；
U02这项新缺口有限关闭，已知剩余恢复/字段/政策粗估回到16–32 AI实施小时，等待与新问题另计。
N14的其他封装、IPv6、完整字段与承载代次、冲突/缺片/预算恢复仍开放；不宣称核心协议完备。
