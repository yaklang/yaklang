# M2-85：CONNECT 隧道帧序列、晚到上下文和递进展示

本项有限关闭 N09.R07：已确认成功 CONNECT 后的已知非 DATA 帧不能作为普通 trailers 接纳。
检查直接位于原生 STREAM 重组后的帧类型 varint，早于长度、payload 和 QPACK 解码。
它不认证完整 TLS/QUIC 生命周期、对端实际交付、连接关闭或 Wireshark 全部语义等价。SMB 继续跳过。

## 行为和信任边界

- 经过原生 QUIC 认证、当前 owner/域的客户端双向 STREAM，在有效 CONNECT 和最终 2xx 上下文中，
  已知 HEADERS 及其他核心/禁用 HTTP2 类型产生连接级 H3_FRAME_UNEXPECTED（0x105）。
  只收到完整类型 varint 就能报告；不依赖 QPACK、完整帧长度或 payload，避免残包被误作等待普通 trailers。
- 保留第一个连接错误，只隔离原违规方向的应用解析；其他 STREAM、原始 transport 观察和原字节不消失。
  DATA 继续遵守隧道语义，不作为普通 HTTP 正文计长。普通 GET trailers、非 2xx 和未知扩展不能误判。
- 方法/成功响应晚到时，有限保留第一条类型见证：最多八字节、来源引用受既有64项/连接字节预算限制。
  RESET、关闭、退休和复用不继承旧连接证据；接口/VLAN 域隔离继续作为验收门。
- 服务端同方向 STREAM 偏移能证明类型位于 2xx 后。客户端类型先于成功上下文被捕获时，
  跨方向偏移无法证明因果顺序，明确显示 order-unverified，不追溯制造连接错误或错误代码。
- 原发布事件、状态、物理来源与 ResponseTo 不变。后来合法请求仍是 decoded，新的发现携带原 Frame PDU ID
  与 Observed In PDU ID；Inspector 使用原有64项/字节预算和 revision 补充当前视图，私有槽标记不外泄。
  九项 http3.tunnel.* 别名仅从原生私有证据投影，都是上下文字段，不伪造密文偏移。
- Windows 列表优先显示 CONNECT 违规及两个观察号；详情优先显示错误/范围/结论，再展示大型来源树。
  类型字节有独立、拥有的 extracted source；只接受实际 byte 类型及 worker 类型描述符，不把 Base64 文本猜成字节。
  关联字段不高亮；点击关联字段保留当前明确命名的字节来源。隔离 worker 缺少私有身份时的 UDP 中间标题，
  由父进程已封存的原生 HTTP/3 身份回填，不采信公共 Authentication/ALPN 标志来提升身份。

