# M2-33：SMB1 WC17 扩展安全协商响应

代码提交 `7387371611d5073be5017960f9361b8b2b8cf000`，分支
`codex/shark-m2-smb1-extended-response`，基线为已全绿并ready的
[PR5376](https://github.com/yaklang/yaklang/pull/5376) / `a4077dd1162bac1fb672a0cd91dfb487117cdaa3`。
M1和父PR的范围保持阶段性；本批只验收此响应profile及其必要的原生连接/展示合同。

## 解析与身份边界

依据[MS-SMB 2.2.4.5.2.1](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb/d883d0a5-5a0a-4626-8e3e-87b0b66b79aa)，新增两个显式canonical入口：
`CIFSNegotiateExtendedResponse`、`CIFSDirectTCPNegotiateExtendedResponse`。
要求成功REPLY、WC17、extended-security标志/能力、ChallengeLength0和完整ByteCount。
ByteCount至少16、至多65535，Direct TCP完整记录为89..65608字节。
GUID16和其后的security blob均保留owned bytes，blob允许为空；不凭blob推断认证通过。

Header和Body保留精确literal类型/范围：FILETIME为uint64，Server Time Zone为int16，
不是unsigned64876。原request入口、legacy SMB binding和custom parser注册合同保留。
目录、嵌入规则archive、原生分发、unsigned字段alias及worker展示已同步；当前alias schema
没有signed/bytes类型，时区/GUID/blob保留正确Body值，不伪装成unsigned/string过滤字段。

响应索引只从反向、同捕获section/interface/VLAN、同TCP代次、完整PID32+MID匹配请求解析名字。
TID/UID仍为literal，不据此建立树/用户会话；SMB1 MID不进入SMB2 pending命名空间。
采用保守的一代一次协商：第二个完整应用请求即使原字节相同也使选名ambiguous；
先观察响应、完成后复用key和迟到旧响应均不能借未来请求。新SYN代次重置。
错误方向/PID/MID不消费合法pending。只有匹配到原请求的`NT LM 0.12`且请求声明extended security才选名。

完整已验证SMB2封装会退休旧SMB1选名资格，包括body拒绝但完整header链/身份安全门通过的情况。
这仅退休旧资格，保留body错误、`headers`与`State Updated=false`的命令事务语义，
不提交被拒绝记录里的SMB2 pending/async/files/dialect，不推断认证或数值SMB2 dialect。
partial/framing/resource/身份冲突没有这份恢复证明。新增最小序列曾实际复现错误借名，修复后通过。

预算在Node分配前检查；frame/message/aggregate-buffer/depth拒绝保持私有typed ResourceExceeded，
状态为limited，公开Status/Protocol/Fields/Structured修改不能将其变成成功。私有offer计费含外/内slice capacity。
未接入的完整响应形式明确unsupported；可证明WordCount/ByteCount越界的完整记录仍malformed。
诊断preview与完整PDU区分，缺完整消息时不伪造CompletedIn。

## 原始官方捕获与逐项比对

完整原始`smb2-getinfo.cap`为366518B/2096物理帧，SHA256
`a2fbbbe0512da4fb4a741935d1d0b742855361152d629009d30404d66d673119`，来源为
[Wireshark SMB2/GetInfo](https://wiki.wireshark.org/SMB2/GetInfo)。未裁剪后再回放。
原frame16请求223B/11方言；frame17物理219B，完整PDU153B，SHA256
`e7b5ad59b612c347280cd65e2e897681bfa482a10202aa6349bb9b65b4f5ebd7`。
旧64B preview SHA49384fb2…不是该完整PDU。正确选择索引10来自原请求，不来自端口或响应长度。

候选盲checker03由原始捕获/独立TShark4.6.9及主规范冻结；实际SDK检查匹配两个原始记录，
78个源出现项在Fields+Session两副本合计156个literal检查，1142断言，0差异/遗漏/重复。
78包含重复/边界出现项，不等于78个标准语义字段；67个来源/checker合成负控另计，不能充产品测试。
`M2-SMB1-real-source-audit69-01/summary03.json` SHA256
`0ac251c662c7b15773e417d76367e801e4c714f221b40dd5f91a4863d9770b42`。

原完整capture原生2060→2061事件：原frame17由malformed preview变为完整decoded response，
随后新暴露frame19 SESSION_SETUP_ANDX为unsupported诊断。新增事件不是重复PDU。
另外两份官方getinfo/peter完整SDK导出与父阶段逐字节一致。
三份原捕获全部2350个QUERY_INFO、31725固定头literal+17453正文envelope literal仍一一匹配，
0遗漏/重复/已映射差异；六对原同域typed样本的21个literal和54个checker负控继续通过。
49178包含opaque buffer及其边界，不能解释为全部高级语义字段已解析。
`M2-all-official-QI-audit69-01/summary.json` SHA256
`b22493d9f5a7e5540fcd550c10e8c241bdc7466d5fa87fb005e0ad9d65e8f5b6`。

Weber The-Ultimate-PCAP.pcapng SHA256
`ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4`，
7387的SDK/CLI/flow三套完整回放均exit0。5个offer、107目标记录/121头、全部281来源门通过；
93-PDU QI门8382断言通过。原严格checker04仍有1400覆盖缺口、0已映射值差异；独立映射闭合392，余1008。
同接口/VLAN隔离保持，未借参考的跨VLAN inherited selector补字段。
与父阶段0769构建相比：42223 SDK行中只有5个offer的Selection Policy/Status说明变化，
全部ID/raw/来源/flow身份相同；51328物理行、29个延迟publication及完整flow均逐字节相同。
输出连续性不作为全部语义正确性的证明。
`M2-runtime-closure69-01/summary.json` SHA256
`15aecb88f0e164cb6031f083486d12ed7caa9638dcde5ebc87ac3533234b68b8`。

## 本地、fuzz与Windows

Go1.22.12/windows-amd64；构建/测试-p1、3CPU/2GiB；所有运行前后检查源码/索引/输入。
7387提交后选定5包矩阵510 pass/0skip/0fail，日志SHA256
`5826a8d58262ac5a419e64edfb88b2b6c772fcbd505b17214620ac0c00d5ba88`。
完整parser/...+rules+CLI为9636pass/3历史skip/0fail（最终carrier退休修复前相同parser/UI源码；
其后510矩阵与实际回放覆盖该修复），日志
`bc030d5fa687511094466f9ddc8d0b5f809c8f104ed642a26da76cc1925e0f52`。
两组有重叠，不相加。历史skip为PreparedOperatorRemainingInventory、可选外部预览审计、控制台child入口。

覆盖canonical/native精确类型与拥有性、零/最大blob、签名之后的全部prefix/trailing、非extended/坏共同envelope、
PID高低/MID/方向、索引/请求标志、重复/早到/迟到/新代次、现代carrier/body拒绝、
真实TCP乱序补齐/重传、永久缺段strict/AllowIncomplete、IPv4/IPv6、interface/VLAN、eager/deferred、1/3workers。
原失败69-01、69-03和错误借名69-09保留；没有删除正确性断言。
第一次UI断言错误地禁止context副本来源；修正为严格核副本bytes/标题/独立source，仍禁止响应wire range。

首轮带插桩90秒SMB1正文fuzz109765次/9新覆盖输入/0fail，日志
`2019e6f27854db8dfceb7f4807ab45a5d51bc2ce04e7b731b99db76ed7c59d25`，在最终carrier退休修复之前。
7387最终提交后重新编译独立插桩fuzz，90秒103984次/9新覆盖输入/0fail，源码/索引/archive未变；日志SHA256
`e854a242fb6cc7e8fe24acf21447f0e6c56e2bb1499aed9929ade023d97cb1c1`。
所有fuzz使用仓库外cwd/corpus/cache，1CPU/768MiB；原输入不被target修改，独立LE scalar/owned bytes oracle。

Windows实际运行7387构建，两页53个真实控制台状态。列表同一行显示SMB1、NT LM 0.12、opaque blob64；
树为Direct TCP→Header→Body→Observed context，GUID[73,89)和int16时区-660[68,70)的高亮准确。
完整物理帧219/PDU153/请求上下文10B三份来源逐字节核实；请求来源明确标response offsets unavailable。
mouse/Tab/180x48→80x30→恢复/CP936通过，root已看实际图；证据为console cells/attributes，不是Terminal窗口像素。
`M2-Windows-SMB1-root-closure69-01/summary69.json` SHA256
`6ba53d74ae0690c05fa31af0d7388f533b0f0bba5f51390e0236c5e6306b2dc3`。

central captures.zip仍18916097B、SHA256
`0371e1304d6711497f697ea8a7622c707140e7e5378215d67bde7f0985bd8213`；manifest/index未变。
本批没有新增PCAP散文件、外部原捕获、raw PDUs、keylog/密钥或完整导出入库；测试只构造内存样本。

## 新缺口及交付边界

M2-34新增SESSION_SETUP_ANDX：完整原输入独立源22个SMB1记录，frame19–22是两对WC12/WC4扩展安全setup，
PDU228/416/340/246B；AndXCommand=0xff的终止标记不是第二条命令，终止时非零offset必须忽略。
四条源记录110个leaf出现项含raw/pad/未知尾部；两个请求各20B规范未证实尾部不照抄参考PrimaryDomain解释。
旧generic AndX的ByteCount位置不适用于这些记录，不能把YAML存在算原生已支持。
来源及16个边界计划已冻结，产品尚未实现；修完仍有16条其他命令记录，下一frame23 TREE_CONNECT_ANDX另计。
源收据SHA256 `9d71cae681224ad66344b243239314f77bbb60e3b83ec99317f3ee72fe05a931`。

其余SMB body/compound/高级晚到、5类QI typed输出、完整流会话账本、TLS/QUIC高级恢复及其他协议差异继续开放。
实际窄窗口Stream起始页还暴露M2-35：非HTTP连接仍显示HTTP专用缺头提示；这是独立展示改进项，未用两页字段验收掩盖。
M2-33只有限profile交付；最终交付SHA的47checks+2statuses和完整traffic清单仍须实际CI闭合。
