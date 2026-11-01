# M2：缺少请求方法的 HTTP 响应观察

本批代码及干净构建为 `c0b53eedab27bc5e4d0c30956d69b591b5d128a5`，继续唯一 M2 阶段分支和
[PR #5371](https://github.com/yaklang/yaklang/pull/5371)。M1 已合并；SMB 继续搁置，不作新的语义分析。
此前 `3d70333bab584719207e80283e3d385cc2905b82` 已独立通过 47 个必需检查、2 个状态和完整 traffic：
313502 pass / 25 历史 optional skip，131084 required 全 pass / 0 skip。
[traffic](https://github.com/yaklang/yaklang/actions/runs/41301299644)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41301302855)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41301306047)。
这些旧成功不替代本批最终说明提交的 CI；最终 SHA、实际 checkout 和完整产物由阶段 PR 独立记录。

## 范围和行为

缺少客户端请求时，完整响应头仍可以确认 HTTP/1.0 或 HTTP/1.1 及其字面状态码；
却未必能决定响应正文边界。HEAD、成功 CONNECT 和普通响应有不同规则，不能猜成 GET。
依据 [RFC 9112 §6.3](https://www.rfc-editor.org/rfc/rfc9112.html#section-6.3) 和
[RFC 9110 §9.3.6](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.3.6)。

- 原生完整头采用 `http-response-header-observation` profile，标明请求方法与正文边界未知。
  原有头部 Raw 和字段 API 保留，公开字段缓存或 Raw 修改不能代替私有已接纳的语义。
- 同次 Feed 的头后字节和后续实际到达的流字节分别保留。后续数据采用
  `http-unframed-stream-observation`，只表示先前已确认的 HTTP 响应上下文。
  不解析为新 HTTP 消息、不制造 method、response_to、transaction 或 CompletedIn。
- 预览最多 64 KiB，并服从 MaxMessage 与状态预算；另保留完整观察长度、SHA256、流偏移、
  来源类型及有界贡献帧/父 PDU。正文范围未知不等于字节不可观察。预算不足明确说明，不能无界积累。
- 新 TCP 代次、方向和捕获域的私有状态分别管理；迟到的孤立请求不会让此前不明边界数据重新同步。
  正常已知 GET、HEAD、CONNECT 的完整消息路径保持原行为。
- 列表和 packet/stream worker 显示具体 HTTP 版本及 header only / unframed stream 范围。
  详情依次为 Header、Unframed Stream、Context；头、未定界数据、物理帧和解密来源分别拥有字节。
  后续数据里的 `HTTP/1.1 204` 或 `GET` 不产生新的响应码、方法或消息边界。

关闭 M2-55 和 N07 的有限可观察性合同。原永久缺请求的完整响应数量/划分差异仍保留：
先前严格参考显示 6 个响应，产品只确认 1 个完整响应和未知方法的头。没有缺失信息时的完整划分等价承诺。
TLS/QUIC 全状态机、高级恢复、连接政策及其余字段差异仍按主清单开放。

## 来源、工具和独立参考

证据根目录为仓库外 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。真实捕获、keylog、密钥、
完整 SDK/CLI/flow/PDML 导出、fuzz cache 和页面均不入库，不进入两个捕获 ZIP。
Go 1.22.12 / windows-amd64；构建硬限 3 CPU、2 GiB、`-p 1`。
TShark 使用原有便携版，SHA256 `662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b`。
每次运行冻结源码、输入及配置哈希，输出目录不复用；运行期间不修改源码、index 或 HEAD。

`HTTP-header-source490-01/` 实际启动三个 loopback peer 并以 Npcap/dumpcap 抓取 45 个原始纳秒 PCAP 包。
分别为普通 GET、HEAD 后流水 GET、CONNECT 后 opaque 字节；双方实际收发内容逐字节和哈希核验。
捕获 SHA256 `1dc58f9392ff9d7db040649482613de94edc88241ac1fdecd64c52d9bfb74760`。
每连接 15 包；另外从原捕获只删除一条原客户端数据记录得到 14 包 orphan 控制输入，
其余原记录和时间戳不变。这是捕获缺失控制，不冒称新网络丢包。
GET 头 109 B、尾 63 B；HEAD 头 62 B、后续 54 B；CONNECT 头 39 B、后续 80 B。

`HTTP-header-reference497-01/` 执行六次独立两遍 TShark，空配置，明确 Decode As 实际测试端口。
GET 完整/缺请求分别为 1/0 请求和各 1 响应；CONNECT 同样；HEAD 完整 2 请求，
默认正文重组下完整/缺请求均 0 响应。`HTTP-HEAD-reference501-01/` 仅切换
`http.desegment_body:FALSE`，两个 HEAD 输入都显示 2 响应。默认结果仍保留，
不据此猜测 Wireshark 内部状态或认定其计数是缺失方法下的唯一消息真值。

两份完整公开输入保持原 SHA256：

| 输入 | 包数 | SHA256 |
|---|---:|---|
| Weber The Ultimate PCAP | 51328 | `ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4` |
| Sharon 2022-02-13 | 328127 | `aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c` |

在新导出之前冻结 `HTTP-header-public-plan500-02/` 的 54 个候选：Weber 29、Sharon 25。
`HTTP-header-public-source505-01/` 独立遍历全部 379455 条物理记录，核验完整头的唯一字节窗口、
接口、时间和 SHA256，不将跨接口参考继承当作同域来源。
`HTTP-public-reference536-01/` 又以完整原捕获两遍执行 TShark，空配置、无 Decode As/keys；
54 个来源帧全部发表对应响应头，216 个捕获长度/线上长度/版本/状态码检查通过。
这确认头部字段，不确认完整正文、请求方法或会话划分。

## 当前构建验证

`bins-HTTP516/` 为最终干净源码构建。CLI 237685248 B，SHA256
`6fb49ef510bc63b13387d5c48963e2157f2240a0b19cb467ffe9e4a3229455af`。
`HTTP-header515-full01/` 完整运行 pcaputil、shark-cli、scripts/ci 受影响包：
**239449 pass / 12 历史 optional skip / 0 fail**，467.625 秒。
日志 SHA256 `8e2a3b1fea3326641e8d809273eeb86c50370506ea5f22333a17bd867f7eb69b`。
新增 **82 个原始 ID（34 native、48 CLI）全部 pass**。包含 eager/deferred、1/2 workers、
整段/拆分/乱序重传、新代次已知 HEAD、公开 DTO 修改、预算和拥有来源反例。

`fuzz-HTTP520-01/` 使用带 Go 覆盖插桩的最终二进制，3/3 稳定种子、10788 次执行、0 fail。
硬限 1 CPU、768 MiB、同时 2 进程，峰值 347930624 B；超额第三进程负控拒绝，关闭 Job 清理拥有子进程。
范围是未知方法 HTTP 观察、别名和来源，不是所有 HTTP 状态序列。

`HTTP-header-samples518-01/` 24 次实际旧/新 SDK 和 CLI 对照：三类完整已知方法输入的导出逐字节不变；
三个 orphan 保留全部 63/54/80 B 未定界尾字节、不增完整消息、不从 opaque 内容产生 HTTP 别名。
`HTTP-TLS-loss524-01/` 再用此前真实 TLS 1.3 KeyUpdate 永久缺失捕获及授权 keylog 重跑最终 SDK/CLI，
原始 peer 的 602 B 独立正文与拥有的 89 B 头、513 B 尾逐字节一致；解密来源、原父子和贡献帧保留。
仍只有两个完整 HTTP 消息及一个头观察，原严格完整消息数差异不豁免。

`HTTP-public-admission525-01/` 的八次完整捕获实际 OnFrame 回调保留 1146 个双向 chunk。
`HTTP-wire-admission531-01/` 逐一从原始 IP/TCP 字节核验其中 1102 个 HTTP 方向 chunk 的
端点、序号、声明范围、哈希及 PacketRef；54 个头和 1048 个后续观察均有独立拥有来源，
共 1335496 B 未定界字节被保留，每段恰好一次。零 IP 长度不会伪装成有效声明边界。

## 完整对象和页面差分

`runtime-HTTP-public519-01/` 最终新构建的六次完整 SDK/flow/CLI 回放均完成，输入和源码不变。
`HTTP-SDK-comparison529-01/` 对照全部 305332 个旧事件：

| 输入 | 旧/新原生事件 | 头部观察改进 | 新 opaque 观察 | 额外差异 |
|---|---:|---:|---:|---:|
| Weber | 42223 / 42749 | 29 | 526 | 0 |
| Sharon | 263109 / 263631 | 25 | 522 | 0 |

新旧事件 ID 仅按显式 PDU 引用 schema 一一映射；HTTP/IPP、DNS、DHCP、RADIUS、BFD、FTP
的生成关联和 RDPUDP 原载体引用均从源码确认。线上 DNS ID、BACnet InvokeID、端点、flow、
domain 和 Raw SHA256 不转换。`HTTP-reference-negative535-01/` 七类负控全部拒绝，
故意与 PDU ID 相撞的 BACnet InvokeID 仍保持 7。未解释差异不会被统一整数替换掩盖。

`HTTP-CLI-comparison534-01/` 核验全部 379455 个物理行及 580 个原有 publication，
共 380035 行无遗漏或新增 publication。仅来源限定的 Weber 555 / Sharon 547 个物理行
从 TCP 改进为 HTTP/1.1 并显示 header only 或 unframed context；原物理字段、顺序、PDU 归属、
域、端点、transport/flow 及其他字段均保持。两个完整 flow 导出逐字节相同。
输出保留证明回归范围，不等于全部 Wireshark 语义正确性一致。

`Windows-HTTP526-01/` 最终构建实际运行两个控制台页面、35 状态：
GET 的独立 109 B 头中第二个 `X-Repeat: two` 精确高亮 [92,105)，不借第一个重复值；
HEAD orphan 的后续 54 B 作为自己的完整字节源高亮 [0,54)，显示先前头上下文且不产生 204 字段。
HTTP/1.1 列表、Header→Unframed→Context、packet/stream worker、鼠标/Tab、
80×30 与 180×48、CP936 恢复均通过。图片为实际 console cells/attributes 固定字体渲染，
没有宣称所有 Terminal 字体、DPI 或 IME 已覆盖。运行硬限及拥有进程清理均有回执。

## 保留的失败和 CI 门

原 capture490 资格器不识别纳秒 PCAP；491 在同一原始捕获上补齐格式识别，没有重造网络结果。
samples511 实际发现 opaque 的 `HTTP/1.1 204` 被 DisplayFieldValues 误发表为状态码；
c0 同时修复私有 alias 资格和 stream worker 名称，原断言不弱化，518 全部通过。
Windows522 的 QA 选择器定位到 IPv4 Version，原 54 B 全源断言真实失败；526 修正导航到实际字段，
保留全部字节/范围断言。SDK527/528 的生成引用差分保留；529 只加入逐项源码证明的引用 schema。
CLI533 的缺失辅助函数运行错误保留，534 使用冻结函数后全部原门通过。

`HTTP-CI521-01/` 在未来 CI 产物读取前冻结 313609 个原始 ID 与终态：
313584 pass / 25 历史 optional skip，父 313527 个 ID/终态全部保持，新增 82 必须 pass，
131084 required 必须全 pass / 0 skip。manifest SHA256
`a73e4305bf8dedd6ba1cae649e6d2882ca8819d6e4165a1dfdad4b4810d4fd9b`。
CI2worker/ASDU10161 原始帧 ID、已有平台例外和 Actions GUID skip 均不改变。
最终提交仍需 47 个必需检查名、两个状态、实际 checkout、官方 artifact 大小/SHA256/上传日志及
完整逐 ID 终态核验；skipped 或旧 SHA 成功不算新 CI 全绿。

完整证据摘要 SHA256：

| 证据 | SHA256 |
|---|---|
| 真实样本最终518 | `1ad4ffd90ce78d03ca23777d95791daf2bad60ebd983a70dbd91afcf841204af` |
| TLS loss524 | `80ad5fa9683127b5d0584b616f5739344f123994b082e126bd654fb4f04802c9` |
| 实际Windows526 | `91950cef3f83d0821bdd3d6ea1d9b1497c0f3038fca7e70d4e8ca4b6c0e204c7` |
| 大捕获六回放519 | `1551a515dbd10086de1189393904bc1281e10fa5b487176ab2ed03031f2726a6` |
| 实际回调原始来源531 | `aeb3f995743dc5ac216f3be8c9321f11760e4892b71298609fc800a738304297` |
| 全SDK529 | `681ba8bf420eca6397c05f66a729c303a85036f37f4b99502330d9f77f7d04dc` |
| 引用负控535 | `90412898e058a434b677c114628eb7d0b03eb58c0fef477c0869d79476be8b85` |
| 全CLI534 | `846a947719fa86fe39eff50723305c496b0a57538966c8ca20623ab111c362d4` |
| 新独立TShark536 | `60334603a6be741e3759a938da3e1ed9246fa72d44c211379e1187d1601c0358` |

两个捕获 ZIP 未修改，无散装捕获、外部大型输入或敏感材料新增。当前 55 个父项，45 个有限完成、
6 进行中、1 未开始、3 SMB 搁置，剩余 10；非 SMB 17 包中 11 有限完成、6 开放，
后续粗估 23–46 AI 实施小时，构建/回放/CI 等待与新发现另计。完整核心协议并未验收完备。
