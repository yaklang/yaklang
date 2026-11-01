# M2：初始证书上下文、认证恢复与诊断字段

原生条件修复 `192a2cbd582e12b17b1b1b09f63348100b9864eb`，精简诊断文本为
`5f09c2456b24accb27dd7a253a4274ead02409e5`。
最终普通字段视图和干净SDK/flow/CLI构建为 `7aab30def3f2c545e108c15e1ac0297cd891809a`。
继续唯一M2分支/[PR #5371](https://github.com/yaklang/yaklang/pull/5371)，不强推或创建新阶段。
M1已合并；SMB继续搁置，没有新增SMB语义分析。

前一最终说明 `c1ed95906b0b8358ea7b7258a379e01bdc5817b2` 已独立47个必需检查名、
2个status及全部91个实际执行检查成功；92原始检查中唯一自动draft traffic skipped不计通过。
[traffic](https://github.com/yaklang/yaklang/actions/runs/41323522014)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41323526474)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41323529762)均完成。
313711个原始ID各一次run/terminal，313686 pass /25历史optional skip，43新增及131084 required全pass/0skip。
实际checkout、官方完整产物17017339 B及上传日志SHA256
`365b60a070d774c0b6ae916fe6c993429fd666360de0e3e86d0f7d344ea5ba49`一致。
闭合615 SHA256 `6fb9414a35fa71ce940ed1ebaac89fb0701757c426a027b34137b74e6495a09a`。
RAG成功官方产物也完整下载核验，4个查询ID各一次run/pass；它不代表后续本地上下文/页面修改已通过CI。

## 两个初始上下文条件

初始CertificateRequest的context必须为空，客户端Certificate应回显请求context。
依据 [RFC8446 §4.3.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.3.2) 和
[§4.4.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.4.2)，
在角色已确定、仍使用握手密钥的初始交换里，非空上下文不能按合法正文接纳。
此前两个内部向量完整的输入都能继续发布HTTP；现在给出具体MalformedMessage并停止后续明文发布。

应用密钥时期的客户端Certificate和服务端CertificateRequest仍保留其上下文。
应用epoch的认证恢复在正文分发前完成：缺失Finished时，只有授权application secret确实通过AEAD，
才采用恢复后的epoch执行当前条件；不把旧握手状态误用于PHA正文。
公开无角色语法API保持可检查性，不凭端口推断角色或制造缺失消息。

49个新原生ID覆盖三套件、两个客户端方向，以及初始空/非空、应用上下文、
缺Finished后从授权application secret恢复的两类消息。检查身份、序号2→3和epoch保留。
607首次QA把nil和存在但为空的字节切片混为一谈，失败保留；608改为先检查字段存在再逐字节比较，
保留真实的12个错误接纳及父测试失败（36 pass /13 fail），之后才修改产品。
609相关1149 ID全pass/0skip。

本补充不验证完整消息顺序、所有角色/阶段、PHA offer、context唯一性/多轮关联或证书类型协商。
已知服务端使用application keys发送Certificate的完整阶段约束也仍开放；N13不关闭。

## 原始mTLS源和两个衍生反例

证据全部位于仓库外 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。
源为本机原始mTLS抓包 `mTLS-CertificateRequest241-tls13-01/live.pcap`，17个物理包。
244独立源事实SHA256 `275ee2beace6074ccd1364b86ee926d3bd2e5dbe1b33644d082dfc48f4030286`，
已从原TCP/NSS/AES256-GCM确认方向、握手/应用epoch、记录序号和实际peer HTTP字节。
610只读这些原始输入，生成两个明确标记的同长AEAD控制：

- 初始服务端请求context由0变2字节，同时移除一个未选择的签名标识并修正嵌套向量长度。原严格244初始上下文oracle必须拒绝；内部向量仍完整。
- 初始客户端证书context由0变1字节，同时把证书签名数据缩短一字节并规范重编码DER长度。X.509结构仍可读取，TBS和公钥逐字节不变，证书及握手签名不声明有效。

保留全部原PCAP记录、时间戳、长度、TCP seq/ack及未受影响记录，只改目标AEAD和受影响TCP校验和。
不是新的端点流量，不声明修改后transcript、CV或Finished有效。
610 manifest SHA256 `d85163650be220fd18ac2a95410a7a368f9393c67eef14c6fa2457e968fea83d`。
目标原生PDU ID仅用于导航；身份绑定使用原始记录SHA、方向、epoch和帧6/8。

612以隔离配置、授权原keylog、真实端口57266 Decode As，对原流量和两个控制完整两遍TShark回放。
同一TShark SHA256 `662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b`。
4个目标握手的67个原始字面字段检查全部通过，包括上下文、长度、19/20个签名标识和完整证书字节。
参考仍展示这些消息及其字面值；保留参考行为，不把字段能展示当成初始上下文合法。

## 最终构建实际运行

627为干净7aab的三个二进制；CLI SHA256
`aca88977f1466c1bb9b6c00cbd25af268638bd322a13abd44d522782f381f657`。
Go1.22.12/windows-amd64，构建及大捕获回放沿用3CPU/2GiB，SDK小样本、参考及Windows有独立硬限回执。

628为旧17fe/新7aab × 三个输入 × SDK/CLI/flow，共18次实际执行：

| 输入 | 旧事件/HTTP | 新事件/HTTP | 其他约束 |
|---|---:|---:|---|
| 原始mTLS | 16 / 2 | 16 / 2 | SDK、CLI、flow全部逐字节相同 |
| 初始请求context反例 | 16 / 2 | 6 / 0 | 具体错误、17物理行保留；不发布成功正文/别名 |
| 初始客户端证书context反例 | 16 / 2 | 11 / 0 | 具体错误、17物理行保留；不发布成功正文/别名 |

每组新旧flow字节相同。较早617构建的619同组运行也保留；不将重复运行加成新的用例数量。

629再对原始PHA、重传、乱序、缺失、截断、损坏、真实四元组复用、TLS1.2 CCS/Finished、
初始TLS1.3 mTLS九个profile做旧/新SDK/CLI共36次实际运行。
全部完整导出分别逐字节相同；正例HTTP字节与实际peer文件逐项相同。

| profile | 物理帧 | HTTP / 应用epoch Finished | 约束 |
|---|---:|---:|---|
| PHA原始 / 重传 / 乱序 | 29 / 30 / 29 | 各6 / 1 | 一代连接、无重复发布 |
| 缺失 / 截断 / 损坏认证flight | 28 / 29 / 29 | 各4 / 0 | 不发布PHA Finished或最后一条请求，不补洞 |
| 真实同四元组复用 | 52 | 12 / 2 | 两代TCP连接，各6条HTTP |
| TLS1.2 / 初始TLS1.3 mTLS | 19 / 17 | 各2 / 0 | 各原始peer字节保持 |

这些有限profile不关闭完整PHA关联、TLS/QUIC高级恢复或所有TCP/UDP会话政策。

631又以最终7aab执行两个完整公开捕获的六次SDK/flow/CLI回放，回放前冻结614的六个完整输出SHA。
新六份输出全部逐字节相同，保留306380个原生事件、379455物理行及580既有publication，共380035 CLI行。
无重编号、通用数字替换或差异豁免。这是限定回归范围，不是全部协议与Wireshark全部正确性一致，亦非SMB再验收。

## M2-57：诊断代码与错误消息的有限展示合同

622真实Windows发现，即使原生文本精简，`Error: MalformedMessage: ...`仍把消息尾部截断为`emp…`，
导致严格字段选择失败。原失败、console cells、PNG和退出回执保留；没有放宽“完整可见”条件。

普通字段视图现在独立显示结构化`Diagnostic code`和原始理由。
只移除与结构化代码精确一致的一层`code: `前缀；不猜代码、不剥离不同代码或无空格前缀。
SSH decoder仍只在SSH命名空间拆分；其他协议的`ssh:`和内部冒号保留。
原始事件、SDK/CLI导出的完整Error及证据不修改；两个诊断字段不声称线上的字节范围。
既有early QUIC、evicted和其他专用视图不冒称经过本合同的统一化。
此合同减少重复前缀，不承诺任意长错误、所有窗口宽度或完整自动换行。

623保留8 fail /2 pass的红测；624完整CLI包1307 pass /2历史optional skip /0 fail。
10个新ID、9个诊断场景分别经过直接视图和packet/stream JSON worker，包含代码不一致、
无代码、内部冒号、仅一层包装及跨协议decoder负控，且检查调用者JSON保持和无借用范围。

630以最终CLI实际运行两个页面，共44状态，分别查看目标PDU5/帧6和PDU10/帧8。
在默认分栏的详情半窗内同时严格核验完整Diagnostic code和Error，root已分别看图。
75 B及893 B目标记录全部从真实字节窗逐页读回，SHA与610独立目标记录一致；不展示成功正文或借用错误字段范围。
mouse/Tab、packet/stream worker、80×30↔180×48、CP936恢复及拥有进程关闭均通过。
这是实际console cells/attributes的固定字体渲染，不泛化全部Terminal字体/DPI/IME。
621摘要字段名预检错误保留，622的产品截断失败促成修复；前期不足的页面不当作成功。

## 本地集合及新CI门

618完整受影响三包在5f09为239600 pass /12历史optional skip /0 fail，日志SHA256
`925a376884c5e4a49574019a781a8eab5fc4814cda79d4c351660b9554013c86`。
其后仅两个CLI文件改变；624完整CLI和626当前源码CI脚本又通过，所有原生输入逐文件不变。
632按唯一ID合并这些实际完整运行的集合：239610 pass /12历史optional skip，重复运行不叠加。
它不是一次命令输出；各源、范围、变化和完整父ID/终态分别核验。

在读取未来最终head产物之前，已冻结313770个原始ID：313745 pass /25历史optional skip，
父313711逐项保持，49 native和10 CLI新增均必须全pass；131084 required全pass/0skip。
manifest SHA256 `b82e3de90c8fcd233c2d42a9509a504f3703bcda91ce96326ceff2d565246d40`。
ASDU2worker/10161帧原ID及历史平台例外不变。
新最终说明head仍须独立47必需检查名、两个status、实际checkout、官方完整产物/上传日志及全部ID闭合；
旧c1ed成功不能代替本批。必需检查失败继续修复，不用skip或删除断言过门。

| 证据 | SHA256 |
|---|---|
| 三次完整新参考612 | `1b2ddb69592cde3e83f2ea87fc7ffa6e2c2d87dd6e6f2316d1094b4d35dec544` |
| 最终18次控制628 | `dac1e711d9200b4ee39e6be27d516e8e33d87ec1030022d1abcd9e5d9c38504a` |
| 最终36次恢复/复用629 | `dd97fd9947257166374cd5386a09da6f1d5b870c10dbc8673338194e8283f586` |
| 最终六次完整大包631 | `fddaa6ecdec01ea6556b9787fbc0db009810bfd2b3bd6271213e8ff2c5931fbd` |
| 最终两Windows页630 | `43b37077e4a6fe6a4e8350ed95ef45207d7ea9e2b4e2e3b82c73d3ee5245d016` |

2026-11-02验收续记：最终ce26已独立47验收检查名、2statuses及全部91实际执行检查成功。
313770原始ID逐项保持，新增59及131084 required全pass/0skip；官方16967503B产物SHA256
`05032644de53203749ad829ef2225e2decb35bea40daad805b9dda87d4c5c00e`与实际checkout/上传日志一致。
[traffic](https://github.com/yaklang/yaklang/actions/runs/41329494037)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41329497344)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41329500697)。645/646闭合分别为
`e097131dea56ef4c379fb9283cf110414d5caabf79106f14463b1343b8efec8b`和
`a4e644e259253ec0f83746d8937b7398731731e5a1aded7d3c4e8ca5b46a76bf`。
随后[480b5服务端证书epoch和新握手边界](shark-m2-tls-epoch-boundary-20261102.md)及其最终说明head另验CI；不沿用本报告ce26成功。

以下57/47是本报告原交付时的计数，最新58/48见主清单。
初始上下文归M2-56补充，诊断截断新增并关闭M2-57有限合同。
当前57父项，47有限完成、6进行中、1未开始、3 SMB搁置，剩10。
非SMB仍17包中11有限完成/6开放，粗估23–46 AI实施小时，等待及新发现另计。
外部捕获、秘密、完整导出、页面和fuzz corpus继续留仓库外；两个捕获ZIP及manifest不变。
