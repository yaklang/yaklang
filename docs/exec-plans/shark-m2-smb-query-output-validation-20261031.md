# M2 SMB 输出、恢复与页面：0769 构建验收

本批 PR 为 [5376](https://github.com/yaklang/yaklang/pull/5376)，基线为已全绿的
`f9faae9f82c0dbe6a657bfaa94d7d6acdc845380`，不扩张 M1 范围。
实际构建、SDK/CLI/flow、Windows 与覆盖引导 fuzz 的代码提交均为
`0769c2617d8f8fad4e66ecf2a323461669261a10`。本说明是随后文档提交；
最终 PR 提交的完整 CI 另以 GitHub 精确 SHA 检查和仓库外闭合记录为准，不能用旧 SHA 的成功代替。

## 本批有限能力

共享 canonical/native 的 QUERY_INFO 输出只新增 InfoType1/class7 的4-byte
FileEaInformation 和 class5 的24-byte FileStandardInformation。
保留原始 Boolean 字节、signed64 大小、Reserved、拥有字节和绝对 PDU 范围；
非零 Boolean 解释为 true。只有反向同域同连接代际的已观察匹配请求能决定响应类型，
没有上下文仍 opaque；请求选择器不伪造为响应 wire field。七类目标中其余五类仍开放。
布局依据及初次实现见[初始记录](shark-m2-smb-query-output-20261031.md)。

M2-30 修复完整记录的正文失败使后续流永久停止的问题。两个官方旧捕获在首个
SESSION_SETUP 使用 StructureSize17/80-byte历史布局；它不是现代25/88-byte布局。
保留完整 raw、Header、Unparsed Body 和错误，标明 `headers` / `unsupported body layout`。
仅完整 Direct TCP 边界、全 compound 固定头和关联身份安全检查通过后，才在下一已知记录边界继续；
没有扫描 payload 猜边界，没有提交失败记录中较早命令的 pending/dialect/file/AsyncId 状态。
framing/header、资源预算、身份冲突仍中止；不降低现代 SESSION_SETUP 的长度约束。
现代成功/continuation/ERROR 的 bounded body 与 canonical 保持一致。
不照抄参考在旧布局上与安全 token 重叠的现代 PreviousSessionId 字段；旧方言确切草案版本仍未证明。

M2-31 修复 STATUS_PENDING 借用另一请求 AsyncId、同请求换 AsyncId 遗留别名和
compound 预检漏冲突。pending 使用 literal MID；final 才能使用已观察的非零唯一 AsyncId。
同 ID 重复幂等，换 ID/碰撞拒绝，完成后清理唯一别名；MID 复用后的迟到旧响应不能借新 selector。
未声称认证、签名或所有乱序状态机已验证。

M2-32 修复列表只有泛化命令、拒绝记录被写成 stream bytes 的实际展示问题。
列表直接显示类型、EOF、allocation、links、directory 或 EA size；整数不经 float64。
树按 Header → Body → 命名类型 → literal 展开；上下文和 raw 同时保留。
完整拒绝记录显示 unsupported body layout；混合拒绝/成功 PDU 的两种回调顺序与连接重绘均保留 partial 状态。

## 原始捕获逐项比对

使用 Wireshark 官方 [SMB2/GetInfo](https://wiki.wireshark.org/SMB2/GetInfo) 与
[SMB2](https://wiki.wireshark.org/SMB2/) 页面附件，完整原捕获回放，未裁剪成只含目标的输入。
所有原捕获、完整 PDML/JSONL、PDU、密钥及 keylog 均在仓库外。
TShark 4.6.9 使用空配置；TCP 乱序重组配置与不同统计单位分别记录。

| 捕获 | 字节数 / SHA256 | 来源QI命令 | 旧f9原生 | 新0769原生 |
|---|---|---:|---:|---:|
| smb2-getinfo.cap | 366518 / `a2fbbbe0512da4fb4a741935d1d0b742855361152d629009d30404d66d673119` | 2048 | 0 | 2048 |
| getinfo.cap | 65790 / `867e02b4442bb3fcb4a4c298b1a3252080eeaa3a28dd909cbb5b299c7506963f` | 192 | 0 | 192 |
| smb2-peter.pcap | 525520 / `eabad564ae039cbdc24af89eb9ec9333f0bf08a9963e41ade6caef2495f7bde3` | 110 | 110 | 110 |

2350 个命令（1175请求+1175响应）的原始包号、raw、捕获域、方向、连接代际、
31725 个固定头 literal 和17453个正文 envelope literal 一一匹配，0遗漏/重复/已映射差异。
49178 项包含 opaque buffer 边界与原字节，不等于49178个高级内容语义都已解析。
`M2-all-official-QI-audit68-01/summary.json`：
`2a30cfc75df1365731965f627cdfdb01bd8d84f3ab8346e4ee6538e260d20662`。
另6对先冻结的同域请求/响应、21个类型化 literal 全通过，54个 checker 负控通过；
`M2-QI-types-audit68-01` 的计划 SHA256 为
`017bae0375ec1ca3d18e3f3daf73fdb08727494317ffc210b06675e8d39cde18`。

peter 默认参考70条中的40个完整响应位于 tcp.segment_data；独立 raw/stream 均是110条。
分别只关闭 TCP desegmentation、sequence analysis、out-of-order reassembly，均恢复110。
这是参考观测配置差异的有限归因，不是40条 Yak 漏解析，也未证明 Wireshark 内部 C 状态原因。
来源账本、110条原生一一对应及负控见 M2-29；原有70条输出仍保留。

大捕获 The-Ultimate-PCAP.pcapng SHA256
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`，
0769 的三套完整回放 exit0。5个 legacy offer、107个选定记录/121命令头、全部281来源记录通过；
独立 QUERY_INFO 检查为93 PDU/113唯一命令/731固定正文字段，8382断言通过。
原严格 checker04 仍报告1400缺口、0已映射值差异；独立 QI 映射闭合392，余1008
（961未发表/47范围未映射）。Weber 的 VLAN81 请求/VLAN80 响应继续隔离，不能借 inherited class 解码。

与 f9 完整导出比较：42223原生事件中42221整行相同，2条仅是失败 SESSION_SETUP 的
Body/Error Response 投影变化；全部 ID/raw/包来源/流身份保持。
51328物理包中51323整行相同，5条仅 Info 文本变化；29个延迟 publication 相同，flow 整文件相同。
输出连续性本身不作语义验收。闭合记录 `M2-runtime-closure68-01/summary.json` SHA256：
`c36669abd2af6b2f44066632e55c8a55e9018dc725c52fe62297fafbea0bcb09`。

## 测试、fuzz、Windows 与输入保全

Go1.22.12，构建/测试 `-p1`、GOMAXPROCS3、GOMEMLIMIT2GiB；资源与源码在执行前后守卫。
选定4包矩阵428 pass/0 skip/0 fail；完整 stream_parser+CLI 为8785 pass/3历史 skip/0 fail，
两者有重叠，不能相加。日志 SHA256 分别为
`0d8ead7653ebc61d6f21482cfa269e91039677da21ff518d61aa5566f4d66c0c`、
`7e12666d66e7a8935b16049014a7f165eaf027fb01316cd2b75d1709ed3aead2`。
历史 skip 是 PreparedOperatorRemainingInventory、可选外部预览审计、控制台 child 入口；
完整外捕获与真实 Windows 运行另有闭合证据。最早错误的 gap 测试和 UI 失败保留，未删正确性断言。

覆盖引导 `FuzzSMB2StatefulBodySources`，6类真实 pending 入口，90秒、1CPU/768MiB，
62949次、43个新覆盖输入，0 fail；含独立 little-endian scalar/owned bytes 检查。
日志 SHA256 `40bf4e8dcf35e237548be049a3f08f20f788fe25b59f13cd0cdb934b0b97ac22`。
编译启用 Go fuzz coverage；runtime cwd/corpus/cache 在仓库外，源码和样本输入未变。
有限正文 fuzz 不代表所有协议状态序列已验证。

Windows 实际运行0769构建，四页/80个控制台状态：EOF7、Directory1、EA41、旧 SESSION_SETUP。
完整物理帧166/166/146/224字节、PDU100/100/80/158字节全部逐字节核实；
选中字段高亮分别为 PDU[84,92)、[97,98)、[76,80)、未解析正文[68,158)。
列表名称/值、逐层展开、mouse/Tab/resize、CP936恢复通过，主 agent 已逐页看图。
证据来自真实控制台 cells/attributes，不冒充 Windows Terminal 像素截图。
`M2-Windows-QI-types-root-closure68-01/summary.json` SHA256：
`f36806a919530f8d8718db231bf5a736e6a0b61dd23e8c1eae7ff249a7d6b318`。

受版本控制的 central captures.zip 保持18916097字节，SHA256
`0371e1304d6711497f697ea8a7622c707140e7e5378215d67bde7f0985bd8213`；manifest/index未变。
没有把官方原捕获、敏感内容或导出加入 ZIP；没有历史重写/强推。

## 明确未关闭

新增 M2-33：smb2-getinfo.cap frame17 是完整153-byte SMB1 NEGOTIATE 扩展安全响应，
WC17/ByteCount80，GUID16+security blob64；现有入口仅支持 request，误报 malformed。
诊断只保留64-byte preview，不能拿它与完整PDU哈希混为一谈。
独立事实/规范/原始字段已冻结，原生/canonical/连接关联/UI修复尚未执行。
此项在下一分支实施，不能把本批2350个QI通过推广为整个SMB完成。

另有5类QI高级输出、其余SMB body/compound语义、缺前驱晚到、完整流/会话账本、
TLS/QUIC高级恢复及其他协议/字段/fuzz差异，继续按 M2 清单逐项验收。
主表33项中完成25、进行中4、未开始4，剩余8；剩余工作粗估28–56 AI实施小时，
不含CI等待、数据获取和新发现，范围有重叠时不重复累计。
