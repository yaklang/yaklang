# M2 QUERY_INFO 类型化输出的有限验收

分支 `codex/shark-m2-smb-query-info-types` 从已全绿的 M2 检查点
`f9faae9f82c0dbe6a657bfaa94d7d6acdc845380` 开始；不改变 M1 的验收范围。
初始实现提交为 `8015e90bcb`。本页保留初次实现的证据快照；
后续正文恢复、异步身份、真实回放与Windows页面的最终代码验证见
[0769构建验收](shark-m2-smb-query-output-validation-20261031.md)。
下文“后续待验”仅描述初次快照，不覆盖该后续记录。来源准备不等于产品验收。

本批只支持以下两种完整输出，不把整个 M2-04.09 或 SMB profile 标成完成：

| 请求选择器 | 类型 | 本文的有限合同 |
|---|---|---|
| InfoType1 / class7 | FileEaInformation | 恰好4字节，uint32 EA Size；不是 EA 名称/值列表 |
| InfoType1 / class5 | FileStandardInformation | 恰好24字节；有符号64位 Allocation Size/End Of File、uint32链接数、两枚原始 Boolean 字节和 Reserved |

布局以 [MS-FSCC FileEaInformation](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fscc/db6cf109-ead8-441a-b29e-cb2032778b0f)
和 [FileStandardInformation](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fscc/5afa7f66-619c-48f3-955f-68c4ece704ae)
为准。[Boolean](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fscc/8ce7b38c-d3cc-415d-ab39-944000ea77ff)
任意非零字节都表示 true；保留 literal 字节，解释值没有额外 wire range。
Reserved 可为任意值。EndOfFile 必须非负；没有卷的 cluster size，不能证明 AllocationSize 的倍数约束。

响应本身不携带选择器。原生路径只接受同捕获 section/interface/VLAN、同连接代际、
反向且 MID/SID/TID 或合法 AsyncId 匹配的已观察请求。没有这些上下文时，
熟悉的4/24字节长度仍然保持 opaque。请求类型名称是 context，不伪造响应字段范围。
canonical 两个显式入口供调用方明确选择，不自动推断独立响应的身份。

递进树在 Header → Body 中保留响应 envelope、命名的输出类型、其精确字节字段和原始
Output Buffer；Observed context 与原始字段分开。compound 使用已有全 PDU Output Buffer
起点，只加输出内部偏移一次。相同值出现在不同命令时仍保留各自的来源。
完整 envelope 内的坏类型化记录不抹掉 SMB2、原始输出或其他命令；残缺的 declared buffer
不能消费 pending。完整 final response 中坏 typed payload 只完成它自己的 pending，
不会借给后来重用的 MID。BUFFER_OVERFLOW 的 partial 状态单独可见。

本地 `test-M2-QI-types66-02` 的4包选定矩阵为 **401 pass /0 skip /0 fail**，源文件、索引和输入
在执行期间保持不变。覆盖 canonical/native 的精确 Go 类型、范围和拥有值，长度切片/尾随字节、
有符号边界、非0/1 Boolean、Reserved、未知选择器、compound、异步、完成后 MID 复用、
TCP 乱序补齐/重传、缺段、连接代际、IPv4/IPv6、interface/VLAN、eager/deferred、
1/3 workers，以及 local/packet/stream 展示。首轮400 pass/1 fail仍保留：新测试错误要求
严格缺段回放成功；现在分别断言 strict 必须报原有 gap 错误，AllowIncomplete 保留可用来源，
两者均不得伪造 typed 字段。没有删除原断言来掩盖产品错误。

外部独立来源是官方 Wireshark 三个捕获 `smb2-getinfo.cap`、`getinfo.cap`、`smb2-peter.pcap`，
来源页面为 [SMB2/GetInfo](https://wiki.wireshark.org/SMB2/GetInfo) 和 [SMB2](https://wiki.wireshark.org/SMB2/)；
完整捕获、PDML 和 PDU 均留在仓库外。先冻结的6对同域请求/响应中，class5/7各3对，共21个
literal 字段、18个 PDML 原始位置匹配。来源事实 SHA256：
`4955d97b58020579fd8beda8dfa0995cab253e1510d011128c75bb87404d3245`；
6对/21字段及54个 checker 负控的来源计划 SHA256：
`51d083100c446818d9f47984730c24c338ca265b154ba960a6f077854600c2dd`。
这些 source-only 事实和负控不替代后续独立产品回放、Windows 页和最终 SHA CI。

原 Weber 的请求 VLAN81/响应 VLAN80 继续隔离；响应上的 Wireshark inherited class 不是
响应 literal。七类高级输出中的其余五类、canonical compound tail、SMB 高级晚到恢复、
完整语义字段/流/会话账本以及 TLS/QUIC 恢复仍开放。M2-29 的 raw110/PDML70 计数差异
单独追踪，不能事先当作40条 Yak 漏解析。