依据：[RFC9114 §4.4](https://www.rfc-editor.org/rfc/rfc9114.html#section-4.4)。
Wireshark 4.6.9 在本次样本没有报告 CONNECT trailers 错误；参考的字段/字节相同不能推翻规范约束。

## 实抓和独立来源

仓库外 CONNECT-tunnel-source1763-01/live.pcap：Windows 自有 loopback、aioquic 1.7.0、完整和恢复两条实际连接，
复用 UDP socket，第二条实际接受0-RTT；请求半流保持开放。第一响应200 + 264字节DATA + 禁止的HEADERS，
第二响应200 + 264字节DATA为合法连接对照。PCAP SHA256：
`fe6e46465d3256cf938633ab8b248836415a273ef9fb8ab7a6075b9460561c21`。

候选修复前固定1764独立 AEAD/PN/CID/STREAM/frame walk/pylsqpack：21保护包、四消息、五头块/七头对。
独立来源摘要 SHA256 `868312898e6a18b92611c4222311a2b8c14b9514713c0303b323513dcc3a58da`。
1772三个乱序控制只移动相同21个保护包各一次，保留时间与原始保护字节，拆分coalesced时重算外层校验和。
它们是一次实抓的三个变换，不是三次额外实抓；清单 SHA256
`fd1a28d5c41731a00f8e91f3964c7d91722d480e1c3fb4d930e3a3f5bf6d9140`。

1821 最终 CONNECT 代码/展示检查点 b08d8993152e51694c7ea9055d467af69ecbb2b8：8次native/CLI、4次全新两遍Wireshark，
84保护包逐一比较，16独立消息、28参考头对，四违规目标和四合法复用连接对照通过。
成功 CONNECT 上下文已知时，被拒绝的类型之后不解 QPACK；整个原始 STREAM 明文字节仍与独立来源及参考一致。
方法晚到前已经完成的旧解码保持原样，后来补充违规结论，不追溯删除旧字段或改写发布历史。
摘要 SHA256 `6254b05ed62c9be5447df1955f7b89bc06fc43920f00b0d03fced9c7b19f271d`。

## 回归、页面和证据范围

新增92个固定 ID：分片/缺口填补、非最短varint、坏tag后有效重试、v1/v2三套件、方法/2xx迟到、
GET/非2xx/未知扩展负例、RESET/owner复用/域、64边/内存/歧义/淘汰、不变历史、fuzz模型和IPC字节来源。
17528次最终运行源六种子插桩执行，无崩溃；1CPU/768MiB/最多2进程及第三进程拒绝探针均有效。
1853插桩二进制与1854执行均基于最终运行源86b；原66f163e另有34303次早期执行证据，两次分开记录，不混为一次运行。

1841合并后旧实抓及十个控制33次native/11个新参考通过；353 QUIC字面、14对头/八帧和276控制断言均通过。
四组接口/VLAN严格控制 actual_mismatches=coverage_gaps=0。
1844 合并后两大公共捕获六完整 SDK event/flow/CLI 导出与上一验收逐字节相同；这只证明回归稳定，不能当全部字段正确。
1847 在86b合并后实际 Windows 可执行文件上完成五场景90状态，CP936/颜色、列表/详情身份、字段点击、类型01精确高亮、
180×48→80×30→180×48、退出恢复均通过；完整UI摘要 SHA256
`9510831b725a60595ba175001fd5b75400d7aac2d272d1b61c230795a0d9ded8`。

所有PCAP、密钥/keylog、原始导出、失败证明、插桩语料和截图均在仓库外；两个受控ZIP及既有断言不变。
原始失败记录保留：1748 authored缺口、1786 native红测、1791跨方向假确定性、1815字节源红测；
1813/1814手工DTO缺少私有身份/类型描述符，1825/1832对保留字节源的测试预期错误，也保留并明确不算产品失败或通过分母。

## 最终阶段合入与尚未关闭

远端新增4ccbc87927660735149efc779896c4328aad5556（#5387 WCF MC-NMF）在普通合并中完整保留，绝不强推覆盖。
合并后运行源86b622eab6baf836fcf5db94936aa495259667a4：重新构建、1840实抓/参考、1841旧源、1844六完整公共、1847 Windows和1854 fuzz均已通过。
1840摘要 SHA256 `0e44dc3ca64f29e5e30e91348cf21e74a7435b3f93ed4e68a8b869cb2d72bf4b`。
b08一次完整291224pass/25历史skip；合并后1369定向pass/1 WCF外部可选skip，完整CLI1719pass/2历史skip。
严格合并集合291408 ID = 291382pass + 26skip，所有旧291157 ID/动作保留，92新CONNECT均pass，继承WCF 158pass/1外部样本skip；不冒充86b一次完整运行。
最终说明SHA将另验316482原始Linux ID/动作（316456pass/26skip）和131084 required零skip，最终CI未通过前不标全绿。
资格1852 SHA256 `61e70c73b70165dbb85c1bfda387e83fcd638d46266c656a50c51ddaa3b07922`。
WCF真实企业样本的可选测试及其SOAP/Binary XML opaque范围不算CONNECT验收，也不算完成WCF全部正确性。

完整N09仍开放：永久丢失、跨连接/恢复状态、完整ACK因果和QPACK/QUIC生命周期；N10六TCP政策、N13完整TLS阶段/协商、
N14其余IP字段/重组及原94/39分类差异继续按清单推进。非SMB四开放包粗估17–34 AI实施小时，不含外部等待和新增问题。
