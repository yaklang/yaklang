# RSVP Intserv：字段、父边界与实际页面

生产源码 `c04a60630ce2a3d0207aeca507e99dc535f8a4dd` 在共享 canonical/native RSVP codec
接入 Intserv 分层字段：RSVP 对象 → Intserv 头 → service → parameter → 具体值。
协议目录保持 partial；这轮没有把完整 RSVP 过程、已安装的预留或 TOP30 完备作为结论。

## 实现范围

| 对象 / C-Type | service | 选定参数 |
|---|---|---|
| SENDER_TSPEC / 2 | 1，Global Sender Traffic Specification | 127，桶速率、桶深、峰值速率、最小计费单元与最大包长 |
| FLOWSPEC / 2 | 5，Controlled-Load Receiver Traffic Specification | 127，同一线格式下的接收流量规格 |
| ADSPEC / 2 | 1 或 5，General / Controlled-Load Characterization | 4、6、8、10：跳数、可用带宽、最小路径时延、MTU |

对象、Intserv message、service、parameter 四层分别守住自己的范围。后三者的长度单位是
32 位数据字，不包括各自的四字节头。service 与 parameter 和原有对象、SCOPE 地址共用
collection budget；零长度未知参数也不能绕过预算。未知 service/parameter 保留自己的
不透明字节，并可在已证明的边界后继续解析合法兄弟节点。service2 Guaranteed 布局尚未选定。

每个 IEEE32 值同时保存原始位、分类、精确十进制文本与单位；NaN/±Infinity 不写成 JSON
数值。私有字段由34项增加到65项；十进制字段的类型明确是字符串，原始位另列 unsigned32，
没有冒充 Wireshark 的数值 float alias。零带宽标为未知，0xffffffff 路径时延标为不确定；
MTU 保留完整 uint32。字段和重复出现各自拥有精确的 PDU 字节范围，派生位不伪造字节。

语法与参数值约束分开。r/b 的零值虽能编码，仍产生正值约束问题；负零、非有限值、非法
m/M 也记录到所属参数的 Observed Value Domain Issues。原始字段不会因此消失。官方真实
输入的9个桶参数 m=0，其中2个接收 FlowSpec 的 M=0；Wireshark能显示这些数值不证明它们合法。
I/B 位是观察到的标志，没有被用来证明当前参数有效、路径能力或预留成功。

## 发现并修复的额外问题

- 完整 future Intserv version 的 Context 曾被两层通用逻辑额外升级为 Format，现保留未知布局原因；独立长度或校验和证据仍单列。未来版本的字数字段不授权套用 v0 体布局。
- capture decoder 曾先返回 Budget/Context，从而掩盖同一错误中的 Format/Integrity。现在已确定的线格式与校验和失败优先保留，并测试抓包层与原生状态一致。
- 未满四字节的浮点残余曾仅落到 RSVP 根余体。现在残余保留在已证明的 parameter/service/message 节点，未完整捕获的字段不补零、不借兄弟字节。
- Windows 独立详情重解原先只数对象和 SCOPE 地址；新增 service/parameter 一并计数，保持字节来源与原始 Session 一致。

## 来源与实际执行

独立 neutral source48 在候选输出前冻结，保留 source46 的16个真实 Intserv 对象和全部
原始字段；新矩阵覆盖29个 authored PDU。direct49 的16个真实目标重用9帧/3种 PDU 字节，
另外33个评价来自29个 authored PDU；预算0..4重用同一个 PDU，不充作5个物理包。

direct49 实际全通过：583个来源原始字段、326个派生项，其中63个 IEEE32 观察和17个拥有
范围的不透明/残余字节项。浮点复合观察的 bits 与 decimal 两种 API 表示不相加充独立线字段。
schema 桥另有8个拒绝负控，neutral 模型有7个负控。脏树 direct build 的11928个源文件
与后续生产提交保持，12个 Git 变更 blob 均与实际执行来源相同，另有明确的提交桥。

原生/CLI 默认4096预算单独冻结并实际执行38物理帧：官方9 + authored29，45个 Intserv
目标子树。23 decoded、9真实 snaplen incomplete、5 malformed、1 context-required；
65个 descriptor、1696个私有出现、38个 SDK/CLI snapshot 与物理递进桥均通过。
Session1250个 API range occurrence，GetFields1022个，分别核验，不相加为独立来源覆盖。

37组原公共/真实恢复输入又实际重跑，完整307819个原生对象、两份完整 flow 文件、384518条
CLI记录与55b逐字节相同，未作生成ID或字段归一化。CLI包括383936条物理记录与582条非物理
发布。输入、二进制、执行收据与完整文件哈希均绑定；这证明本轮保留范围，不认证每条历史解释。

