# M2：CertificateRequest 正文、来源及同帧摘要

本轮只关闭明确的正文、字段及页面合同，不关闭整个 TLS/QUIC 会话正确性。
SMB 继续搁置；可交接的范围见 [SMB 任务书](shark-m2-smb-taskbook-20261101.md)。

生产提交为 `186bb2c147219e069aedb880a7f1b26255471657`，同帧列表修复为
`4f7bd2a1ea4b0b0aa351ba6b041086adce512e1a`。实际干净构建、真实捕获、
Windows 和有效 fuzz 的代码来源是后者。后续 `f99ef2b7ad97c135d9f2e49add582cf8b8e46e2d`
只连接已 squash 合并的 M1/main 基线，整个文件树仍为
`5d5fe68f492588f643654ba6020f5258e0e772fc`；不把旧收据改写为新 HEAD。
最终提交的完整 CI 另以 [阶段 PR #5371](https://github.com/yaklang/yaklang/pull/5371)
及仓库外精确 SHA 收据验收，本地通过不代替 CI。

## 已实现的有限 profile

原生 `tls` record/session 路径现在按已观察的协商版本解析 CertificateRequest，
QUIC 的 TLS CRYPTO 路径复用同一正文解析器：

- TLS1.2：证书类型列表、完整 uint16 签名算法列表、CA 向量及各 DER 名称。
  TLS1.0/1.1 的旧正文没有 TLS1.2 签名向量；这里只补其正文结构，不扩大解密承诺。
- TLS1.3：上下文、扩展总长、逐扩展 type/length；解析 signature_algorithms、
  signature_algorithms_cert 和 certificate_authorities。其他扩展保留明确的
  uninterpreted octets，不声称已校验全部扩展语义。
- 先校验完整握手长度、每层向量、重复扩展、必需签名扩展及尾部，再应用集合预算、
  构造拥有值。拒绝已知客户端方向的请求和明文 TLS1.3 请求；拒绝消息不提交连接状态。
  缺协商版本时保留 qualified envelope，不能拿 record legacy 0x0303 猜 TLS1.2。
- 11 个握手别名包含 type/length 和上述正文 literal。字段使用既有 `derived` 来源，
  不为跨 record 或解密后的字段伪造密文 PDU/物理帧范围；不把后续连接上下文当新消息。

TLS1.2 向量遵循 [RFC5246 §7.4.4](https://www.rfc-editor.org/rfc/rfc5246.html#section-7.4.4)
及已验证 [EID2865](https://www.rfc-editor.org/errata/eid2865)、
[EID2864](https://www.rfc-editor.org/errata/eid2864)，签名向量为 2..65534 且偶数。
TLS1.3 正文依据 [RFC9846 §4.4.2](https://www.ietf.org/ietf-ftp/rfc/rfc9846.html)。
算法名固定采用 [IANA TLS 参数表](https://www.iana.org/assignments/tls-parameters/)
2026-10-28 的已映射标识；未知名称仍显示原编号。TLS1.2 的 ECDSA hash/signature pair
不显示 TLS1.3 才增加的曲线约束。名称不是实际选用、签名有效或推荐算法的断言。

页面顺序为握手头 → CertificateRequest body → 列表/扩展 → literal；保留外层 record、
认证结果及观察到的连接上下文。完整握手拥有独立字节源，空上下文没有虚构的字节范围。
IPC 后重新核对编码、版本、认证条件及全部已发表正文值，矛盾元数据不能获得权威范围。

## 真实来源及逐字段参考

证据仍全部位于仓库外 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。
两份 241 捕获来自本机真实双向证书认证 SSL peers；不是手写密文。

| 来源 | 物理帧 | 捕获 SHA256 | 完整请求 |
|---|---:|---|---|
| `mTLS-CertificateRequest241-tls12-01/live.pcap` | 19 | `b4d678871c7074ce1f465ac82b50a44a1c48442a1df44c8e8e153d2b1e3e1c34` | frame6，52B 握手/48B 正文 |
| `mTLS-CertificateRequest241-tls13-01/live.pcap` | 17 | `deadc7ec4bb9672ffeb96046565ef02b69009cd9c7d18b2258a2c113696c6999` | frame6，53B 认证握手/49B 正文 |

独立 244 在产品正文实现前读取 wire、重组 TCP，并独立验证 TLS1.3 SHA384/HKDF/AES256-GCM；
它没有执行 Go、SDK、UI 或 TShark。原始 245 是事后字段适配审计，不能称盲测。
290 使用同一冻结来源及新构建重新执行 SDK，并重新运行 TShark4.6.9：两份完整捕获、
隔离配置、显式将本机已知服务端口 Decode As TLS、授权 keylog、乱序重组开启。

结果为 56 个 source facts 全匹配，其中 55 个参考 literal/范围精确一致，1 个空上下文由
literal 零长度明确表示。两消息各 26 个此前未发表的正文事实已发表；这是补覆盖，
不是修正 52 个错误值。两边真实 HTTP request/response 与 peers 的原始字节仍完全相同。
TLS1.3 范围是拥有明文的局部坐标，不是物理密文位置。CA 扩展缺席没有变成 CA 长度零。

`mTLS-CR-native290-02/summary290.json` SHA256：
`61cb6b041d5faaeffe56c20f023fbedf38638669d2307f5bb1072c5e0b27869b`。
此 profile 没有验收真实非空 CA、post-handshake authentication、完整握手顺序、
上下文唯一性、签名或 Finished HMAC；这些仍是开放项。

## 本地测试、Windows 和有效 fuzz

| 收据/阶段 | 实际结果与范围 |
|---|---|
| red273 | 测试误把 ProtocolErrorKind 当 error，编译失败；不能作产品红测 |
| red274 | 修正 typed-error 断言后 11 fail，复现只发表 envelope、坏内部向量未拒绝 |
| green275 / boundaries276 | 11 pass / 142 pass，正文、拥有值、预算、DER/扩展、来源与 worker |
| complete278 | 两包完整回归 238780 pass / 12 历史 optional skip / 0 fail，528.609s |
| final280 | 正文、所有分片位置、QUIC gap/乱序补齐/重复/冲突、eager/deferred、全新连接和字段拥有性，共147 pass |
| row red286 | 2 pass / 4 fail，复现同帧多个完整 PDU 或同 record 多消息挤掉具体请求 |
| row controls288 | 完整 CLI 1093 pass / 2 历史 optional skip / 0 fail，包含反向顺序、错误/未证明正文不得前移、KU 优先级及应用身份 |

以上测试执行时实际 HEAD/dirty 输入按原收据保留，之后才提交；没有把测试前的 SHA
改成产品提交。Go1.22.12 Windows/amd64；Go/SDK/UI 顺序运行，Go 构建和测试使用
GOMAXPROCS=3、GOMEMLIMIT=2GiB、-p1 环境目标，不冒称 Windows Job 硬上限。

Windows 原生控制台实际跑出 36 个状态：TLS1.2/1.3 两页、鼠标字段选择、Fields/Bytes
切换、80×30 与180×48缩放、CP936恢复。根 agent 已实际查看两页选中图及窄屏图。
完整 52/53B 握手及长度高亮分别为 `[8,10)` / `[11,13)`，与独立244一致。
初次导航只滚16行而未到同帧后面的请求，该失败保留；随后按真实 worker 树定位。
第二次真实页面确实暴露列表前三个完整PDU的摘要上限：正文已正确，但请求被挤掉。
4f7 修复后保持原数据和断言，列表现在以 Certificate Request · 20 signature algorithms 开始，
完整详情仍保留原消息顺序。错误诊断及更高层应用身份不被新摘要覆盖。

`Windows-CR292-01/closure293.json` SHA256：
`39f7dbac33377b56f70fa63c90bbbb6003a8cc8c0ff5250b6c618bfe7dd2f136`。
这是 console cells/attributes 验收，不是 Terminal 像素、字体、DPI 或 IME 全覆盖。

第一次外部 fuzz281 工具误加 `-test.fuzzworker=false`，递归启动协调器，虽然返回 PASS，
3 个 baseline 均未完成；该轮无效，未计入 fuzz 验收。已清空核实为本次启动的进程。
纠正的 fuzz296 复用相同插桩二进制，删除内部 worker 参数覆盖；Windows Job 采用1CPU、
总768MiB、最多2个同时进程及 KillOnClose，独立 TEMP/TMP。第三进程拒绝正控通过。
实际3/3种子完成，60秒 81017 exec，47 new interesting，0 fail；峰值 Job 317059072B，
退出后无测试进程。所有 corpus、共享文件及完整导出留在仓库外。

用户授权的唯一磁盘维护 agent 按固定清单、独占句柄、文件身份/时间/40B头哈希复核，
清理417个本次遗留的共享临时文件，释放实际allocation 43727327232B；不改pagefile或用户服务。
删除前清单 SHA256 `affe91e6f882d1ac9a14a12cf975525add8389de9804b6bc1b048f90c016abbd`，
结果 `7f653fed28c8ed20bb161d20c7bdc810efbbe5e0e1dfffd576e77169c7da43bc`。

## 两个完整大捕获的回归与新参考

4f7 的 SDK、flow、CLI 共6个完整回放全部 exit0，源码、索引、输入保持不变。
Weber 使用明确授权的外部 keylog，Sharon 无外部 secrets；嵌入 secrets 均忽略。
flow observer 的128MiB是完整输入上限，超限拒绝，不是截取128MiB来冒称全捕获。

| 数据集 | 物理帧 | SDK PDU events | TrafficFlow / callbacks | 新增并核对的别名值 |
|---|---:|---:|---:|---:|
| Weber | 51328 | 42223 | 824 / 14172 | 742 |
| Sharon | 328127 | 263109 | 840 / 7700 | 352 |

与独立保留的231旧非SMB分支输出比较：两个完整 flow JSON 逐字节一致；所有既有非SMB
SDK值、ID、raw、来源及已映射字段保持一致。新增别名逐值核对当前完整握手，随后验证
所有 packet.pdus、packet.published_pdus 和独立 publication-sidecar 与原生PDU ID对应，
再仅排除这些已验证的新增字段做旧值回归；未重编号。Weber既有CLI值0差异；
Sharon有两条明确的 version-unavailable envelope 提示变化，另1条SMB差异按用户要求搁置，
没有分析其正文或弱化断言。大包中没有新增的已知版本 CertificateRequest 正文验收样本；
两条缺版本 envelope 不能代替241真实来源的正文验收。

回归摘要 `CR-public-comparison298-05/summary298.json`：
`08d53e7e6a3da50921b2df4c45530267d6abcd7d4535bfd245a16ff3f6a26604`。
这是输出保持合同，不是与 Wireshark 全部协议语义一致。

新 TShark4.6.9 以独立空配置、两遍、无display filter/Decode As、乱序重组开启，
完整读取两捕获；Weber授权keylog与原生配置对应。512MiB首次碰到明确GLib分配失败并保留；
在既定资源范围内改为硬1GiB/2CPU/1进程后成功，没有改样本、字段或断言。
`reference-CR-public297-02/summary297.json`：
`780acc539a1e601366f6e9413e21cce614e45ae2d02af052fbe4942979fe7bb3`。

| 数据集 | 字面列表Protocol相同的物理帧 | WS tcp.stream / udp.stream | 原生TrafficFlow |
|---|---:|---:|---:|
| Weber | 38703 / 51328 | 641 / 2416 | 824 |
| Sharon | 39777 / 328127 | 35420 / 9855 | 840 |

这些口径不同，不将字面列匹配率当协议正确率，也不将 udp.stream 当应用会话。
例如 Weber 有2205条 STP/RPVST+、864条 RMCP+/IPMI 的显示层级差异；Sharon还有大量
Fragment、GSMTAP/UDP、SCTP承载的专用协议差异。已知尚无IP碎片重组需要显式列为
M2-45/N14；各候选组是否完整、是否冲突、是否能合法恢复，必须先做独立原字节台账。
不能把所有 Fragment 列差别都宣布为可恢复包或 parser bug。

`CR-column-comparison300-01/summary300.json`：
`17c1a6a95e69011575a24994301d6ee15c923f961ddcd7c05ab5b09ca40692cb`。
其逐包列配对和原始frame-set分区都是诊断证据，不冒称全部字段、流或会话已等价。

## 当前验收边界和后续

在读取本轮CI结果前已冻结旧M2的312752个ID/终态及161个新增ID，预期共312913个ID，
312888 pass、25个既有optional skip；131084 required仍必须全部pass、0skip。
冻结清单 SHA256 `ebace46f803740d0b17b28166efdb2707e689a74b4416efeedd340d097683fa2`。
最终SHA必须检查真实checkout、全部工作流/必需状态以及完整traffic artifact，不能用旧绿灯替代。

N13仍开放：完整消息阶段、PHA/上下文唯一性、Finished长度与epoch、缺握手资格、
丢失/乱序后的恢复和连接复用；TLS13证书验证、所有扩展与QUIC完整语义未验收。
后续先复现 Finished/应用epoch边界，再按N14独立来源细分IP碎片接入；
N07/08/09/10/11及3项SMB搁置仍保留。当前数量、剩余AI实施粗估以
[主清单](shark-m2-correctness-checklist.md) 和 [非SMB清单](shark-m2-non-smb-differences-20261031.md)
为准，不能从通过测试数推导系统完成率。
