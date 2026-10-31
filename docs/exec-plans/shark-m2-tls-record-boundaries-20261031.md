# M2-41 TLS1.3 KeyUpdate record边界与旧epoch残片

本批从已ready的非SMB PR #5381 `39ca64b4468c81a43c2104ee909b0d21063ab5a9` 分出。
其最终47 checks+2 statuses成功，traffic实际checkout/完整日志/artifact已核，312523 pass、
25历史optional skip，131084 required全部通过。本批[PR #5382](https://github.com/yaklang/yaklang/pull/5382)
以其分支为基线；代码 `ab24d5ec7657a058d3d52a0925f9589e9964d0dd`，
SHA384 Finished正控长度修正及干净运行构建为 `6bacd80c8df9e4a3352f88cadaee889e5058ca3a`。
文档提交不冒充本地运行时的HEAD；最终交付SHA的CI结果另固定在PR说明及外部闭合收据。
SMB按用户要求跳过，[缺口记录](shark-m2-smb-deferred-20261031.md)不改变，也不引入#5380的生产代码。

这是M2-10.N12的有限合同，不是全部TLS恢复、其他换钥消息、QUIC或核心协议完备的声明。

独立来源与原失败
----------------

外部证据根目录为 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。原真实OpenSSL抓包131有53帧、
29 TLS record、25 AEAD record、6 KeyUpdate和12 HTTP消息；原捕获SHA256
`a17eac72939386defbda490063d0d1829b91fb4d1c9b59a0f62d13dae63adb0e`。
131使用真实peer及key更新，不是手造密文；本批176的六个控制是明确标记的独立AEAD变异，
不是新增真实peer流量。各控制保留前21个原packet record，只改最后frame22的payload、长度及校验。

176源合同SHA256 `9314fd72514eeed5b01bbd45c3dc1b26431cc9646fe97fb87542b077022ab4f8`：

| 控制 | 独立预期 | 基线39ca实际SDK | 修复6bac实际SDK |
|---|---|---|---|
| KU恰好结束record | 成功 | 通过 | 通过 |
| KU在两个旧key record中分片 | 成功 | 通过 | 通过 |
| KU后inner padding | 成功 | 通过 | 通过 |
| 同record两个KU | 拒绝，不换key | 错误接受两个KU，2项差异 | 拒绝，无KU字段 |
| KU分片间夹AppData | 拒绝，不发表应用明文 | 错误接受并发表HTTP，3项差异 | 拒绝，无伪HTTP |
| KU后残缺握手头 | 拒绝，不换key | 错误接受，2项差异 | 拒绝，无KU字段 |

源有9个受控AEAD record和12个拒绝负控；独立Python进行HKDF/AEAD认证。
规范依据为[RFC8446 §5.1](https://www.rfc-editor.org/rfc/rfc8446.html#section-5.1)
及[§4.6.3](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.6.3)：KeyUpdate必须在record边界结束。
Wireshark4.6.9的版本及六个完整前缀参考共7次实际运行均exit0；它也展示三个规范负例的KU且无expert。
参考的字段展示不能作为endpoint接受非法序列的依据。该参考汇总SHA256
`cffc55681d7d92db8e30d4fce4d289ff353afb31f344e9a67647ae581dcd085d`。
原连接主动关闭NST发送，不能声称真实NST三个控制已覆盖；前置NST仅是明确标注的合法authored unit正控。

恢复源180是4 intended/3 observed record：旧epoch KU头4B，缺失旧epoch末1B的23B wire record，
新epoch应用数据，再完整新KU。保留遗漏，不制造压平TCP序号的PCAP；仅验已知record边界观察合同。
合同SHA256 `9ad5090ee874e6727bc3c0f03b1f5aa848724aaa2f170185f50b8e85bc1dfd10`，
有独立认证及9个源负控，最后record与原131 frame30完全一致。

修复行为与回归
--------------

- 仅在确认KU恰好结束当前record后调用换key handler。保留旧key分片、合法前置消息及inner padding。
- 连续观察的同方向握手残片不允许被AppData穿插。拒绝后不安装新secret/IV/AEAD，不把明文交给HTTP；
  已认证旧record的序号可正常递增，不能与非法重置新epoch seq0混淆。
- 恢复试探只在认证成功且实际跨epoch时提交旧方向握手残片的退休及认证lineage隔离；
  失败试探、反方向状态、同epoch跳号及原应用gap合同不变。
- 详情在Record header→Record decoding→Observed connection递进展示，恢复/遗漏/丢弃字节属于上下文，
  不借密文范围；握手独立字节源的[4,5)只高亮请求值，拒绝记录不生成成功KU字段或握手明文源。

红182保留8 pass/22 fail；红183重排状态断言后确认8个真实旧残片遗留失败，未删除断言。
绿184相关50 terminal通过。原生广回归188为5038 pass/0 skip/0 fail；CLI完整187为
1055 pass/2历史optional skip/0 fail。UI新增断言先红后绿。SHA384 Finished fixture此前误声明32B，
修正为该suite的48B后，196的8个相关terminal重跑通过；不将测试构造器缺陷算产品修复。
新增7 native父测试+1 UI父测试，共37 terminal-action ID（含父级与中间级，27个叶），CI须逐ID核对。

新干净196构建的events/flows/Yak/实际Feed observer均成功。Yak SHA256
`1ab5e53c92ea51a631ceccedfd2c7a411ed50d79b9670372df2be2237920ca6d`；
events SHA256 `516b0c3f4eeed0a00ac70d82136473d3e75477cf7cd30912c7410423ef3f0d03`。
203是真正带Go fuzz插桩的独立运行：90s窗口及bootstrap共120.328s、96752次、11个新interesting输入、
0 fail；日志SHA256 `454f93b5707b31e339d38ab867f859ac1fa0cbc30069b373d3175df1f6addf6e`。
三套AEAD由常规回归覆盖，fuzzer当前用AES128；不能推导全部TLS状态序列已覆盖。

197最初的checker178错误要求被拒记录也有CompletedIn，保留其1/2/1投影失败。
既有API合同只给成功解码/延迟消息CompletedIn；202另冻v02投影，正例仍强制22，拒绝强制缺失，
raw SHA/完整字节/域/代次/贡献refs22/无伪KU等断言保持。同一v02重跑基线和修复：
基线三错误仍2/3/2，修复六例全通过，并拒绝12个DTO负控。未把产品输出改成伪完成帧。
闭合202 SHA256 `08b316acb9d7b443101d01dbf48216d597f8d30ec73ec39832c10704b02da28a`。

运行、页面和未关闭边界
--------------------

198及200各实际运行七个原真实捕获及删除/乱序控制；SDK和实际Feed observer输出逐字节一致，
与先前4477原导出也逐字节相同。六个strict门通过；永久缺客户端KU/请求的场景仍有6个HTTP差异，
仅1完整请求/响应及未知方法头，不猜GET，不因缺信息切错HEAD/CONNECT正文。callback实到时刻与
CompletedIn来源归属分开验证，不以回填旧时间戳冒充提前回调。

207/208实际复验8个旧TLS恢复/复用场景，7个strict通过；TLS1.2 C1未配新SYNACK场景仍exit4/8差异。
独立来源审计显示额外4TLS+3HTTP均旧Flow2及实际Feed57，不能直接称新Flow串入；保留政策缺口。
209四个QUIC域组合全部SDK exit0；接口/VLAN full/full两个strict通过，各域2022认证record、
3坏tag/6tail与4556来源检查。两个app-only场景各2664 strict差异仍保留，准确UDP预览不等于完整QUIC。

Windows214实际三页：合法跨record KU、双KU拒绝、握手/AppData交错拒绝，每页18状态，共54状态。
原201/211因QA滚动未达到目标字段而exit2，保留失败及图片；213实际同构worker只定位字段导航，
214仅调整滚动次数，不改变源、字段值、高亮或拒绝预期。UI内部认证字段投影名称与可见标签不同，
旧closure期待Authentication Verified失败；215版本化适配只改为实际标签Record AEAD verified，
SDK布尔true、来源/完整字节/分组/精确高亮/拒绝/鼠标/Tab/80x30↔180x48/CP936门保持。
随后列表完整KU摘要断言仍失败：一物理帧含多个PDU时，前面的incomplete摘要将后面的KU枚举或
malformed原因挤出180列。该问题单列M2-42，不删除失败条件，也不称完整UI通过。
215 v03仅将这个实际失败与其余详情/来源门分别报告，full finite_UI_acceptance保持false。
root实际看完三页selected/narrow/restored九张图，独立证明SHA256
`eaa355d310d29be503c473d88b9900dbfac3d508aa15fa1a331b4a9ceb850216`。
这里是控制台cells/attributes实际输出，不是Windows Terminal字体、DPI、IME或所有协议页验收。

两个大捕获199的SDK/CLI/flow共6次实际回放全部exit0；216完整比较六份输出均与旧133逐字节相同，
无ID归一化。因此137的固定133项字段门以完整SDK精确字节身份绑定新6bac构建。
Weber为42223事件/60种协议，Sharon为263109事件/54种协议；原真实捕获在仓库外，
事件数量不等于物理帧数量。216闭合SHA256
`734273f652dc551b0d0c156546159cec66a4e726990b7b567e3a3aae769fdf13`。
这不是所有协议/字段/会话/fuzz与Wireshark一致，也不冒称本轮重新执行完整TLS TShark oracle。

非SMB剩余有限清单为N07–N11五项，粗估13–26 AI实施小时；新增M2-42列表摘要1–2小时另计，
不含CI/构建/执行等待及新问题。
N10.02本轮仅做原328127帧来源/参考审计：两同向SYN+URG、不同ISN/载荷，没有SYNACK/ACK/关闭证据，
暂不足证明两条建立连接或产品误合并；原IPv4长度0与Wireshark presumed-TSO显示108分列，
严格reader拒绝及初版投影失败都保留，未运行新产品、其余5例未审。不把参考stream号当连接事实。
其他CH/SH/Finished/EOED的record对齐、零长握手片段、TLS高级恢复和QUIC CID/epoch/STREAM/QPACK/0RTT均未由本批关闭。

捕获ZIP/manifest/批次index/rules归档哈希保持不变；公开大型PCAP、keylog/密钥、真实抓包、
完整导出、源控制与fuzz corpus和截图全留仓库外。磁盘资源检查与限额保持，用户进程不动。
