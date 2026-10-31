# HTTP/3 缺失开头、QPACK 字段恢复与 LSA 完整性

`7e0220ccfe53268077444318295e8aa6cda900d7` 修复已认证 h3 连接的新客户端双向流在
开头缺失时退回 QUIC 的展示。它只标明已确认的连接和 STREAM 身份，明确显示
`stream prefix missing · awaiting reassembly`；不创建 HTTP/3 framing 状态，也
不填造 HEADERS、method、事务或请求锚。单向流仍须观察到类型前缀，未知 ALPN
和新连接不能借用旧身份。开头缺失时也不排除未观察到的 WebTransport 扩展。
依据：[RFC9114 §6.1](https://www.rfc-editor.org/rfc/rfc9114.html#section-6.1)。

同一提交补齐 QPACK 解阻塞后已成功解出的 method/status 字段投影。独立 pylsqpack
确认原始字段 section 在编码表插入后解出 GET；实际 parser 测试先复现旧遗漏，
修复后只在完成解码的 PDU 发布一次，保留原始压缩块的字节来源，后续重复/缓冲
片段不附上旧请求头。筛选器的否定谓词只针对提供的 PDU 中新解出的字段，不代表
整个 stream 没有请求。该动态 QPACK 测试是原始字节控制，不冒称新抓的真实流量。

| 已实际运行的范围 | 结果 |
|---|---|
| 新 Windows TLS/QUIC | 2245物理记录、2220 PDU、41700私有字段occurrence，物理与标签差异0 |
| 新流量完整对象 | 仅2个源包增加身份/缺失开头元数据，其余对象逐字面保持 |
| 37输入公开语料 | 307262完整native事件、2完整flow、384518完整CLI行均逐字面保持 |
| 4个QUIC恢复输入 | 7693事件；47个身份修正有独立来源，其余7646完整对象保持 |
| 2个TLS整记录丢失控制 | 360完整事件保持 |
| HTTP/3粘性fuzz | 60秒、136113次、0失败 |

QUIC 恢复控制重新从 literal ODCID 解 Initial，读取 CH/SH 并解密服务器 Handshake
中的 EncryptedExtensions ALPN，按认证 CID 和实际到达顺序推导缺失开头的源槽。
没有借用 Wireshark 的连接号或旧 native 的 ALPN 作为身份依据。47个修正均绑定
raw摘要、方向、CID、SID、偏移和已认证 h3；STREAM字节、重组内容、flow、事务、
response_to及所有其他值不变。10个错误CID/方向/ALPN/虚构HEADERS或事务等反例均
被拒绝。这里含真实流量及明确的离线丢弃/损坏控制，不是6次新抓的现场故障。

Windows 默认20000/64MiB实际运行选中 frame1011/PDU1021：协议列与详情均为
HTTP/3，层级为Loopback/IPv4/UDP/QUIC/HTTP/3，摘要说明等待重组。CID的PDU范围
1..9、物理范围33..41逐字节与独立来源一致；视口中的物理/QUIC来源前缀准确，
没有声称1200字节PDU全部同时可见。鼠标tab、缩放及CP936恢复核验通过，主agent
亲自查看实际控制台cells渲染；它不是Windows Terminal像素截图。原始proof的
visual-review-pending标志保持不变，后续人工查看另有摘要。

原本完整本地回归的285571通过/24跳过/3失败保留原结论：一个旧fuzz及两个seed
不允许已认证h3连接的角色判断。更新该合同后90项定向检查通过，未将旧失败改名
为成功。7e的 [Essential](https://github.com/yaklang/yaklang/actions/runs/40868209040)、
[显式完整SHA流量](https://github.com/yaklang/yaklang/actions/runs/40868252006)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/40868209044) 和
[路径大小写](https://github.com/yaklang/yaklang/actions/runs/40868209058) 均通过。
自动草稿流量40868209077为skipped，不算通过。

随后 `5c608d4a642731a665feab7acbcd1feb8439461b` 修复独立复查发现的LSA校验漏洞：
checksum字段0000即使Fletcher残数C0=C1=0也必须拒绝，合法ffff对照仍通过；不能
把父包校验正确当作LSA正确。旧7e原生程序实际错误接受0000，新二进制实际改为
malformed且保留原字段，ffff仍decoded，两个父包校验均true。113项相关检查通过。
两大包339条完整LSA原字段均非零。5c再次实际回放37输入，307262完整native事件、2份flow及384518完整CLI行与7e逐字面保持；新的OSPFv3 fuzz为60秒、145457次、0失败。规范依据：
[RFC2328 §12.1.7](https://www.rfc-editor.org/rfc/rfc2328.html#section-12.1.7)、
[RFC5340 §4.4.1](https://www.rfc-editor.org/rfc/rfc5340.html#section-4.4.1)。

外部构造器最初遗漏既有的三个HTTP/3身份别名、CLI适配器空格错误、首次Fletcher
控制器keylog占位错误及单测的错误文案断言各自保留。更正检查器时不改源值或
物理范围，严格比较不忽略字段、不重编号、不归一化对象。

原始捕获、keylog、完整case/plan/期望对象/导出、PDML、日志、程序和图片全部在
仓库外；归档只含源码与计数/摘要引用。详见[证据索引](recovery-http3-7e022-summary.json)。
OSPFv2 的556帧来源/参考核验尚不能替代其native/canonical/字段页面接入；其他
TOP30高级边界仍继续开放，没有宣称核心协议已经完备。
