# EE 拥有字段与 EOF 协议统计：2026-11-02

M2-71、M2-72分别关闭EE正文/来源展示和首个EOF统计窗口两个有限合同。仍使用唯一
M2分支及[PR #5371](https://github.com/yaklang/yaklang/pull/5371)，SMB继续搁置。
main `d705e2a1a0468fc7fda23f2b0fead3ebecd2c431`通过普通merge
`388138a341b869bd15a8d68f6c31b195bf8bd598`合入，无冲突、无历史改写。M1已合并。

EE核心代码为`a74ec14b92a0805a57f74ba43cb5f2b22bfa09f0`；`868e5c6`消除本条EE
ALPN与连接上下文重复显示。EOF修复、干净构建和最终本机验证代码为
`cf37880551ad2396286f80fbfdf34f0c654271bd`。后续仅文档的最终head须独立验收CI。

## M2-71：拥有的 EE 正文、字面字段和递进展示

根据[RFC8446 §4.2/§4.3.1](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.3.1)
及[RFC7301 §3.2](https://www.rfc-editor.org/rfc/rfc7301.html#section-3.2)，先验证完整
type8/u24消息及扩展vector、重复类型、预算和已知禁止位置，再发表字段。已知禁止集合为
5/13/18/21/41/43/44/45/47/48/49/50/51；此前EE资格层同步补上5/18/47/48。
ALPN必须为单个非空且边界完整的名称。其他扩展的Data无损保留，不冒称已验证全部语义。

原生事件新增拥有的编码正文、版本和结构视图。扩展长度、type、len和ALPN长度/原字节
发表derived alias，无密文字节范围；名称仅在合法UTF-8时发表文本alias，raw hex始终保留。
列表展示当前消息ALPN一次；详情为record/context→handshake→EE body→extension→ALPN。
右侧来源明确为认证后的握手明文、无原帧偏移。GUI/worker重新核对编码与已发表字段；矛盾、
未认证、缺原生QUIC carrier或错误加密级别不能借用这套来源。

独立公共来源1167从原始PCAPNG读取IPv6/TCP及接口243的物理frame49522；CH来自49519/49520，
使用独立HKDF/AEAD重建服务端handshake序号0，EE25B，SHA256
`537c83b762a50d17ebf1d69600eaf2850165b8e19a609565650a3b11cbb0411c`。
扩展types=[0,16]、lengths=[0,11]，ALPN原字节为http/1.1；名称span=[17,25)。
1177重新完整双遍TShark4.6.9、空配置、原keylog、明确TCP443 Decode As，七组字面alias
与独立来源一致。完整参考使用2CPU/1GiB硬限制；PDML SHA256
`289d27558c8e9cc6af73e7ac79efcd26b03c10840afbb46b9faeb0c2c38f1431`。

Windows原17帧mTLS来源241的EE21B由独立244来源验证，编码SHA256
`598ab59f1fcce1142d51cdb6647155b3d655ab3e8d84b7740393c0f211cb9ef0`，名称span=[13,21)。
最终1198重新执行正常捕获及三个独立重算AEAD/checksum故障的旧/新SDK、CLI、flow共24次。
正常仅EE字段和本条摘要有预期变化；未提议ALPN、禁止版本、禁止key_share仍拒绝，三个完整
导出分别保持；四组flow逐字节不变。派生故障不冒充新peer或合法修改后transcript。

## M2-72：首个 EOF 图表使用最终物理窗口

实际完整Weber Windows运行发现首个EOF图表仍停在上次定时采样，搜索刷新后才变为最终窗口。
修复在EOF立即刷新最后128个物理包的协议统计；支持晚到观察，保留live采样间隔，不制造
流量速率/计数，也不让搜索或pinned packet改变统计窗口。六个新ID覆盖最终批、晚到元数据、
过滤/pinned、空窗口及live间隔。

1196重新对整个51328帧捕获双遍TShark，按物理frame51201..51328读取128包及ESP SPI/seq、
IKE版本字段：ESP78、IKEv2 50。isakmp.version原值20，明确转换为IKEv2。参考PDML SHA256
`c871c97d46fbbbb8b49d9e897a7f3178eff57465dc247115f48610ddabb3abd5`。
最终Windows首个EOF图表即ESP61%/IKEv2 39%，搜索后相同；原分类器和数据导出不变。

## 验证、构建和实际 Windows

证据位于仓库外`C:\tmp\yak-shark-ESP-SSH-UI-20261028`。外部大包、原始帧、keylog/密钥、
完整JSON/PDML、fuzz缓存和截图均未入库；两个fixture ZIP SHA256保持。

| 验收 | 实际结果 |
| --- | --- |
| 核心完整1164 | 干净a74：289780pass/24历史optional skip/0fail；stdout `8bf56b66c82c38480b8a29502d0cb2c1007a7ffd5b261f18d82224a01e85c6d3` |
| 最终CLI1197 | 干净cf：1615pass/2历史optional skip/0fail；stdout `a38b8ffc871234973bbb736012b9cdeb58d6d4946713aa7fec1a0094aca10e8d` |
| 组件等价闭合 | a74..cf仅5个CLI文件，核心全部输入哈希相同；以最终CLI替换旧CLI终态，289786pass/24skip，原本地ID/动作保持。这不是一次在cf执行的完整套件；最终CI另验 |
| 新增ID | 本次实际编写53个EE和6个EOF，共59个，全部pass。并发worker子项和既有Windows环境项不算新增；父CI原始ID动作单独继承，最终产物逐项验证 |
| 插桩fuzz1171/1172 | 明确coverage构建，a74核心；EE admission6种子43036次、owned view4种子41362次。1CPU/总768MiB/最多2进程，第三进程拒绝正控通过；cf核心相同，不称fuzz二进制来自cf |
| 大捕获1190 | Weber/Sharon共306855原生事件、379926物理/独立发布CLI行逐条核对：相对4c仅SDK41537和物理row49522变化，新增EE结构/alias/摘要逐字节对上独立AEAD和新参考；其他行文字与字段保持，两flow字节相同。summary `30155f0dac82a386f99602fd2b4f53f758901be6248d6dae16608c69ddae0fb6` |
| 最终六导出1195 | cf重新执行两捕获SDK/flow/CLI共六次，与已资格化868全部逐字节相同；summary `72d75008e4be8c60ad561d0ad115575871a286856477480a2b764a73a0fe84d9`。输出保持不等于全部协议字段/会话与Wireshark一致 |
| 最终实际Windows1194 | 两个完整捕获输入17/51328帧，UI分别显示17/51.3k；169截图状态中135为等待、34为交互状态，不能算169个独立测试。21/25B来源和[13,21)/[17,25)精确高亮、80×30/180×48、鼠标/Tab、CP936恢复、全部拥有进程关闭，主线程实际查看页面。summary `ec45bbe45c86e83d80c920e00d7ece69711525ede7469f6e0860308438f675c6` |
| Windows资源 | 默认20000包cache；Go heap256MiB、进程硬上限512MiB、总job1GiB、最多8进程、1CPU。历史驱逐后的field filter partial明示保留；不称全Terminal/DPI/IME验收 |
| 综合1205 | `289b93303f96ea05e36853f96355a170765fa5d81b8462b94b9599843c87a148`；空闲28217483264B，高于8GiB底线 |

Go1.22.12，最终events/flow/yak二进制SHA256分别为
`c4920820b2815cdef3648d4c0b04d599835f29a79271543c8656f83250e0941c`、
`e9a047aa7919552dbaa001bbaa6d992025ae1d0993bc49d5369978b5d5960531`、
`40830082fa88d12178032dc83f7bfd3a62fbda7171535a305f94f02e52e9f446`。
公共样本为Weber The-Ultimate-PCAP（51328物理帧，SHA256
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`）及Sharon
ultimate_wireshark_protocols_pcap_220213（328127物理帧，SHA256
`aa4fcc81af216e2b788171c64271b43681974d9156aa9ddbdc1a7a25e730714c`）。
SDK完整回放3CPU/2GiB，其余回放2CPU/512MiB；所有完整输入留在仓库外。

## 保留失败、CI 和未关闭范围

1173错误参考pref、1176参考512MiB OOM、1178 GUI Go heap512MiB触及同值硬上限均保留。
1179严格摘要比较因已有多PDU footer归属失败，后续单独保留该footer再逐消息核对；1175
遇独立publication无top-level protocol，1190保留并比较这些行，不跳过。1203/1204汇总误把
本地与CI的并发worker命名差异算新增，1205仅选实际编写59项，既有断言与原始ID继承不变。

最终QUIC1199/1200四组真实来源域回放：interfaces/VLAN full/full各0strict差异及4556来源检查，
每域2022认证/3坏tag/6尾部；两组app-only仍各strict exit4/2664差异。1201各核验B的2007个
准确UDP预览，其中1332截断64B、675完整短载荷，来源不符和借用QUIC/H3均0。预览正确不等于
完整QUIC语义；N09恢复/上下文仍开放，没有枚举或猜测无关连接secret。

父`a7967c3d0a382338ac0bbe2dc2bb8f38845f203b`已独立47约定检查、91实际执行、2状态全成功，
314926原始ID/动作、102新增与131084 required全pass/0required skip；
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41420719964)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41420725680)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41420730937)。官方产物13130682198、17075272B，
SHA256 `4bb6e968b17a59c228d9a1c4083e4ba9d4a7dc323bc694e3f47cf05f3c420393`，闭合1160
`5789150997c388b60845d107ff48c6e21532c2be9e83138aaadaac141a80d2e3`。
新最终说明head必须独立验收314985原始ID、314960pass/25历史optional skip、59新增及全部CI。

现72父项62有限完成、7进行中/3SMB搁置、10未关闭。非SMB17包13有限完成/4开放，
N09/N10/N13/N14粗估16–32 AI实施小时，构建/CI等待与新发现另计。N13.P04完整阶段、所有
扩展依赖、证书类型/压缩、QUIC参数/恢复仍开放；N14.U04本窗口合同有限关闭，完整IP重组
字段/代次/冲突/缺片/预算恢复仍开放。SMB S01–S06由承接者独立复核，不以本批验证替代。
