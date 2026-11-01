# M2：Certificate 密钥阶段与恢复后的握手边界

本批完成 N13.P01 的限定合同，并将新发现的恢复边界问题明确追加为 M2-58 / N13.C03。
M2 仍未完成：58 个父项中 48 个有限完成、6 个进行中、1 个未开始、3 个 SMB 搁置，剩余 10 个。
非 SMB 仍是 17 包中 11 个有限完成、6 个开放；两个分母不相加，也不按协议数或测试数估算完成率。
SMB 只维护任务书和既有回归，不进行新的语义分析。

## 实现与范围

原生修复为 `26c07b785bc6254d4f3da9ffc1bcf070c946563b`；摘要与字段组为 a713，
混合行提示为 57cc，最终代码、SDK/flow/CLI 干净构建为
`480b5d12ee62d9f4f37dc13ad08313edc16ea0ae`。
均为同一 M2 的普通提交，没有历史改写、强推或第三个阶段。

1. 观察到 ClientHello、已知服务端方向时，TLS1.3 Certificate 只能使用握手密钥。
   正文向量仍由角色无关的公开 API 检查；记录通过应用密钥认证，不代表服务端此时可以发送 Certificate。
   客户端 PHA、初始空 context、未知角色的保守处理和 TLS1.2 原合同保持。
2. 认证恢复到新 epoch 时，如果该方向还有旧的半条握手消息，而新记录也是握手内容，
   丢弃旧片段后不能把新字节直接当成消息起点。记录认证事实保留，握手边界明确不可用；
   不发表消息正文或由碰巧匹配的 KeyUpdate 头推进密钥状态。
   同一 epoch 的后续记录也不能凭一个合适的头恢复解析；另一方向不受污染。
   `Missing Earlier Plaintext`沿用恢复序号/应用缺口口径，不抵消另列的旧握手片段和边界未知事实。
3. SDK 摘要使用 `Handshake · boundary unavailable`。
   CLI 每个包保留一次醒目的 `TLS handshake boundary unavailable` 提示，
   同包的合法 CertificateRequest、KeyUpdate 或更具体的 HTTP 仍保留。
   不通过搜索应用文本决定是否省略提示，也不改写应用文本。
4. 详情按 Record header → Record decoding → Observed connection 递进。
   认证、丢弃片段数、边界理由没有虚构字节范围；密码套件和 SNI 仍可作为已观察的连接上下文，
   不伪装成本条记录的握手字段。拥有字节源仍是完整重组记录。

