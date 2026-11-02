# IPv4 IP-in-IP父层范围、来源与实际页面：2026-11-02

M2-64关闭完整可观察IPv4头的零长度IP-in-IP父层拒绝合同。运行代码、干净SDK/flow/Windows CLI
构建均为`6ef07ebcbf6f3b3b9b225defc667e8d38c5494a4`。仍只提交M2 PR #5371；SMB继续搁置。
N14完整IP字段、传输代次、IPv6及其他封装边界仍开放；本批不是全部Wireshark语义验收。

## 问题与修复

M2-63拒绝零长度碎片以后，独立来源探测929又复现：一个字面Total Length为零的IPv4父包，
经IP-in-IP归一化后可以建立内层DNS。直接非法UDP已有承载拒绝，归一化却使其只检查内层有效IP。
相同入口还可建立TCP连接，或者把无效父层里的片段及未来时间戳交给碎片存储。
真实红测932为13 fail；修复后的原生检查933为14 pass，包含严格API子解码错误保留。

原生分发现在先检查已选择拓扑中的IPv4协议4/41父层字面长度，再归一化或接纳内层状态。
拒绝事件为`ipv4-carrier-extent-refusal` / `IPv4CarrierExtentUnavailable`，只保留观察到的父头、
物理域及明确的normalization/transport/reassembly拒绝，不赋予子PDU字节、完成帧或连接。
原包已有子解码错误时，专用包装保留严格捕获API错误与计数，避免重复泛化诊断或 transport redispatch。
非分片TCP的既有捕获前缀政策、直接UDP拒绝、未选择封装的优先门继续保持。

概要、详情、quick、packet worker及stream worker采用同一完整合同。只有真实当前物理包、域、
头链、字面长度、ID、flags和next protocol同时匹配，才给观察字段原始范围。
没有帧的stream诊断保留IPv4身份及原因，不借取物理字节。父层无效时也不把地址形字节当作有效子端点。

## 来源与逐项参考

937从Sharon原14367/14368两帧的完整1702B DNS响应构造九个明确声明的派生场景，
应用字节保持；另从原公开捕获取12个零长度IP-in-IP父包，完整原帧字节不变。
它们不是21次新peer抓包。所有捕获、账本及完整导出均在仓库外。

945执行147次真实运行：21旧/新SDK、21旧/新flow、21新CLI及42新TShark。
TShark固定4.6.9、空个人配置、双遍、IP重组和TCP乱序重组打开，分别显式设置TSO补值开/关。

| 控制 | 旧native DNS | 新native DNS | WS默认TSO | WS关闭TSO |
|---|---:|---:|---:|---:|
| 直接有效 / 有效IPIP（每例） | 1 | 1 | 1 | 1 |
| 直接零长度 / IPIP内层零长度（每例） | 0 | 0 | 1 | 0 |
| IPIP父层零长度 | 1 | 0 | 1 | 0 |
| 同UDP tuple有效→无效父层→有效 | 3 | 2 | 3 | 2 |
| 完整首片→零父层尾片带未来时间→完整尾片 | 0 | 1 | 1 | 1 |
| 同上接口隔离 / VLAN隔离（每例） | 1 | 1 | 1 | 1 |

950完成953项来源/字段断言及967项字面参考检查。七个接纳DNS逐字节验证拥有Raw、所有头标志、
问题名称原字节/type/class、四个answer和一个OPT的全部owner/type/class/TTL及完整opaque RData，
四个typed alias、域、贡献顺序和CompletedIn。参考同时验证所有标准RR及OPT元数据。
重组恢复的贡献必须为1/3，零父层帧2不能供字节或新状态。DNSSEC签名、信任和全部RData语义不在此验收内。

