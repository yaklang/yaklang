# M2-43 / N13：TLS 身份资格与 CertificateRequest 正文缺口

从已全绿的 `26bc40ff20b9c4e88313c578895be0dd1f3400f6` 创建
`codex/shark-m2-tls-admission-audit`；不修改已ready的[列表修复 #5383](https://github.com/yaklang/yaklang/pull/5383)。
SMB按用户要求跳过，[缺口记录保留](shark-m2-smb-deferred-20261031.md)。
本轮完成有限来源审计，未补原生正文实现，不关闭M2-43/N13。

## 大捕获疑点

Sharon原捕获SHA256 `aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`。
225608、309343两帧均接口226、无VLAN，原IPv4 Total Length字面值0。
独立提取保留0：严格IPv4长度路径拒绝，另列捕获extent兼容解释；不把Wireshark的
presumed-TSO推算值回填原头。完整捕获双向tuple普查中两个tuple各仅一帧，
没有该普查提供的hello/建立握手证明，不能推断协商版本或认证状态。

同一Wireshark4.6.9、空配置、完整328127帧原捕获解码后才筛选目标，未强制Decode As：
**两帧默认也显示TLS，同时报告结构错误**。不能认定为已证标签误识别，也不能直接改纯TCP。

| 帧 | 独立源事实 | 仍需处理 |
|---|---|---|
| 225608 | 完整record38B，Type22/legacy version0x0300/length33；Type6/length0后29B为不完整握手，随后23B不是有效record header | Wireshark兼容显示HelloRetryRequest并报vector越界；历史类型/版本支持、record framing、正文合法性、连接确认须分列 |
| 309343 | 完整record53B，Type22/0x0303/length48；Type13声明正文41B，后3B属下一个握手片段；随后5B为坏record header | 按TLS1.2正文布局，CA声明70但当前消息仅余13B；不得借相邻3B填充。参考也报截断；不能由record legacy version断定协商版本 |

240另保留legacy/TLS1.3布局检查，未假设已协商TLS1.2；其3正/6负控仅验证辅助读者。

## 新抓的真实双向认证流量

实际本机loopback TLS客户端/服务器均要求并验证对端证书，使用本次生成的测试CA；
dumpcap抓取，无手工密文。私钥、证书、keylog、PCAP及完整导出全部留仓库外。

| 项目 | TLS1.2 | TLS1.3 |
|---|---|---|
| 物理帧 / 捕获字节 | 19 / 5255 | 17 / 6598 |
| 捕获SHA256 | `b4d678871c7074ce1f465ac82b50a44a1c48442a1df44c8e8e153d2b1e3e1c34` | `deadc7ec4bb9672ffeb96046565ef02b69009cd9c7d18b2258a2c113696c6999` |
| wire选择套件 | 0xC02F | 0x1302 |
| CertificateRequest编码 / 正文 | 52B / 48B | 53B / 49B |
| 独立来源 | 明文，Authenticated=false | 实际SHA384 HKDF/AES-256-GCM，精确序号/头AAD认证，Authenticated=true |
| 来源typed事实 | 28，含3证书类型、20算法值、字面空CA向量 | 28，含空context、扩展头、20完整u16 SignatureScheme；CA扩展不存在 |
| 当前原生输出 | Type、Length、Authenticated；completeness=envelope | Type、Length、Authenticated；completeness=envelope |

两次实际SDK均exit0，分别还原与peer文件逐字节相同的一组HTTP请求和响应。
运行时工作区26bc；events构建来源9af，两者之间只有3个Markdown文件，不冒称26bc重编。
events二进制SHA256 `516b0c3f4eeed0a00ac70d82136473d3e75477cf7cd30912c7410423ef3f0d03`。
TLS1.2复用已实际闭合的242 SDK，TLS1.3实际运行243；复用不写成重跑。
最初242辅助读者编码名`utf8-sig`拼错，243修为`utf-8-sig`；原失败保留，协议预期不变。

独立244在执行前冻结源计划，从原PCAP重建SYN代次/方向/TCP字节、wire CH/SH与ClientRandom，
未读242/243候选或参考。TLS1.3独立认证握手及双向应用明文；TLS1.2只独立核明文请求，
不扩张为独立应用解密。7源负控实际拒绝：两握手截断及TLS1.3错序号/tag/AAD/key/截短密文。
AEAD成功不等于证书信任、Finished HMAC已验证或全部TLS状态正确。

root随后按245后验映射逐项核244与243：**55个字面值及归一化范围一致，
另1个空context由字面长度0确认，0来源/参考差异**。TLS1.2物理起点1312；TLS1.3范围属于
独立拥有的解密握手，不映射到密文offset。4个字节/范围辅助负控拒绝。
映射写于已观察原native/参考之后，明确不是盲验产品门。

完整参考树71/70节点含容器及legacy字节视图，不能直接计为71/70个缺失字段。
TLS1.3按完整u16 SignatureScheme保留，不把legacy hash/signature字节拆分当协商算法语义，
不凭未知数值命名。字面空CA与不存在CA扩展分开。
原生头值和认证标志匹配，但合计52项正文typed事实未发布。原生已明确标记envelope；
结论是**正文覆盖/校验缺口**，不能写成52个错误值或已证TLS误识别。

## 后续验收门

1. 可信协商版本选择正文布局；无上下文保留可观察信息和缺失原因，不用端口或record legacy
   version猜版本，不把envelope冒充完整语义。
2. TLS1.2证书类型/签名hash向量/CA及DN、TLS1.3 context/扩展/SignatureScheme均限于当前正文；
   检查长度、奇数字节、重复/截断扩展、消息尾。拒绝时连接、密钥与方向状态不能部分提交。
3. 跨record/TCP完整重组握手拥有字段源；TLS1.3不借密文范围，认证/来源分列，保留未知值及原DN。
4. 保留两个真实正例，补非空CA/context、后握手认证、未知扩展、分片/乱序/缺失/复用/域隔离；
   坏41B消息不得借相邻3B通过。
5. 原生字段、SDK alias/来源、递进详情及实际Windows页面逐项验，再核精确SHA CI。
   本轮无新原生实现、产品fuzz或Windows正文页面验收，不关闭以上门。

N13后续实施粗估3–6 AI小时，加原N07–N11的13–26小时，合计16–32小时；不含执行等待、
新增问题或其他协议。主表仍43项：34有限完成、6进行中、3SMB搁置；N01–N13仍7完成/6开放。

## 外部证据

根目录`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`，完整捕获、密钥和导出不入库。

| 证据 | SHA256 |
|---|---|
| `TLS-admission-source238/contract238.json` | `98b76fafaa64d56bf2a1cdda6edb3e4e00f6839fae8117f5e5ae043350e8f930` |
| `TLS-admission-reference239-01/summary239.json` | `f173a86d67817f297063cab27b4f9575169a282675d02ce9ec373434ce61c503` |
| `TLS-admission-body240/proof240.json` | `a33c052fe2d2c3bc6e6ab80ddcf559aeba9a178ac1232eecf416a4b98228ddf2` |
| `mTLS-CertificateRequest-replay243-01/summary242.json`（243新目录保留旧文件名） | `d40c6ee4c8432a6ec26694ec5c023b11782010a1e7fb5d4bb112771fd6a35ed4` |
| `mTLS-CertificateRequest-source244-01/receipt244.json` | `94646cba9768a6fce53939f75933a26d2b7fcb817fc71411679c356dd9e62e92` |
| `mTLS-CertificateRequest-field-audit245-01/summary245.json` | `f057a2a3b0b1e896fb68badfe66f619353920891307125576e51b97bf9f4d4e7` |

布局依据：[RFC5246 §7.4.4](https://www.rfc-editor.org/rfc/rfc5246.html#section-7.4.4)、
[RFC8446 §4.3.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.3.2)。
只覆盖上述实际版本/输入，不宣称实现后续全部TLS规范。