规范依据：[RFC8446 §4.4.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.4.2)、
[§4.6.2](https://www.rfc-editor.org/rfc/rfc8446.html#section-4.6.2) 和
[§5.1](https://www.rfc-editor.org/rfc/rfc8446.html#section-5.1)。
握手消息不能跨密钥变化。被动分析遇到缺失字节时，也不能反向断言这些未见字节一定违规；
这里保留记录认证、拒绝无依据的消息对齐，不重建未捕获的消息。

## 独立来源与实际旧、新回放

复用本机真实 OpenSSL 来源550：41 个物理包、23 条 TLS 记录、12 条实际 peer HTTP 消息。
来源552已独立验证 AEAD、CertificateVerify 签名、两个 Finished MAC 和 HTTP 原字节。
源码事实 SHA256 `99936664ba5827738451c7bb280e4b0a5b4d8747be69d52784f3a8603c3eb84f`。
本批三个控制都是构造反例，不冒称新端点会话、端点接受或有效完整 transcript。

| 输入 | 独立固定的事实 | 旧版实际结果 | 新版实际结果 |
|---|---|---|---|
| 原始41包 | 正常860B Certificate、原握手密钥 | 1条证书、12 HTTP | 三份完整导出逐字节相同 |
| 服务端应用密钥封装原证书636 | 860B正文/DER不改，882B记录用原授权应用密钥序号0认证；18个握手序号候选均拒绝tag | 接纳 Certificate，6 HTTP | 记录认证仍可见，明确 MalformedMessage；无证书正文，按既有连接失效策略停止后续应用发布 |
| 跨epoch证书尾部650 | 原握手密钥封装前7B，应用密钥封装剩余853B；完整客户端流不改，服务端仅替换该段 | 发表握手类型84 | 边界不可用，不发表消息，6 HTTP保持 |
| 片段后恰有完整KeyUpdate头650 | 原握手密钥前7B；应用密钥封装5B KeyUpdate编码 | 接纳KeyUpdate并推进密钥 | 只接纳记录认证，不接纳消息或再次换钥，6 HTTP保持 |

650 两个控制分别增加22B、减少826B。逐包显式重算受影响的 TCP seq/ACK、IP/TCP 校验和及捕获长度，
再独立拼接完整双向 TCP 流：客户端完全相同，服务端与指定替换逐字节相同；41条物理记录和时间戳保持。
三个控制的所有旧、新 flow 导出分别逐字节相同。654/680共24次实际 SDK、CLI、flow 执行，
不是仅检查标签。客户端合法字节与错误服务端阶段的处理策略分开记账。

## 与 Wireshark 的边界比较

647/651使用隔离配置、原授权 keylog、实际端口 Decode As 和乱序重组，两遍完整回放共五次。
TShark SHA256 `662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b`。
每个输入均保留全部41包并核对目标记录外层原始头；原始证书字段来自独立向量，未从产品输出生成期望。

原流量参考为1条证书、12个HTTP包；三个控制均没有参考证书正文、各有6个HTTP包。
参考在 Finished 之前没有替控制试用应用密钥，不能把它未展示正文误称为它已经执行了阶段校验。
产品有独立 AEAD 证据，因此能够明确区分“应用密钥认证成功但阶段非法”和“认证后握手边界未知”。
636产品的后续发布为0、参考仍发布另一方向6条HTTP，是已说明的连接失效策略差异，不冒称全部语义一致。
两个残片控制保留6条HTTP；新控制消息没有假接纳，也没有借其他记录或连接的正文补齐。

## 丢失、乱序和连接复用复验

683 对九个原始/衍生 profile 各跑旧、新 SDK 和 CLI，共36次：

- 原始、完整重传、认证航班乱序：各6条HTTP、1条PHA Finished；与实际 peer 字节逐一相同。
- 永久缺失、截断、损坏：各4条HTTP、0条PHA Finished；没有补造最后一个请求。
- 真实同四元组复用：两个 TCP 代次，各6条HTTP、各1条PHA Finished；旧连接不替新连接提供状态。
- TLS1.2 CCS/Finished 和初始 TLS1.3 mTLS：各2条HTTP，原有阶段和正文保持。

八个 profile 的 SDK/CLI 完整输出逐字节相同。损坏 profile 仅 SDK 的20/21两个摘要、
CLI第20行提示改变，均由旧输出已存在的认证/内层类型/缺失握手事实逐项资格检查；
除这些明确字段外，所有值、拥有字节、来源、关联、连接代次和物理行完全相同。
原668/674的严格“全输出相同”失败保留，没有改动协议字段断言来过门。

## Windows 实际页面

682在最终构建上实际启动三页，共66个控制台状态。根线程已逐页查看默认180×48的真实cells/attributes渲染图。
具体错误和边界说明在详情左半窗完整可见；75/893B旧报告不沿用，本批分别读回完整882B、875B、27B独立记录。
字段无借用范围，诊断、握手消息和连接上下文分组正确；packet/stream worker、鼠标、Tab、
80×30↔180×48及退出CP936恢复通过。所有本任务 UI/worker 子进程关闭。
图片属于实际控制台状态渲染，未泛化为全部终端字体、DPI、IME或桌面像素验收。

旧SDK harness未导出Error；QA DTO仅在id/protocol/status/byte_source与实际CLI PDU完全一致后补其完整错误。
没有把手写理由当成SDK自身的错误导出。拒绝的记录和两个上下文别名仍分别检查真实拥有源及origin。

## 完整大捕获与本地测试

686在最终构建上对原始 Weber/Sharon 各跑 SDK、flow、CLI，六个完整旧输出SHA在运行前固定。
六份新导出全部逐字节相同，无重编号、删除记录或差异豁免。688从这些新完整导出重新统计：
306380个原生事件、379455个物理行、580个publication行，总380035个CLI行；每协议计数和完整原导出都留仓库外。
这证明本批未改变既有大捕获输出，不把它当全部 Wireshark 字段、会话和fuzz差异已经解决。

648在26c07上完整四包：279960 pass /23历史optional skip /0 fail，
stdout SHA256 `99aa10bc234932aaf86c7a852d12ea7043795e8b5b554191da7fb68e0e795460`。
之后原生只增加已明确测试的摘要分支，658全部TLS/摘要相关1696项通过；
最终CLI/CI脚本678完整1319 pass /2历史optional skip。逐文件资格检查后，唯一ID并集为
279977 pass /23历史optional skip；不是一次命令输出，也不叠加重复测试。
本批不把旧58636插桩执行冒称为新阶段的持续fuzz验收；普通完整运行仍执行既有fuzz种子。

## 最终提交CI门与保留失败

前一 `ce26c0d9238194e4dde43547e536199a357ff803` 已独立全绿：47个验收检查名、2个status、
全部91个实际执行检查成功。313770个原始ID与事前manifest完全相同：313745pass/25历史optional skip；
131084 required全pass/0skip。官方产物16967503B，SHA256
`05032644de53203749ad829ef2225e2decb35bea40daad805b9dda87d4c5c00e`。
[traffic](https://github.com/yaklang/yaklang/actions/runs/41329494037)、
[Essential](https://github.com/yaklang/yaklang/actions/runs/41329497344)、
[Diff](https://github.com/yaklang/yaklang/actions/runs/41329500697)。其成功不包含本批新代码。

685在读取未来最终提交的产物之前冻结313844个原始ID：313819pass/25历史optional skip，
父313770逐项保持，新增63 native +11 CLI共74项均必须pass，131084 required必须全pass/0skip。
manifest SHA256 `bae878001ffb480df14c3d85de25ba604e1d0c06d8207eb8fadff0560cc24b82`。
本地ASDU3worker与CI2worker分别检查全部10161帧；正式CI保留原始worker2路径，未归一化ID或削减帧。
两个Windows专属测试和其standalone child历史skip分开核验，不作为新增Linux验收项。
包含本文的最终提交仍须独立47验收名、2status、实际checkout、官方完整产物和全部原始ID闭合。
自动draft skipped不计通过，最终结果更新同一M2 PR #5371。

638保留12个真实缺陷用例及父测试失败；640/642暴露跨epoch残片会被当成新消息，推动恢复修复。
657/670保留摘要和同包提示的原红测。649错误提交号在编译前被预检拒绝，后续独立目录679成功；
665把本地worker路径当CI新增ID的错误被拒绝，685按未改动配置逐项资格核验。
656/675的HTTP/上下文过宽假设、668/674的旧显示不变假设和684脚本作用域错误均保留，
最终执行不覆盖它们，也不借这些QA错误取消产品断言。

## 可复核证据与剩余范围

仓库外根目录：`C:/tmp/yak-shark-ESP-SSH-UI-20261028/`。

| 证据 | SHA256 |
|---|---|
| 错误epoch独立源636 | `79ed981d4f464bf40d720c8e5ca54071226ccbccdedf01e291fadab8df5f553d` |
| 两个残片控制650 | `bf7eae218e4edd66aace74e393a917820c5d0aa69448da4cb406eea34b5e9820` |
| 两次完整参考647 | `a01b78989b62f4de001f904cf89eaa0f6aef2edf6f052a2549b16253cc24b4d6` |
| 三次完整参考651 | `d5914c235c8e2ce41d652f3a4d62db6e9d97ee8b646ec5a49f80d3a26b9f3447` |
| 最终四输入/三种导出680 | `4239cd1f57b4d5fa97f7da5532490e63cfe1a2fdc74c73e6f855e63779d37453` |
| 九恢复/复用profile683 | `37bc2271a2ac2d7fd68b30c9fd660a1016d53801aae9158c28040bab666f0b65` |
| 三页Windows682 | `e77bc8a633df79a4a192d082474af0b76c0770bf950804b31cce116314e28a3e` |
| 六完整公开回放686 | `9bf3e518b4132ded9a7f5df6a760671d2c4e38557d372d9a73273983ac4db952` |
| 最终输入、页面目检和全导出计数688 | `14c4fe24bc23b54aa632572578e7504ca2017c95d7d9ef34687ab1b800040f37` |

N13另有 PHA offer/context唯一性、多轮响应关联、完整现支持阶段/协商依赖三项未开始。
更后续epoch、完整缺失记录后的握手边界、QUIC高级恢复及证书类型/信任不由本批覆盖。
N08 TLS1.2 SYN政策、N09缺握手QUIC、N10六TCP/N11二十四UDP fuzz会话政策、N14有界IP重组仍开放。
六包粗估23–46个AI实施小时，构建/回放/CI等待和新发现另计。
所有外部PCAP、真实抓包、keylog/密钥、完整导出和图片仍在仓库外；两个捕获ZIP/manifest不变。