Wireshark默认允许对零IP长度使用TSO捕获长度推断；关闭该推断时，有限控制与native DNS数量一致。
不把默认参考差异删除，也不声称所有版本/配置或真实接收端行为一致。
长度含IP头的依据与TSO实现见[RFC791 §3.1](https://www.rfc-editor.org/rfc/rfc791.html#section-3.1)
和[Wireshark v4.6.9 IP dissector](https://github.com/wireshark/wireshark/blob/v4.6.9/epan/dissectors/packet-ip.c)。

## 完整大捕获差分

947用最终源码重新构建的三种入口完整回放Weber、Sharon原捕获，共六个实际导出。
Weber三份导出及两份完整TCP flow均与6379检查点逐字节一致，379455物理包保持。

Sharon原生事件264101→264103：264094条不涉及这12个候选包的事件，完整拥有字节、顺序、
域、流、状态及所有导出字段保持；原七个父包诊断变为九个具体父层拒绝，每个均独立回查原始物理头。
只映射明确的全局PDU引用：根ID/response_to、ByteSource父PDU及typed pdu.id；不改任何wire ID、帧号或任意数值。

959进一步完整核对328115条候选以外物理CLI行及全部442条独立publication；物理行中附带的
异步published_pdus也全部核对。CLI的fields是typed alias multimap，分别与SDK typed values匹配，
不是与SDK raw dissector tree混为同一结构。九个改变行逐项匹配SDK拒绝、来源与typed字段；
剩余三个候选行完整不变。未丢消息、未重复发布，也未把生成的PDU标号误算为协议字段差异。

## 明确新增的剩余缺口：N14.U01

候选原帧163432、180643、184529的IHL均为24，但四字节option分别为`2e000400`、`2e140000`、
`15008048`，其option长度为零或超过当前头。两版均没有原生事件，CLI保留未解码IPv4、没有子协议/连接。
本批九个具体父层拒绝不包括它们，不能称12个全部已修复。后续须在字面父头和option错误之间建立
准确的观察合同、严格API计数、范围和具体诊断，并补相应反例；不得凭前两个字段猜内层。
此缺口明确归N14，不增加17个工作包分母，也不关闭N14的完整字段、会话和预算恢复门。

## Windows、回归与资源

953实际新版控制台27状态通过：读取派生PCAP，完整逐页读回1764B捕获源，原始`00 00`仅在
Source0的[16,18)高亮；breadcrumb只有Ethernet→IPv4，父头观察先于生成诊断，无DNS/TCP子身份。
窄屏80×30、恢复180×48、mouse/Tab、CP936恢复及自有进程关闭均检查；955由主线程看实际PNG及单元属性。
选中字段会将offset16滚到顶部，这是实际可见窗口；完整读回另遍历全部页，不要求起点始终为零。
这是实际控制台单元检查，不代表所有Windows Terminal字体、DPI或IME配置。

完整四包本地回归939：280399 pass、23历史可选skip、零fail；新37个精确ID全部pass。
其中17 native、20 CLI，覆盖双层IPIP/6in4、同连接恢复、workers1/2/4、关闭协议事件仍无TCP连接、
共享内层fragment ID、未来时钟、接口/VLAN和当前物理范围反例。948/949最终插桩fuzz完成两种子、
34134次实际执行，硬1 CPU/768MiB job/最多2个同时进程，越界第三进程被拒绝，自有后代随job关闭。

938首次调用少传source参数，尚未构建即失败，原目录保留；952错误要求滚动窗口从零开始且只翻两页；
954混淆两种导出fields结构、956未处理物理行附带的publication。原失败全部保留，后续修正校验器后
仍逐字节、逐字段检查，没有删除正确性断言。960为本地有限综合验收；不是旧失败悄悄改成成功。

944仅对11份已关闭的完整同内容导出做SHA及逐字节核对，用硬链接保留全部路径/字节，
释放10份非共享数据5676315340B；保留维护清单。源码、外部捕获、keylog和完整证据未删除，
两份受版本控制fixture ZIP的字节及SHA不变。最终本地闭合空闲10892673024B，继续保留8GiB运行下限。

## 证据及CI边界

本地证据根为`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`，不提交完整导出/捕获/密钥/图像：

| 闭合 | SHA256 |
|---|---|
| IPIP-control-check950-01/summary950.json | `8ea315d5c1354ee48ad4d080f33ccc24f587d2bad75cae510d32867596b0a6dc` |
| IPIP-public-check959-01/summary959.json | `dc9f16e29e0ccde792588aeddf7dc421a7bbe4edec3e4c0e463f66fd07fa0f8b` |
| IPIP-qualification960-01/summary960.json | `b7daf7b657a30136f1e74e5f71b7a0ddc004aed2cd7f3075e271b32a2751fcc4` |
| Windows-IPIP-native953-01/console/01-selected-field.png | `8ea7318834f084e87720c33dfe289dc052d8b2e996b1c75c8d63a0744c70dbd8` |

先前`1f1c0c5b338526695defd33f18234f983e5254c1`已独立47约定检查/91实际执行/2statuses全绿，
[流量专项](https://github.com/yaklang/yaklang/actions/runs/41375224898)完整314229原始ID/终态一致：
314204 pass、25历史optional skip，50新增及131084 required全pass/0skip；official artifact SHA为
`e51391e1395db648ab0f43c2803554bc99453efb5ea322a84c8dd6e3a15cb703`。
[Essential](https://github.com/yaklang/yaklang/actions/runs/41375228059)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41375232423)及自动矩阵均成功，draft自动traffic skip未计通过。
930综合闭合为`ea4ed8e8e54275ad32257ca38a81d1d9f6c2860769836d48777d157206f352b6`。

该成功不认证本批6ef及随后说明提交。新最终head须事前冻结314266原始ID/终态，保留父314229全部动作，
37新增全部要求pass；预期314241 pass/25历史optional skip及131084 required全pass/0skip，再独立闭合最终SHA全部CI。
当前64父项中54有限完成、7进行中、3 SMB搁置，10未关闭；非SMB17包中12有限完成/5开放，
已知剩余实施粗估20–40 AI小时，CI/构建/回放等待及新问题另计。不是核心协议完备承诺。