最新 c04a 五包全量：285664通过、24跳过、0失败；此前首版全量285663通过后
又进行了33项混合错误/归属定向检查。最新 fuzz60秒/2 worker、100837执行、0失败，1GiB
限制下与1GiB CLI回放共存。没有将累计执行数作为性能测量或协议完备证明。

## 与 Wireshark 4.6.9 的逐包区别

实际参考使用隔离空 profile，工具和两个 PCAP、PDML/fields 输出均前后绑定哈希。映射先冻结，
之后执行38帧/45子树的参考对照；参考检查器没有读取候选 Go/SDK/CLI 输出。

| 来源项分类 | 数量 | 结论 |
|---|---:|---|
| literal 数值及原语范围精确 | 831 | 计为精确参考匹配 |
| derived 数值与来源位范围精确 | 143 | 计为派生参考匹配 |
| 仅 aggregate owner 匹配 | 53 | 不计原语范围通过 |
| 预选 alias 未渲染 | 8 | 不补成参考通过 |
| 数值相同，高亮跨度不同 | 4 | 明确保留范围差别 |
| 本轮未选择参考映射 | 469 | 不证明 Wireshark 没有这些字段 |

共1508来源项；已比较数值没有冲突。default38中的62个 IEEE32 与 direct49 的63个分开计数。
112处单位未被参考明确渲染，不计单位一致。未选映射中含3个已有 base ERROR_SPEC alias，
没有在看完输出后补映射扩大通过数。

11类差别有独立证据，主要包括：

- authored frame10 的 TSPEC 对象是 PDU[8,20)，TIME 兄弟是[20,28)。参考仍在原 TSPEC 祖先下把后8字节解释成 parameter；本实现拒绝内层越界并保留余体。
- authored frame12 的 ADSPEC 数值3来自合法[24,28)，参考的聚合高亮却为[20,32)，超过捕获/父范围28。这里仅证明高亮范围错误，不能把它说成数值确实读越界。
- 未知 service 后6个可证合法兄弟字段未被参考渲染；future version 下参考仍显示5个 v0 TSpec 值。本实现保持未知内容与可证兄弟的边界。
- 两个完整 partial header Flags 未渲染；两个 parameter owner 高亮裁到7/11个捕获字节。NaN文本不能区分 payload，原始位仍可核对。
- 17个来源参数域问题没有相应 literal-owned expert；独立的 Unknown session type/过程警告仍可能存在。额外 Guaranteed-service 支持属于参考的更多能力，本轮不充作实现错误。

主证据来自 [RFC2210](https://www.rfc-editor.org/rfc/rfc2210.html) 与
[RFC2215](https://www.rfc-editor.org/rfc/rfc2215.html)。Guaranteed 的专属约束参考
[RFC2212](https://www.rfc-editor.org/rfc/rfc2212.html)，未套到本轮受控负载/通用参数上。

## Windows 实际页面与 CI

Windows 使用新构建与默认20000包/64MiB缓存。根 agent 查看了两张实际原生控制台
单元/颜色还原图：Token Bucket Rate Value6000 和 Composed Path MTU1500，分别选中
PDU[68,72)、[128,132)，对应物理[106,110)、[166,170)。完整 Source0=174B 与
Source1=136B、四个字节和 ASCII 高亮、鼠标/tabs/resize/ack、CP936恢复均核验。
本轮原导航计划即通过，没有更改字段名称、值或范围；图像不是 Windows Terminal 像素截图。

c04a的四项CI全部首次通过：traffic40940218890、Essential40940153618、Diff40940153615、
路径40940153625。草稿自动跳过的traffic不计通过。前一纯归档e814的MITM插件测试曾因
未发现预期漏洞失败，原日志保留，失败项重跑通过；没有改该插件来替代协议验证。
后续文档/源码归档提交的CI单独跟踪，不借c04a冒称新的HEAD已通过。

[聚合证据](rsvp-intserv-c04a-summary.json)保存范围、版本和外部proof哈希。
[冻结归档](../../../common/yak/cmd/yakcmds/shark-cli/testdata/live-audit.zip)
内的 `live-audit/rsvp-intserv48/` 保留21个模型、适配器与检查器的原始源码，以及现有依赖。PCAP、密钥、完整来源/期望、
输出、日志、截图和二进制不进入仓库。

旧原生 runner 的构建预检/检查器参数顺序失败、参考 mapping01 准备错误和 comparator01
的两条范围分类勘误均保留。后续版本修正工具合同，没有按候选结果重写来源值或既有映射。
尚未实现的范围包括完整 mandatory-service/过程语法、Guaranteed/RSpec、认证/重放、
已安装的预留，以及完整 TOP30/核心协议覆盖。
