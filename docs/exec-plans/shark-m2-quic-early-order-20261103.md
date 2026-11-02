# M2-79：QUIC 早期密钥、跨层包号与实际 Windows 展示

运行代码和三个干净构建为 `e71481b05a80c009ae6bf70199617f077e7c6bf0`。
本批仅验收已观察密码套件后的早期密钥加载，以及已接纳客户端 0-RTT/1-RTT 包号的顺序约束。
**真实原始捕获仍有一个先于密钥加载的 0-RTT 未恢复，以及一个关联的 HTTP/3 上下文缺口**；
另复现了迟到 peer SETTINGS 后 QPACK 指令没有重新处理。不能写成完整 0-RTT 或 QUIC 恢复已完成。

当前79父项、69有限完成、7进行中/3 SMB搁置，10项未关闭。非SMB17包中13有限完成、4开放，
N09/N10/N13/N14粗估17–34 AI实施小时；CI、构建、回放等待和新发现另计。
SMB继续保留任务书并跳过；没有修改继承测试、CI、fixture ZIP或SMB代码。

## 行为与正确性边界

原生密钥加载遗漏 `CLIENT_EARLY_TRAFFIC_SECRET`，即使ServerHello和密码套件已经观察到，
晚到的0-RTT仍被报为缺密钥。现在与四个普通secret一起按当前ClientRandom查询、按已观察套件派生。
第五个可选secret不会令完整普通密钥被误判为未加载，避免后续CRYPTO观察重新覆盖应用密钥epoch。
不依据端口、keylog行序、另一个连接或未选择的密码套件猜测早期密钥。

