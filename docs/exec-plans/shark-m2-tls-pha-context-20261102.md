# M2：TLS1.3 PHA offer、请求 context 与 Windows 诊断身份

本批代码和干净构建为 `25c342b86232636532fcf5978f9b17e6ffd1a057`，继续进入唯一 M2 阶段分支
`codex/shark-m2-protocol-correctness` 和 [PR #5371](https://github.com/yaklang/yaklang/pull/5371)。
main 基线仍为 `238faf87f36e9eff5ec1ea0ff06e8c06d473214c`。M1 #5353 已合并；本批不修改 SMB。
上一检查点 `9b0b8a3cc9af7a19aca39f9c363ca44b566f7bb0` 的 47 checks、2 statuses 和完整 traffic 产物已独立全绿。
本批最终说明提交需要自己的完整 CI，不能沿用 9b 的成功。

新增两个有限验收条目：M2-59 关闭 N13.P02 的 offer 和已观察请求 context 合同；M2-60 关闭
已认证 TLS1.3/TCP 握手诊断在独立详情页退回 TCP、以及新展示隐藏连接停止说明的问题。
当前父表为 60 项：50 有限完成、6 进行中、1 未开始、3 SMB 搁置，剩余 10 项。
非 SMB 仍按 17 包跟踪：11 有限完成、6 开放，粗估后续 AI 实施 23–46 小时，构建、CI、回放等待和新发现另计。
N13 子口径为 5 有限完成、2 未开始。完整阶段、响应关联、TLS/QUIC 高级恢复及所有核心协议完备性仍未验收。

## 原生状态和字段

完整、接纳后的 ClientHello 决定 PHA offer：明确声明支持、明确未声明和未知分别处理。
Final ServerHello 会释放用于 retry 校验的 ClientHello 正文；独立私有状态保留 offer，跨 KeyUpdate 继续有效。
被拒绝的 ClientHello 不能改写既有或未知观察。服务器 Hello 和 EncryptedExtensions 中的 49 扩展、客户端非空 49 扩展均拒绝。

TLS1.3 已知服务端的 CertificateRequest context 在连接内去重，包括已观察初始空 context。
历史键拥有字节，采用不可变替换；修改公开正文、编码或事件字段不能删除历史。集合和保留内存预算拒绝后，
请求不发表、context 不消费；已认证记录的序号仍按记录消费。新连接释放历史，接口/VLAN 域之间独立。
首次 application-key 请求的空 context 不被额外当成规范错误；其唯一性仍受已观察历史约束。

正常初始认证的公开输出保持；接纳的 PHA 请求新增 `Certificate Request Admission`：

| 自有字段 | 语义 | 来源 |
|---|---|---|
| `tls.pha.offer_status` | `offered` 或 `unknown`；明确未 offer 时拒绝请求 | context，无物理范围 |
| `tls.pha.context_status` | `unique among observed requests` | context，无物理范围 |
| `tls.pha.requests_seen` | 当前连接已观察 CertificateRequest 数，包括初始请求 | 64 位 context，无物理范围 |

这些值出现在本次接纳的请求上，后续记录不重复发表握手。未知 offer 的保守路径由消息级矩阵覆盖；
本批新实抓的两轮连接完整包含 ClientHello。缺 ClientHello 时如何获得独立可用密钥和身份，仍受现有捕获资格限制。
context 只在已观察历史内唯一；正文/记录认证不证明完整 transcript、响应关联、证书信任或 context 不可预测性。

## 新实抓、独立密码学来源和反例

`TLS13-PHA-source691-01/live.pcap` 是本机新实抓的 OpenSSL TLS1.3/SHA384 连接：41 物理帧、27 TLS 记录、
两轮 PHA、五轮 HTTP。原始捕获 SHA256 为 `fe2ef20cda53bb2a76e1d7c3e2dcf089fdc9340e21124ecf5da851cb94ed7993`。
独立 oracle 692 验证全部 23 条受保护记录 AEAD、4 个 Finished MAC、3 个 CertificateVerify 签名，
10 条明文与两端实际收发文件逐字节相同；两个请求 context 不同。
独立 oracle 不读取产品或 TShark 输出。该实抓不是三套件、HRR 或 KeyUpdate 的全部线上覆盖。

693 生成两组等长控制，保留全部 41 帧、时间、TCP seq/ACK 和记录长度，只替换指定字节并重算 TCP 校验和。
每组全部 23 条记录 tag 重新核验；变更后的完整交换不声称已被真实 peer 接受或 transcript 仍有效。

| 控制 | 旧版实际结果 | 新版实际结果 | 拒绝位置 |
|---|---|---|---|
| 正常两轮 PHA | 37 事件、10 HTTP、2 请求 | 37 事件、10 HTTP、计数 1/2 | 无 |
| CH 将空 49 改为未使用的 0xffa5，明确未 offer | 仍接受两请求和 10 HTTP | 16 事件、此前 3 HTTP；MalformedMessage | 帧/PDU 16 |
| 第二请求复用第一 context，应用密钥 seq=4 重新加密 | 仍接受两请求和 10 HTTP | 28 事件、此前 7 HTTP；MalformedMessage | 帧/PDU 28 |

新版保留全部物理行。拒绝前的所有事件和行逐对象比较；原始记录、身份、哈希、贡献帧和 flow 输出保持。
`CompletedIn` 按 API 只表示完整 decoded observation：Malformed 诊断保留精确贡献帧和哈希，不伪造 decoded completion。
规范依据：[RFC8446 4.2.6](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.2.6)、
[4.3.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.3.2)、[4.6.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.6.2)。

## 与 Wireshark 的实际差异

使用固定 TShark 4.6.9 `v4.6.9-0-g2d548b197c75`、空配置、两遍扫描、授权 keylog、显式 decode-as 和乱序重组。
694 在三个完整捕获逐字面检查 198 个 Hello/CertificateRequest 字段；TShark 仍展示两个坏请求，展示不证明其已检查 offer 或唯一性。

正常实抓的产品和两端都有 10 HTTP；本配置的参考只有 7 个 HTTP 包。705 从私有 debug 中只提取序号事实：
第一次 PHA 的 client Finished 在帧 20；之后参考在 22/26/32/34 帧尝试序号 0，独立验证的实际序号分别为 5、6、7/8/9、10。
参考 tag 检查失败，三个客户端 HTTP 请求未显示，服务器五个响应仍显示。该版本源代码的 Finished 分派会调用
application key 加载，创建新 decoder 并从序号 0 开始，支持这条复现路径。
[固定版本 Finished 分派](https://github.com/wireshark/wireshark/blob/v4.6.9/epan/dissectors/packet-tls.c#L3132) 和
[密钥加载实现](https://github.com/wireshark/wireshark/blob/v4.6.9/epan/dissectors/packet-tls-utils.c#L5974)。
范围限定为此捕获、版本和配置；产品继续保留已独立验证的 10 条明文。私有 debug 含密钥材料，留在仓库外。

704 另生成五组完整来源映射控制，每组 82 帧。原始 TCP payload/seq/ACK 不变，清楚记录接口或 VLAN 变换、贡献帧和时间。

| 控制 | 新版两侧 HTTP | 新版两侧请求计数 | TCP 代次 |
|---|---|---|---|
| 两接口，各完整连接 | 10 / 10 | 1,2 / 1,2 | 1 / 2 |
| 两接口，B 明确未 offer | 10 / 3 | 1,2 / 无 | 1 / 2 |
| VLAN 80/81，各完整连接 | 10 / 10 | 1,2 / 1,2 | 1 / 2 |
| VLAN 80/81，B 重复 context | 10 / 7 | 1,2 / 1 | 1 / 2 |
| 完整 FIN 后同四元组重放 | 10 / 10 | 1,2 / 1,2 | 1 / 2 |

最后一组是从真实捕获派生的代次控制，不称为第二次真实 peer 交换；另有 316 的真实同四元组复用正例。
706 对五组完整 TShark 输出核对 1,804 个帧、接口、VLAN 和 TCP 字面值。本参考配置把前四组跨域流量合成一条 stream，
只在 A 侧展示 7 HTTP/2 请求；完整 FIN 复用则展示两个 stream、每侧 7 HTTP/2 请求。
产品保留接口/VLAN 域隔离，并逐条验证 B 的错误不污染 A。这些分区和 PHA 解密差异完整保留，不把 stream 编号当连接真值。

## 恢复、复用和实际 Windows 页面

最终 766 做八组 SDK/CLI/flow 共 24 次实际运行；最终 759 对九个旧 profile 做旧新共 36 次实际运行。
原流、精确重传和乱序认证流均为 6 HTTP/1 PHA Finished；缺失、截断和损坏认证流为 4 HTTP/0 PHA Finished；
真实复用为两个代次、每代 6 HTTP/1 PHA Finished。TLS1.2 CCS/Finished 和初始 TLS1.3 mTLS 保持原输出。
重传不增加请求计数。除明确新增的三项 context 字段和 admission map，旧九 profile 的全部值、来源、ID、摘要和状态相同。

Windows 新发现两个问题：独立详情页对已认证但握手正文错误的记录仍回退 TCP；保留 TLS 标题后又隐藏了连接停止诊断。
现在仅对有当前连接、匹配原始记录/哈希/贡献域、已知 TLS1.3 套件和认证后 Handshake 观察的 MalformedMessage 保留 TLSv1.3。
当前消息仍为 malformed；同帧含已解码消息时保留其摘要和 mixed-record-outcomes。连接停止解码说明继续保留。
缺认证、缺版本、错误记录长度/版本、外国来源哈希、缺贡献者、跨域来源和其他拒绝均不能借此取得 TLS 身份。
这项页面投影消费原生认证观察，不独立认证外部构造的 DTO，也不扩展到所有 TLS/QUIC 诊断类型。

760 运行最终真实 Windows console/worker，三页共 66 状态：正常第二请求、明确未 offer、重复 context。
顶部和详情均 TLSv1.3，具体错误全文可见。正常页按正文→admission 展示；三个 context 字段无范围、无虚假字节高亮。
正常选中源为独立核验的完整 85B 握手明文，两个错误页各为完整 107B 原始记录；外层记录和独立明文源分别核对。
80×30/180×48、鼠标切 pane、Tab、worker、CP936 恢复及自有进程清理通过，主 agent 已看过三张最终页面。
图片是实际 console 字符和属性的渲染，未把所有 Windows Terminal 字体或整桌面像素列为已验收范围。

## 构建、测试和大捕获

Go 1.22.12/windows-amd64，构建 `-p1`、GOMAXPROCS=3、GOMEMLIMIT=2GiB；原两 fixture ZIP 未修改。

| 工具 | 最终 SHA256 |
|---|---|
| SDK events | `df4f333da8b255a3f33baaf1fff738a180f9ff8d889ec24969bd9bcd82d21cb5` |
| flows | `28fbc4e89b0b5753a1c95ef3cebf72965041b96787fa8cb9225c540f13dde2da` |
| yak CLI | `0d8dd1780a23ac47f6902da1f717eff560f2c398ad5625d7da9a6fb715020bf8` |
| 插桩 fuzz | `f6a2c12884e0fd7b4121c151078cb027ca08b74b1db763df2d71e147ecf68149` |

721 对 b20 最终原生/parser/CLI/scripts 四包完整运行：280,055 pass、23 历史 optional skip、0 fail。
其后仅四个 CLI Go 文件改变，746 对最终 CLI/scripts 完整运行：1,345 pass、2 历史 optional skip、0 fail。
唯一终态并集为 280,078 pass/23 skip，不冒称一个命令的结果；全部 native/parser Go、配置和 ZIP 输入保持。
新增 101 个验收 ID：75 native、26 CLI，均要求 pass。没有删除旧 ID、断言或修改 CI 以接受新 skip。

最终 761 的非空插桩 fuzz 为 29,777 次执行、八种子全部完成，1 CPU、768MiB job、最多两个同时进程，corpus/temp 均在仓库外。
此前 b20 的独立运行 732 为 76,416 次；本批最终门使用 761，不把两次合计当一次运行。

762 用最终三工具对两个完整公共捕获做六次回放，各导出与冻结的 b20/9b 后基线逐字节相同；最终实际重数为
306,380 native 事件、379,455 物理行和 580 publication。每次 3 CPU、2GiB、一个进程；所有 ID/raw/来源保持。
输入是公开项目的 Weber `The-Ultimate-PCAP.pcapng`（SHA256 `ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`）和
Sharon `ultimate_wireshark_protocols_pcap_220213.pcap`（实际 PCAPNG，SHA256 `aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`）。
它们不是 Wireshark 官方下载。公共回放证明本批回归保持，剩余字段、会话和 fuzz 差异仍按原清单验收。
大捕获、真实小捕获、密钥/keylog、完整导出、debug 和图片均留在仓库外，没有打进 fixture ZIP。

## 验收证据和最终 CI 门

本机证据根为 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。

| 文件 | SHA256 |
|---|---|
| `TLS13-PHA-oracle692-01/oracle692.json` | `2702c33acc80dfe540890acf6e46775ef0f364138292aad0053d9a061faa0af7` |
| `TLS13-PHA-offer-context-controls693-01/manifest693.json` | `81ae07d10c6a2dbabf3dbc1fb2f2d5aa3ebc4e642ae55a1bfeb92aae228724de` |
| `PHA-reference-sequence705-01/summary705.json` | `66f16569fd62926a9dccfdceba051032f69c812706f452b61df0552d548c3d0e` |
| `PHA-domain-reference706-01/summary706.json` | `eececa3c0111787af0fcdca077ad9f4ef4d3236aa98abe6920a03fe47afed140` |
| `current-PHA-context766-01/summary766.json` | `8c7a60a64b14cff743700073a6683c5ebac983e545840a6467b297643ab43d34` |
| `PHA-context-regression759-01/summary759.json` | `6781c44f928bb4062db481aafa82494e19af5a5c8615ffce03fb26087f3ff26d` |
| `Windows-PHA-context760-01/closure760.json` | `a68544ae12c9f19b87ea2764b954ed8dd0103ae67f761a21959865d92d91df09` |
| `fuzz-PHA-context761-01/receipt761.json` | `7641877b67e80ebdda78b25761b064afa5f14db021a2808bfb986ca328e7e81b` |
| `runtime-PHA-context-public762-01/summary762.json` | `1401357a5db0fef286e979eaa817b4f7c40429351af7711176b41c199e75d323` |
| `PHA-context-CI763-01/expected-test-IDs763.json.gz` | `7db2050e66fb64896b107e0b73937d1dfaa6c2d8c03703bb90cac01c599232c0` |
| `PHA-context-closure772-01/summary772.json` | `259516cd9a8ccd4996667f6b61849543ed8dc62d79bb04e85662f27a7c1ac965` |

763 在读取未来 CI 产物前固定完整 raw-ID/action 清单：313,945 IDs = 313,920 pass + 25 原有 optional skip。
101 个新 ID 和 131,084 个 formal required 都必须 pass/0 skip；完整保留 9b 父清单。
本地三 worker 与 CI 两 worker 的 ASDU 路径不同，但全部 10,161 frame 均 pass；CI 继续按原两 worker raw ID 比较。
Windows-only 控制另列；旧 GUID 测试自身的 GitHub Actions skip 条件未修改，保留 CI skip，而本机实际 pass。
最终说明 head 仍须独立确认 47 必需检查名、全部实际执行检查、2 statuses、实际 checkout 和官方完整 artifact。
CI 关闭记录写入阶段 PR；M2-11 继续开放，不把本批局部完成当整个 M2 完成。

原失败和未完成尝试均保留：708 实际暴露释放 CH 后 offer 变 unknown；737 的错误页暴露 TCP 回退；751 暴露连接停止说明丢失，分别由 b20、1d808、25c 修复。
703 的 IDB 校验位置、723 的构建 wrapper、730/735 的新增字段与诊断 completion 假设、733 的明文/外层源假设、758 的校验变量重名、738 的旧 CI 环境 skip 假设均另有更正证据，未算作产品成功。
768/770 的统计器错误要求 capture-close publication 自带物理行协议或所有未完成 PDU 都有协议；772 按实际独立 schema 计数，保留未知协议，三组总数均与冻结基线相同。
711 只运行了 fuzz 种子，不计作完整 TLS 专项；完整源门使用 721 和最终 CLI 746。没有覆盖旧失败目录或把前置失败归入最终通过统计。

后续继续 N13.P03 的多轮请求/Certificate/CV/Finished 关联和生命周期，随后 P04 的完整阶段、协商依赖及高级恢复；
N08、N09、N10、N11、N14 与 [SMB S01–S06 任务书](shark-m2-smb-taskbook-20261101.md)的未关闭项继续保留。

## 本批 CI 计时修正

`2fa472af1b3dde6e4c72e684d611124914d32892` 的手动流量专项已通过：
[实际运行 41346603001](https://github.com/yaklang/yaklang/actions/runs/41346603001)。
官方 artifact 为 16,969,896 字节，SHA256 `7c696244e34c21d031e9bbec529820e283243d3af5b4ae029a4edb8c5b47c971`。
775 按预先冻结的清单逐项确认 313,945 个 raw ID、终态和唯一 run/terminal，101 个新增项全部 pass，
131,084 required 项全部 pass/0 skip；旧 313,844 个 ID 和终态完整保留。
这不代表该提交全部 CI 通过：自动 Essential 的 `Test Utils Foundation` 中旧 `TestCache` 测速断言失败，依赖 gate 也失败。

`77ff501940562e4343185e585af87c1d8107a8ae` 只修正 `common/utils/gzip_embed/test/fs_test.go` 的计时方法，
采用 `testing.Benchmark` 校准操作数，再比较每次读取的耗时。原内容校验、读取错误检查和“至少快 10 倍”的断言原样保留，
没有修改生产缓存逻辑、协议实现、测试 ID、fixture 或 CI 跳过条件。
旧方法在 Windows 本机重复 30 次为 29 pass/1 fail，其中 29 次缓存批次测成 0ns；失败批次为 529,900ns 对 1,026,000ns。
新方法完整包为 30 pass/1 原有 skip；另行重复五次全部 pass，缓存为 36–45ns/op，非缓存为 12,940–13,800ns/op。
重复次数按原始 JSON 的每次 run/terminal 计数，没有用去重后的单个 ID 冒充五次或三十次运行。

本机关闭证据为 `cache-timing-qualification784-01/summary784.json`，
SHA256 `f84c3da5fd48596799244de8c88282cb363af703cc9934db3741e0194cd1cb75`。
相对 747 构建冻结输入，只有上述 `_test.go` 改变；生产 Go、配置和两个捕获 ZIP 均保持一致。
本文件前述 747/772 的“全部 Go 输入保持”描述对应当时历史快照；不将它套用到这次测试文件变更之后。
包含本修正的最终推送 SHA 仍须重新运行完整 CI；最终 SHA、官方 artifact 和检查关闭结果记录在阶段 PR。
