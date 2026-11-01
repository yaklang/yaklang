# M2 捕获 TCP 承载资格与 IPv4 原始长度：2026-11-01

代码、测试及干净构建：`788c8e27ba38d9037a4a75284bbcada74f843d7e`。
功能提交为 `fa410b480c5c16a3736c2f105c74742843837cb4`、
`cafaca236946794045841d92d87526a24e1d64a0`；
`f7d6a5afcde57cd2954f1998f35cf2fa14cec5b4` 修正显式 Diff CI 重跑，788 修正历史测试的承载资格。
继续同一 M2 阶段分支与 [PR #5371](https://github.com/yaklang/yaklang/pull/5371)，SMB 保持搁置。
本文关闭 M2-53/54 两个有限合同；最终文档 head 的全部 CI 单独归 M2-11，不沿用旧 SHA。

## 实际发现与修复

Sharon 原始第 207944 包是 IPv4/TCP。原生探测器却会将形似 QUIC 长头的 TCP 字节锁定为
`quic`，再发布“QUIC session was not observed”。实际 Windows 页面因此出现 TCP 列表行和
QUIC 详情树。这是默认承载探测的误识别，未获得 QUIC 包或会话的资格。

已移除该 TCP 自动探测入口。合法 UDP QUIC 路径、显式的独立算法 API 保持各自资格。
旧快照或外部调用若把 QUIC/HTTP3/DoQ 元数据挂在 TCP 上，展示为未确认的 TCP 字节观察，
保留拥有的字节，不展示 QUIC/TLS 字段或认证/消息完成声明；列表、详情、worker 标题一致。
普通未识别的重组 TCP 字节同样使用 observation，完整保留与有界 preview 分开说明。
捕获的传输失败前缀不冒充已经获得边界的重组流负载。

规范依据为 [RFC9000 §12](https://www.rfc-editor.org/rfc/rfc9000.html#section-12)。
对 TCP 中的相似字节不再自动赋予 UDP 原生协议身份，不影响调用者显式提供 QUIC 包字节的算法测试。

另一问题是 gopacket 的 TSO 兼容行为会把 IPv4 Total Length 的原始 `0000` 改成捕获长度。
新页面保留 `Total Length: 0` 的 Source0 两字节范围，单列 Captured Prefix Bytes 和
`TSO origin unconfirmed`。原说明在真实双栏中被裁切，现拆成短行。
这些值只证明捕获前缀，不证明线上 IPv4 包有效、确实发生 TSO 或 TCP 握手成立。

## 原始来源与 Wireshark

全部额外 PCAP、提取记录、完整导出、keylog、二进制与控制台图片留在仓库外。
未修改任何捕获 ZIP、PCAP 或归档清单。构建使用 Go 1.22.12/windows/amd64，
`GOMAXPROCS=3`、`GOMEMLIMIT=2GiB`、`-p=1`；运行期间源码、HEAD、索引和输入保持冻结。

外部根目录 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/` 下：

| 证据 | 实际结果 | SHA256 |
|---|---|---|
| `M2-TCP-source441-01/packet-ledger441.json` | 独立遍历全部 328127 个 PCAPNG 包；52 个物理见证、6 个 TCP 例项、8 段唯一原字节窗口 | `e7576d6b9d26a344989ba330feb5ebc16d967da9edca93a0c5d877d33e8cf2e7` |
| `M2-TCP-reference444-01/summary444.json` | 新全原包两遍 TShark 的冻结输出复核；52 包、752 个原字段检查、12 个零总长归一化显示 | `bfbe9b33cb70a9ae4ee7e37be51a9070b442cce9b2440b92d76a52ef19a9c0d0` |
| `TCP-admission479-01/summary479.json` | 新源码实际回调：44 观察帧、8 段 544B；386B 有声明 TCP 范围、158B 仅零总长捕获前缀；初始片段后无新增接纳字节 | `3f7824e249195b83c470e4bca54d15973cbfbea3983cd3211d2dceafda5a8a73` |

参考工具 `tshark.exe` SHA256 为
`662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b`。
对整个原始 Sharon 捕获执行 `-2 -n`、空配置、无 Decode As/额外 keylog；显示过滤器只限制序列化。
硬限 2 CPU、1GiB、1 个进程。Wireshark 对 12 包的 `ip.len` 显示捕获长度，但 PDML 原值仍为
`0000`，并注明原字段为 0、推测 TSO。不能把其归一化显示当成原始字段。

6 个 TCP 例项以原始域、帧、时间、TCP 序号/标志和实际回调核对。
另有 SYN+FIN/RST、残缺/碎片控制；原始源与全部差异保留。
Wireshark stream 数字不是连接建立证据，`half_open=false` 也不等于已经证明完整三次握手。
本次没有据此改写分区/复用政策，N10 的剩余政策验收仍开放。

## 两个完整大捕获与递进发布

| 输入 | 物理包 | SDK 事件 | 流 / 包归属回调 |
|---|---:|---:|---:|
| Weber The-Ultimate-PCAP | 51328 | 42223 | 824 / 14172 |
| Sharon ultimate_wireshark_protocols | 328127 | 263109 | 840 / 7700 |

原始捕获 SHA256 分别为
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`、
`aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`。
Weber 使用既有授权 keylog，忽略嵌入密钥；Sharon 不加密钥。
`runtime-TCP-public478-01/summary478.json` 的 6 次完整 SDK/flow/CLI 回放全部 exit0、输入未变，
SHA256 `b7154acf4d2a1c6f1105c0a8162c39e9be3b9b7400560cebd611c7b51ec8e14c`。
显式输入上限用于拒绝过大文件，未截断捕获。

Weber 的 3 份导出逐字节相同，两捕获的全部 flow JSON 也逐字节相同。
Sharon 唯一语义修正是原生 TCP/QUIC 候选：旧 PDU180080 变为 PDU262729 的未识别/incomplete
流字节观察，Native flow646、物理 TCP flow508、域 S0/I226、原第207944帧、端点及51B原数据保持一致。
原数据 SHA256 `05b785bb5be58e85a4ef5339fd207f891fe6be9a6aa450b6d453c45a8141df1a`。
它在 capture_close 才发表，不能回填为第207944包当时已经获得的完整应用 PDU。

使用既有归档工具的明确 PDU 引用 schema，逐个以原数据/贡献帧/域/端点/两个流命名空间配对。
82649 个生成编号移动，但其余 263108 个完整 SDK 事件在明确引用变换后完全相同；
不归一化流 ID、线上的整数或任意业务字段。
`TCP-SDK-comparison482-01/summary482.json` SHA256
`a05faefecc5f3fbbea3f987f134314e3f1ae82ef856de7874f40f04a7150430d`。

CLI 核完全部 379455 个物理包。Sharon 旧328677行逐行配对，仅移除旧帧上的过早观察、
增加1条 capture_close 发布，后者逐字段等于实际 SDK 的明确 publication schema。
82437 个旧行因生成引用或该观察发生变化，其余完整行严格相同；5 个端点/域/流/状态/线长变异负控拒绝。
物理协议列计数不变：旧列表本就显示 TCP，改进发生在原生身份、详情与发表一致性。
`TCP-CLI-comparison483-01/summary483.json` SHA256
`26cd7e54620f952ed0fdf14930881a5511b89ef9b275bbc01d4ccb715c447e2b`。
这些保持门不等于全部 Wireshark 字段、会话、协议或 fuzz 正确性一致。

## 实际 Windows、真实来源 QUIC 与 fuzz

干净构建 `bins-TCP473`：events SHA256
`43e135bffa68a2173d16491de484b0ada069f6fc6f0d912ce2fd0c61c65f80be`；
flows `a333bcb02d780e5562f99f3e9f6080b539b98b30dc119389339667682ddcb1aa`；
yak `5201b38caf50c3dcc7f459c54713b33cd7b60975577c94d561cfee2d342cc31c`。

最终 `Windows-TCP477-01/closure477.json` SHA256
`cb4211ff76f30a7ef360e7bb3436a816b09a65b6dd8be31a99918f56669ff627`：
两个真实原记录、36个状态，完整可见105B均与源逐字节相同，Total Length只高亮[16,18)，
无 QUIC 树，短 TSO 说明可见；鼠标/自适应 tabs、80×30与180×48、CP936恢复通过。
每个单记录 PCAPNG 只用于页面：保留原EPB、接口226与时间，显示包号重编号为1；不是原连接回放。
图片来自实际控制台 UTF-16 单元格/属性的固定渲染，不声称覆盖全部 Windows Terminal 字体/DPI/IME。
早期 `Windows-IPv4-zero453-01` 的原截图及失败保留，实际暴露了误QUIC树和被裁切说明。

`fresh-QUIC-TCP480-01/summary480.json` SHA256
`e7b68a0b3cbe1c092fccea0b8b4497eb03fdead55c908989081d7486c1289fa2`：
新构建在今日既有实抓394上重新运行3个完整导出，SDK/CLI/flow逐字节匹配已独立验收的0fc导出。
原394的3套件、端口复用/CID/NAT、12丢/187延/3损坏、2053保护包/2050认证、21握手、
36完整流及73537参考字段闭合继续有效；这3次是新回放，不称3次新网络抓包。
原始来源和范围见[QUIC记录](shark-m2-quic-crypto-qualification-20261101.md)。

`FuzzQUICCannotBecomeCapturedTCPIdentity` 的插桩 binary SHA256
`1f536b80f81f8dc55a015752df1a8af825f78684850e1b9a7415dcea66bb6957`。
`fuzz-TCP476-01` 完成3/3种子、43457次实际执行、0失败、61.968s；
硬限1CPU/768MiB/同时2进程，峰值692924416B，第三进程负控被阻止，Job关闭后确认无该路径残留进程。
失败 corpus/cache/temp 留仓库外，未使用内部 fuzzworker 标志或零次 baseline。
receipt SHA256 `8e2b608846c9753b7dd28730c01c62c9a26cad5357f7693604f1a9e83c4ad016`。

## 原失败、最终测试与 CI 门

所有原失败及原输入保留，未用后续通过覆盖。

| 运行 | 结果与处理 |
|---|---|
| IPv4 443 | 17 fail，定位缺原值/范围/资格；445为17 pass |
| TCP/QUIC 454、455 | 原生71 fail/4 pass及CLI25 fail；454另有CLI测试类型名编译错误，未算CLI红测覆盖 |
| 456、457 | 16个缺尾场景错误要求无重组错误；改为明确要求unfilled gap后，584项全pass，不吞丢失 |
| 458 | 声称认证的TCP元数据导致标题不一致，12 pass/13 fail；现列表与详情共同拒绝 |
| 459、460 | 旧SSH非SSH对照把QUIC统一写TCP；改成正确UDP承载，原断言/ID保留，新TCP反例另测；1225 pass/2历史skip |
| 461、463 | 未识别流字节原被写空协议PDU，9 fail；新增stream拥有范围/preview资格，CLI完整1234 pass/2历史skip |
| 468、472 | 239366 pass/12历史skip/1 fail，原M1 TCP包裹的QUIC算法样本要求错误的QUIC上下文计数；保留所有PCAP/清单/hash，改成更强的无QUIC/无解密认证/无decoded、输入字节计数及显式算法正控；76项通过 |
| 最终475 | 原生+整个CLI+CI配置：239367 pass/12历史optional skip/0 fail，504.438s，源码/输入均未变 |

475 stdout SHA256 `4046aa7912b0ac6870abd7da4b74f9743e2c98c064b17493edc932232c726cdd`。
该本地范围不是所有CI包；完整专项仍须跑最终提交。
显式 Diff workflow_dispatch/rerun 现在忽略既有success状态，failed/skip标签继续生效；
不会把缓存状态导致的 skipped 扫描当作已执行。

未来 artifact 出现前已经冻结 `TCP-CI471-03/expected-test-IDs471.json.gz`，SHA256
`d00cf81483e7e2a0e6f674ee6d8d28a89c8f972b7fdf3aacec031ebfca71b746`：
313527个原始ID，预期313502pass/25历史optional skip；原313400的ID/动作全部保留，127个新增全部要求pass。
131084个required继续要求全pass/0skip；ASDU原两worker的10161帧与历史平台例外均保留。
471-01因旧混合包快照的CLI后来变化而拒绝，最终用475的完整当前源证书生成，未排除依赖输入。
最终必须核验官方产物大小/SHA、上传摘要、实际checkout以及全部必需CI，完成后由PR描述记录链接。

## 保留缺口

[当前M2清单](shark-m2-correctness-checklist.md)为54项中44个有限完成、10项未关闭（含3个SMB搁置）。
[非SMB清单](shark-m2-non-smb-differences-20261031.md)为17包中10有限完成、7开放，粗估25–50个AI实施小时，
不含构建/CI/回放等待及新发现。HTTP永久缺请求的边界、TLS1.2未确认开连接政策、缺握手QUIC、
其余TCP/UDP分区会话政策、完整TLS/PHA/上下文/协商状态以及IP有界碎片重组仍开放。
SMB缺口按[任务书](shark-m2-smb-taskbook-20261101.md)保留，未进行新的SMB语义分析。