[RFC9001 §4.9.3](https://www.rfc-editor.org/rfc/rfc9001.html#section-4.9.3)、
[§5.6](https://www.rfc-editor.org/rfc/rfc9001.html#section-5.6)与
[RFC9002 §4.2](https://www.rfc-editor.org/rfc/rfc9002.html#section-4.2)约束客户端早期/应用层包号。
捕获到达顺序不同于发送包号顺序：小于已接纳客户端1-RTT最小PN的晚到0-RTT可以正常解析；
相等或倒置的跨层PN在重复包处理前拒绝。反方向服务端PN不建立客户端界限。
界限是当前私有连接生命周期内的观察，不由公开DTO提供，也不会随应用换钥重置。
认证失败、保留位/帧语法拒绝及资源拒绝不建立这条界限；被拒绝包保留完整已认证明文，
不提交帧、PN界限或密钥epoch。可恢复的H3上下文等待与真正的帧接纳失败分开。
这不是对全部QUIC帧处理作事务原子性的承诺。

## 新实抓与独立来源

Windows本机loopback上运行aioquic 1.7.0的两个真实客户端/服务端连接，使用同一对UDP socket/端口。
第一条完整握手，第二条由两端确认session resumed及early data accepted，实际发出一个0-RTT包。
两条HTTP/3 POST和响应均与端点收发字节哈希相同。证书过期的初次尝试保留；最终重新生成并信任
本机测试证书，证书校验保持启用。捕获进程限制修正及辅助头观察器的失败记录也保留，未计为有效来源。

最终 `QUIC-early-source1490-01/live.pcap`：19物理帧，含6个就绪marker、13个端点UDP数据报、
21个受保护QUIC包。PCAP SHA256：`2c7bd3871b18bc3fbe0485bc798b20d396b8e1e2ad119278aa90b6cb3a8f35d0`。
独立解码在读取native或参考导出前，从原始ODCID、源ClientHello random、ServerHello套件和授权keylog
核验全部21个AEAD，重组CRYPTO/STREAM并解QPACK，核对两请求/两响应的头和正文。
第二条PSK identity匹配第一条实际NewSessionTicket，CH/EE early_data标志一致。
三个全零UDP后缀单列为未认证datagram padding，不冒充QUIC帧。
这份源证据不宣称独立验证binder、Finished、签名、PKI或通用重放保护。
来源闭合1496 SHA256：`f2cfb00ae78e85d8dab8ff40b057a7d5fa9088cb9edb1cb05ad934fb5220273c`。

构建为Go 1.22.12 windows/amd64；参考为固定SHA的TShark 4.6.9，独立空配置、`-2 -n`、
按实际服务端UDP端口显式decode-as QUIC，并只使用本次授权keylog。Go测试/构建3CPU线程、2GiB，
小回放2CPU/512MiB；GUI单CPU/进程512MiB/总1GiB/最多8进程，完成后关闭整个Job。

基线bb217与最终e714的原始捕获SDK/CLI/flow三导出字节相同：native仍只认证20/21个源包，
先到的0-RTT缺密钥，随后响应缺QPACK peer容量。参考21/21个PN相同；329字面字段检查通过，
但两个native缺口明确保留，**字节相同不是恢复正确性通过**。

另外事前冻结10组真实源变换：3有效乱序、6非法跨层顺序和1丢包；变换不是新的真实端点会话。
独立重封包AEAD、反转tag负控和IP/UDP checksum核验后，再运行30个旧/33个新完整native导出及
11次新完整两遍TShark参考（含原始捕获）。最终13个目标中7个有效观察、6个MalformedMessage拒绝；
错误包后的有效0-RTT仍恢复，两个同tuple/CID代次独立。13个目标的276字面检查、拥有明文及参考PN均相同。
Wireshark展示了这些非法顺序包的PN，没有将其展示结果当作完整顺序接纳的oracle；原专家信息保留。
所有11个完整flow导出保持。迟到0-RTT在响应之后到达时仍有QPACK上下文等待，归R04，未豁免。

## 本地、字段与 Windows

新增85个测试ID，包含v1/v2、AES128/AES256/ChaCha、PN0、双方向、晚到界限收紧、多epoch、
公开DTO变更、认证/语法/资源负控和按random选择密钥。继承的290866个Windows ID/动作逐项不变。
包号红测56个ID中38失败；密钥加载红测13个ID中7失败，最终全部通过。
一次新测试把max=0误作零预算，修正为max=1/两个PING；没有改默认预算或继承断言，原失败保留。

最终干净e714完整命令覆盖bin-parser、pcapx、完整shark-cli及scripts/ci：
**290926 pass /25历史optional skip /0 fail，290951唯一ID**。
完整stdout SHA256：`4707898dc86a6073562a0a567d36c7f4834b9a3048cef2c8a244f11411ba2eee`。
相关QUIC/HTTP3/QPACK 659项零skip；显式插桩fuzz六种子、40568次执行，1CPU/768MiB、最多2同时进程，
第三进程拒绝正控通过。编译、corpus、cache和临时文件均在仓库外。

Windows两个实际原生控制台页面共36状态：

- 非法0-RTT PN4显示具体MalformedMessage，117B完整已认证拒绝明文拥有独立字节源；不送入协议状态。
- 合法晚到0-RTT PN1显示HTTP/3、HEADERS/DATA、POST及实际路径，递进到STREAM和H3帧；84B源逐字节正确。
- 两页鼠标字段选择、准确高亮、80×30/180×48、自适应pane及CP936恢复通过，根agent目视检查两张最终PNG。

首次正向QA把所有Stream Data字段序号用于已经按84B值过滤的可见选择器，导致测试导航失败；
修正QA选择器并核对值唯一后，重新实际运行两页。原失败保留，没有修改产品代码绕过断言。
Windows闭合1522 SHA256：`504cd160f7a015f623502460a4fcdf66fd8fb676818ab5d5bb92ce6a888bf747`。

两大公开捕获379455物理包的六份完整导出与M2-78逐字节相同；事前冻结全部旧hash，
按dataset先核SDK再执行CLI，没有native差异豁免。原四接口/VLAN QUIC contract/checker保持，
mismatch与coverage gap全部0，包括app-only/坏tag负控；不据此关闭全部高级恢复。
完整回放1515 SHA256：`acc78d79bf71337414ae99ae20561838b35ccbf9fee1a91f7aaf76c75a3c1a97`。

## 交付和后续

父M2-78最终 `776246d11c158ddbb82d9a4779305e9f60ec62ac` 已独立核验47约定检查名、91执行检查、
2 statuses全成功，315940原始ID/动作与事前清单相同，224新增和131084 required全pass/0 required skip。
自动draft跳过不计成功；官方17138966B产物digest/CRC/上传及实际checkout已核。
[Traffic](https://github.com/yaklang/yaklang/actions/runs/41513137101)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41513146083)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41513154685)。
闭合1494 SHA256：`3a24a9dae8a944616fedb5e592f6ea27693090415493e1f561182294785501dd`。

本批代码后只补里程碑文档，最终说明提交须独立316025原始ID（316000 pass/25历史skip）、
85新增及131084 required全pass/0 required skip和完整CI，不能沿用776成功。
本地资格1523 SHA256：`00784a5f925be25e6ddb91cff7536e84199be99cc0c9b1a58189e3a027d887c0`。
外部捕获、keylog/私钥、完整分析、PDML、fuzz、console/PNG及CI产物继续不入Git，两个fixture ZIP不变。
资格时可用61404577792B，无需再清理磁盘。仍只有已合并M1与M2 #5371两个阶段，不强推。

N09.R02有限关闭；R03先到受保护包的有界恢复、R04迟到SETTINGS后的QPACK恢复新增为明确验收项。
后续还包括0-RTT拒绝/生命周期、套件变化、丢包、跨方向与域隔离，TLS完整阶段、六TCP政策fuzz及IP字段/恢复。
SMB S01–S06、107完整记录/121命令头及字段分母留给独立承接人按任务书重新固定基线，主线程继续跳过。
