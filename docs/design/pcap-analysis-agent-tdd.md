# PCAP 分析 Agent 的 TDD 开发工作流

> 状态：开发流程约定
> 适用范围：`common/ai/aid/aireact/skills/pcap-analysis`、PCAP 内置工具及真实流量测试集
> 脚手架：`common/ai/aismoking/live/pcap_lab/`
> 当前任务、优先级和执行状态：[PCAP 分析 Agent 单一优化 TODO](./pcap-analysis-todo.md)

## 1. 目标

用真实调查任务驱动 `pcap-analysis` Skill 和系统内置工具持续改进：

- 被测 agent 接收用户问题，自行寻找 Skill、选择工具、执行命令并形成调查结论。
- 开发 AI 负责选题、运行、收集轨迹、核查复盘和决定是否修改。
- 独立 AI 负责阅读轨迹和回答，分析工具选择、分析质量和改进机会。
- 用户收到有证据的批次汇报，而不是一组缺少上下文的“通过/失败”数字。

这里沿用 TDD 的核心思想：先写下可重复的调查任务和预期证据边界，再运行真实 agent，根据失败和绕路改进工具或 Skill，最后重跑原任务和相邻任务。测试不要求 agent 遵守固定调用顺序，也不要求所有问题都由内置工具完成。

## 2. 角色与边界

| 角色 | 负责什么 | 不负责什么 |
| --- | --- | --- |
| 开发 AI | 选题、运行、核查证据、实现改进、组织复测 | 不把未经验证的复盘结论直接当成根因 |
| 被测 agent | 完成用户调查任务，自由选择内置工具、脚本和命令行 | 不接收测试文档、预期答案或参考复盘 |
| 独立复盘 AI | 阅读任务、回答和轨迹，指出行为、证据和可改进点 | 不修改实现，不把单次模型波动当成稳定结论 |
| 用户 | 决定优先级和是否接受产品方向 | 不需要启动 GUI、手动复制 API key 或逐步操作工具 |

样本中的域名、IP、脚本和文件内容只作为离线证据。任何测试都不得访问样本基础设施、执行样本代码或回显原始凭据。

## 3. 单轮工作流

### 3.1 选择测试方向

每轮先选 1～3 个方向，优先选择：

1. 有真实企业问题、但尚未被真实 agent 验证过的场景。
2. 上一轮出现过绕路、漏查、错误结论或能力边界表达不清的场景。
3. 已修改工具或 Skill 后需要回归的相邻场景。

选择结果要记录：目标、样本、自然语言任务、观察重点和预算。不要为了凑数量同时启动大量真实模型任务。

### 3.2 定义自然语言任务

任务应像用户提出的调查问题，不能直接给工具名、消息 ID、固定查询路径或预期答案。测试样本可以复制到独立工作目录并使用中性文件名。

开发者参考 `TASKS.md` 中的预期观察、证据要求和禁止捷径，但不要把这些内容拼进被测 agent 的任务。

### 3.3 运行真实 agent

使用数据库中的 GUI 模型配置，在独立 home、profile、工作目录和 session 中运行。默认保留内置工具和命令行能力，不限制工具集合。

运行需要保存：

- 任务、样本哈希、构建版本、Skill 哈希和配置摘要。
- 工具调用、参数、结果、错误、重试、命令行输出和最终回答。
- 运行时 timeline、evidence、stream 和工具 artifact。

`finished` 只表示运行结束，不表示分析正确；记录不完整、超时或中途取消也必须如实保存。

### 3.4 独立复盘

复盘 AI 先阅读 `task.txt`、`run.json`、`answer.md`、`calls.json` 和 `trace.md`，再按需查看原始事件和工具产物。复盘重点：

1. 用户问题是否被回答，事实、推断和未知是否区分。
2. 内置工具是否被发现，参数和输出是否帮助了分析。
3. 命令行、脚本或外部程序解决了什么问题；它们是否揭示了系统工具的发现、易用性、输出或功能缺口。
4. 是否存在重复查询、无效重试、错误配对、截断误用或遗漏的证据范围。
5. 哪些修改值得优先做，引用具体事件或调用，并给出复测任务。

命令行不是失败。真正需要识别的是：系统已有能力为什么没有被优先使用，以及哪些重复工作值得内置。

### 3.5 开发 AI 核查并形成结论

开发 AI 对复盘中的关键发现做一次针对性核查，把结果分为：

- **已经好用**：分析充分、证据可靠、过程合理，保持现状。
- **值得改进**：已有能力不易发现、参数难用、输出不足，或重复脚本工作适合内置。
- **暂不确定**：单次波动、模型服务异常、证据不足或需要相邻样本复测。

涉及确定性解析、哈希、分页、完整性或查询语义的问题，应补 Go/Yak 回归测试；涉及 Skill 指导、工具发现和自然语言使用体验的问题，应保留真实 agent 任务。

### 3.6 修改、复测和汇报

修改后至少重跑：

- 原失败任务。
- 一个相邻任务，检查是否出现回归。
- 受影响工具的确定性测试。

单次成功只能说明本次运行改善；需要重复观察才能声称稳定改善。没有收益的改动应撤回或继续观察。

### 3.7 改进约束

Skill 只描述调查流程、工具选择和证据边界。不要把协议解析器的字段路径、JSON key 或其他必须与实现同步修改的结构写进 Skill。解析结构变化时，不应迫使 Skill 一起改。

查询摘要需要提供更好的语义披露。这是后续优化中的通用要求，不限于某一种协议：摘要应让 agent 判断一条记录是否相关，而不必为了看基本内容逐条打开详情。完整字段树仍由详情查询提供。具体披露跟随工具输出演进，不回写到 Skill。

DNS 隧道一轮曾提出在 Skill 中提示 TXT 的具体字段，或为 TXT 增加专用解码。两者都不采用：字段路径与解析实现强耦合；这次需要解码的是载荷内部的 base64，不是 TXT 记录类型。查询摘要是否披露更多语义，仍按上面的通用要求单独判断，不把某一种记录类型写进 Skill。

## 4. 仓库中的真实测试目标

以下目标来自 `testdata/enterprise-traffic-analysis/`。自然语言任务是建议起点，不是答案；预期观察、证据要求和禁止捷径以各自的 `TASKS.md` 为准。

| 优先级 | 目标 | 样本 | 建议自然语言任务 | 当前重点边界 |
| --- | --- | --- | --- | --- |
| P0 | HTTP DGA/C2 | `http-dga-c2/capture.pcapng` | 分析 HTTP 活动，整理请求响应链，判断是否存在可疑上传，说明依据和不能确定的地方 | 已有 `body_summary`；URL 参数不是权威身份；DGA/C2 仍是推断 |
| P0 | DNS TXT 隧道 | `dns-tunnel-c2/capture.pcap` | 分析 DNS 活动，判断是否存在异常隧道或命令下载通道，区分事实、推断和未知 | 需要 TXT 内容、分片顺序、长度和语义证据；不得执行脚本 |
| P1 | SMB/DCE-RPC 横向移动 | `lateral-movement-psexec/capture.pcapng` | 评估这段 SMB/RPC 流量能否支持横向移动候选，列出网络事实、推断和未知 | SMB2 摘要可解析；Endpoint Mapper 和 svcctl 业务语义可能缺失 |
| P1 | 主机与用户识别 | `host-user-identification/*.pcap`，共 5 个 | 建立 IP、MAC、主机名和用户候选，标注来源、时间、关联方式和置信度 | 当前脚手架只接受单文件；多文件导入需要先扩展；DHCP Hostname、Kerberos DER 和 LDAP SASL 存在解析边界 |
| P1 | RedLine 下载链 | `redline-infostealer-2023-07/capture.pcap` | 还原疑似恶意软件下载的网络时间线，定位主机、HTTP 跳转、TLS SNI 和 C2 候选 | 无 keylog 时 TLS 正文不可见；C2 候选不能写成已确认；下载对象无法仅凭普通摘要验证 |
| P2 | HTTPS 可见性与 keylog | `https-keylog-dridex/capture.pcap` + `capture.keys` | 比较无 keylog 和有 keylog 时的可见范围，说明解密覆盖率和无法验证的部分 | `pcap-inspect` 支持 keylog；当前 `load_pcapfile/query_pcapdb` 不支持 keylog，AI 持久化链路应明确报告未实现 |

`public-corpus-index/` 只记录后续语料接入建议，不作为当前 agent 测试目标。

## 5. 当前脚手架和执行方式

脚手架入口为 `common/ai/aismoking/live/pcap_lab/run.py`：

```sh
# 真实 agent 运行
python3 common/ai/aismoking/live/pcap_lab/run.py run \
  --capture testdata/enterprise-traffic-analysis/http-dga-c2/capture.pcapng \
  --output /tmp/pcap-lab-http-01

# 不请求模型的轨迹索引
python3 common/ai/aismoking/live/pcap_lab/run.py inspect \
  --output /tmp/pcap-lab-http-01

# 独立 AI 复盘
python3 common/ai/aismoking/live/pcap_lab/run.py review \
  --source /tmp/pcap-lab-http-01 \
  --output /tmp/pcap-lab-http-review-01
```

默认从 `$YAKIT_HOME/yakit-profile-plugin.db` 读取 AI 配置，并复制到运行目录的独立 profile。省略 `--binary` 会构建当前工作树；复测修改必须重新构建。当前运行器一次只处理一个捕获文件；主机与用户识别、keylog 对比任务需要先扩展脚手架，或明确记录为能力缺口。

## 6. 批次汇报格式

每轮向用户汇报时使用以下内容，保持简短并附运行目录或事件引用：

```text
本轮方向：
  为什么选择这些任务，运行了哪些样本和预算。

已经好用：
  哪些调查能力、工具选择和证据表达已经达到要求，建议保持。

值得改进：
  观察到的具体行为、影响和证据；区分行为事实与根因假设。
  每项给出优先修改建议、预期收益和复测任务。

暂不确定：
  单次波动、能力缺口或缺少相邻样本的情况。

下一步：
  是否实施修改、需要扩展的脚手架，以及下一轮准备观察什么。
```

汇报不强制每轮都产生修改项。如果当前能力已经达到任务要求，应明确建议保持，并记录下一轮更难的测试方向。

## 7. 完成标准

一轮测试只有在以下条件满足时才算完成：

1. 被测 agent 的输入、构建、样本和预算可追溯。
2. 运行轨迹和最终回答已保存，异常和中断没有被伪装成成功。
3. 独立复盘有具体调用或事件依据。
4. 开发 AI 已核查关键发现，并明确结论属于“保持”“改进”还是“暂不确定”。
5. 如果实施修改，已记录修改前基线、修改内容和复测结果。

## 8. 测试与优化历史

> 更新：2026-10-30
> 运行根目录：`/tmp/pcap-agent-lab.4eSy4K`
> 分支：`feat/pcap-ai-real-traffic-tool-upgrade`
> 这里只记录真实 agent 测试、复盘结论和已经做出的取舍。活动任务、优先级和验收统一维护在 [PCAP 分析 Agent 单一优化 TODO](./pcap-analysis-todo.md)。
> 测试中确认的协议解析证据集中记录在 [PCAP 真实测试中的协议解析缺口](./pcap-analysis-parser-gaps.md)。

### 8.1 已测完

| 目标 | 运行 | 结果 | 决定 |
| --- | --- | --- | --- |
| HTTP DGA/C2 | `baseline-02` 296 秒，正常结束；`review-http-01` 385 秒，正常结束 | 结论可用。agent 自行加载 `pcap-analysis`，用 `load_pcapfile`、`protocols`、`details`、`body_summary`、`sessions` 整理出 4 组请求响应，并把 POST 外泄标成候选。`body_summary` 给出长度、哈希和敏感标记，没有把正文当成唯一证据。 | 分析链保持。长期取舍见 8.2，活动任务统一由 [单一 TODO](./pcap-analysis-todo.md) 跟踪。 |
| DNS TXT 隧道 | `dns-01` 291 秒，正常结束；`review-dns-01` 到 600 秒预算，`engine_finished=false`，没有 `feedback.md` | 结论可用。agent 识别查询主机、两台 DNS 服务器、TXT 分片、空应答轮询和 NBNS 噪声，并区分事实、推断、未知。它在思考中把 TXT 文本里的 base64 解成了脚本，没有调用独立解码工具。 | 不为 TXT 增加专用解码，也不把 TXT 字段写进 Skill。独立复盘没有完成，不能把超时当成“没有问题”。 |
| SMB/DCE-RPC 横向移动 | `smb-01` 跑满 600 秒，`engine_finished=false`，没有 `answer.md` | 网络事实已经足够：SMB2 会话建立，`ADMIN$` 写入 `PSEXESVC.exe`，`IPC$` 打开 `PSEXESVC` 管道并建立 IOCTL/WRITE/READ 通道；135 端口为 EPM，49688 为带 NTLMSSP 的 svcctl，agent 从原始流看到 9 个请求均有响应且无 Fault/Reject。超时发生在继续确认命令是否真正执行、尚未收口时。 | SMB 分析链保持。DCE/RPC 认证 PDU 的结构化解析有真实缺口，列为候选修复；命令执行仍应标为未知。 |
| RedLine 下载链 | `redline-01` 285 秒，正常结束；`review-redline-01` 525 秒，正常结束并产出 `feedback.md` | 分析链可用。agent 只调用 `load_pcapfile` 和 `query_pcapdb`，没有使用 shell，恢复了 HTTP 跳转、TLS SNI、自定义端口会话和双向字节量。未识别 TCP 流只有 base64 原始块，agent 靠人工解码才得到可读契约和枚举字符串。 | 分析链保持。增加未识别流可读取证能力的候选修复；最终报告存在候选写成已确认、完整回显疑似令牌和枚举列表的问题。 |
| 主机与用户识别（单文件） | `host-user-windows-01` 606 秒，正常结束；`review-host-user-windows-01` 574 秒，正常结束 | 单文件结论可用，但主要来自 15 次自写 Python 解析。内置库已有 Kerberos 53 条 decoded、LDAP 47 条 decoded、SMB2 41 条 decoded；agent 未使用 `protocols`、`fields` 或 `details`。UDP/137 的 20 条 NBNS 被标成 DNS 且名称未解码，UDP/389 有 34 条 CLDAP `unrecognized`。 | 保持单文件调查结论。身份摘要、NBNS 名称解码和 CLDAP 分发记入协议缺口清单；5 文件关联仍未测。 |

HTTP 最终 `answer.md` 只有收尾确认，详细报告留在中间答复。下载响应 id4 没有做正文摘要，报告里的“成功拉取载荷”略强于证据，不影响 POST 外泄主结论。

DNS 证据记录里回显了较长的 PowerShell stager。任务要求不回显完整脚本；这是证据边界问题，不是 TXT 解码问题。

### 8.2 已决定不改

- Skill 不写解析字段路径或 JSON key。HTTP 复盘提出的字段级提示不采用。
- 不改 `details` 的正文返回，也不让 `preview_bytes` 改字段树。用户判断当前不影响分析效果。
- 不改 DNS TXT 摘要或解析器来自动解码。base64 在载荷内部，SPF 一类普通 TXT 不应被特殊处理。
- 查询摘要应能披露足够语义，这是通用方向，不是本轮已经开工的补丁。具体披露随工具输出演进，不回写 Skill。

### 8.3 工具修复历史

本节原有的候选修复、状态和“待复测”标记已经迁移到 [单一 TODO](./pcap-analysis-todo.md)，避免与真实运行记录形成两份状态。已经完成的相关能力包括 HTTP body 摘要、DCE/RPC 认证 PDU、NTLM Type3、SMB2 READ/svcctl 摘要、MAC/IP 端点查询和 CaptureOverview。

后续新增的 PCAP 工具或查询能力问题，只需在本节记录发生行为和运行证据，执行状态写入 TODO 中对应的 `PA-*` 任务。通用工具调用体验和输出安全策略不进入 PCAP TODO，除非用户明确重新纳入。

### 8.4 SMB/DCE-RPC 本轮结论

已核实事实：

- 三个 TCP 会话都是 midstream，捕获从连接中段开始，不能声称握手、挥手或整段会话完整。
- 源 `172.16.66.1` 到目标 `172.16.66.36`：445 端口为 SMB2，135 端口为 DCE/RPC Endpoint Mapper，49688 为 svcctl 动态端口。
- SMB2 第二轮 SESSION_SETUP 响应 `Status=0`；`ADMIN$` 和 `IPC$` Tree Connect 都成功；`PSEXESVC.exe` 创建并写入约 146KB；`PSEXESVC` 命名管道打开成功，后续有 IOCTL(PIPE_TRANSCEIVE) 和 WRITE/READ。
- 135 端口确认 EPM；49688 的 bind 携带标准 svcctl UUID `367abb81-9844-35f1-ad32-98f038001003` 和 NTLMSSP；agent 从原始 stream 中解析出 9 个 request 与 9 个 response，无 Fault、无 Reject。这是 agent 的原始字节观察，不是结构化消息索引的现成结果。

证据边界：

- “命名管道打开”是协议事实；“服务已创建并启动”是基于 PsExec 工作流的推断，报告应这样标注。
- “命令已经执行”仍未知。管道载荷不透明、RPC stub 被 NTLM 封装且 Win32 返回码不可见，不能从现有网络字节证明命令执行结果。

超时原因不能归成单点：

- 运行做了 28 轮、27 次成功业务调用，prompt 从约 3 万 token 涨到约 14.8 万 token，延迟明显上升。
- agent 在 todo-4 继续读取管道 IOCTL/READ 原始字节，试图证明本轮无法从网络侧证明的“命令是否执行”，没有及时按未知收口。
- DCE/RPC 结构化解析在认证 PDU 处失败。持久化库里只有 record 25、26、27 三条 dcerpc 消息，其中 26 报 `invalid bind result 3`，27 报 `48 unconsumed message bytes`；49688 的后续请求和响应没有成为结构化消息。agent 只能用 `bash` 解 PDU 头，还先用了错误偏移再修正。record 27 的 48 字节正好是认证 verifier 的 8 字节头部加 40 字节认证值；record 26 是 EPM BindAck 里一个 context 接受、另一个 context 被拒，被当成整条消息失败。

重复出现的工具协议摩擦：

- `require_tool` 数组再次被当成字面工具名：`cannot find [["load_pcapfile", "query_pcapdb"]]`。
- 批量调用漏 `tool_name` 被拒绝：`directly_call_tool_params_group[0]: tool_name is required`。
- 多次批量结果返回 `execution_succeeded=0; execution_unknown=N`，即使子调用已经返回结果，agent 仍要检查结果。

协议开发项：

DCE/RPC 认证 PDU 的解析边界、验收条件和测试证据已整理到 [PG-002](./pcap-analysis-parser-gaps.md#4-pg-002dcerpc-authenticated-pdu)。认证 PDU 的结构化解析已经实现，真实 agent 回归由 [单一 TODO 的 PA-002](./pcap-analysis-todo.md) 跟踪。本轮只保留测试结论，不在测试流程里穿插协议实现。Skill 只增加通用证据边界：管道或认证 stub 不透明时，把命令执行、进程结果和凭据成功范围标为未知；不写 svcctl 字段路径，也不内置服务方法名映射。

已落地：

- 认证 PDU 的结构化入口会切出 `8 + auth_length` 认证尾，保留 `Auth Type`、`Auth Level`、`Auth Pad Length` 和认证值，不把认证尾算进正文。
- 会话解析器不再因为认证尾存在而中断 TCP 方向；Auth3、Bind/BindAck、Request/Response 可以继续产生结构化消息。
- BindAck 的 `negotiate_ack`（result 3）按 context 记录为协商结果，部分 context 成功不会让整条消息失败。
- 认证级别 6 的 stub 标记为 opaque，OpNum、Call ID、Context ID 等明文头字段仍可查询。
- 回归覆盖：`common/bin-parser/dcerpc_auth_test.go`、`common/pcapx/pcaputil/protocol_session_dcerpc_review_test.go`、`common/pcapx/pcapdb/pcap_dcerpc_auth_test.go`。真实 `lateral-movement-psexec` 捕获的 DCE/RPC 消息从 3 条 malformed/decoded 混合结果扩展为完整调用链，pcapdb 查询可看到认证后的 9 组请求/响应。

复测：

- `smb-02` 使用同一 SMB 任务和同一 600 秒预算，仍然超时，没有 `answer.md`。
- 但 pcapdb 结果已从旧的 3 条 DCE/RPC 消息（其中 2 条 malformed）变为 25 条全部 decoded；Bind、BindAck、Auth3、svcctl 请求和响应都能直接查询，不再需要 `bash` 解 PDU 头。
- 新的主要摩擦变成证据边界：agent 继续手工解 NTLM Type3 和 SMB2 READ/命名管道原始字节，试图证明命令执行结果。认证 RPC stub 已被解析为 opaque，管道载荷也没有可证明的命令明文；这类情况应直接报告未知并收口。

最终复测：

- `smb-03` 于 2026-10-30 使用当前工作树构建的新二进制、同一自然语言任务、600 秒和 40 轮预算，516.317 秒正常结束，`engine_finished=true`。
- 共 20 次调用：1 次 `load_pcapfile`、18 次 `query_pcapdb`、1 次 `cybersecurity-risk`；0 次 shell，0 次失败。轨迹为 metadata → sessions/protocols → 12 次 details → 2 次 protocol_data → 风险落档。
- DCE/RPC 已经直接返回 svcctl Bind、ROpenSCManagerW、RCreateServiceWOW64W、ROpenServiceW、RStartServiceW、RQueryServiceStatus 和响应关联；SMB2 直接返回 PSEXESVC.exe 上传及服务管道读写摘要，没有手工解析 PDU/NTLM 头。
- 完整报告在 timeline item 530 交付，引用 dataset、record、会话和消息 ID；明确把加密 stub 的业务返回码、命令明文和退出码列为未知，没有从 PKT_PRIVACY 载荷猜测明文。`answer.md` 只保存后续 finish 摘要，属于运行器记录方式和已移出的通用输出格式问题，不改变本次 SMB 结构化分析通过。
- 独立复盘 `smb-03-review-01` 381.501 秒正常结束并产出 `feedback.md`，确认结论可信、三档区分准确、未使用 tshark/scapy/tcpdump/bash；与 `smb-02` 相比，行为改善明显，但两次运行同时改变了 commit、binary 和 Skill，不能把差异精确归因到单一改动。
- 复盘确认的剩余 PCAP 工具瓶颈是 12 次单条 `details`，继续由 `PA-004` 处理。批次结果 `execution_status=unknown` 属于通用工具执行框架问题，按 `N-008` 不进入 PCAP TODO。

### 8.5 RedLine 下载链本轮结论与未测

运行记录：

- `redline-01`：900 秒预算，40 轮，实际 285 秒，`engine_finished=true`，24 次内置工具调用，0 次 shell。
- `review-redline-01`：900 秒预算，实际 525 秒，独立复盘正常结束。完整分析报告在 `events.jsonl` 的 seq 4692，`answer.md` 只有 310 字节收尾确认。

已经好用：

- agent 自行加载 Skill，只用内置工具覆盖会话、协议、详情、正文摘要和流数据，没有依赖外部 tcpdump/tshark。
- HTTP 跳转、TLS SNI、捕获范围、自定义端口会话和双向字节量都有可追溯证据；TLS 载荷内容被正确标为不可验证。
- 报告主体区分了事实、推断和未知，时间线基本完整，没有执行捕获内容或访问样本基础设施。

值得改进：

- 报告的事实/IOC 表述把硬编码 IP 和疑似外泄会话直接写成 C2 或“已确认恶意”，与同文的“无法确认托管归属”矛盾，也违反样本任务中不得写成已确认 C2 的边界。
- 最终报告完整回显了疑似授权标识和凭据枚举列表。即使它可能只是 Bot ID，也应按敏感证据处理，只保留必要的脱敏片段。
- 542KB 出站数据只能证明会话传输量；内容未解码时不能直接写成“凭据/文件窃取结果”，只能写成待验证的外传候选。

暂不确定：

- 独立复盘认为报告“质量高、没有协议误判”，但开发核查在 C2 置信度和敏感信息边界上不同意。需要同任务复测或第二份独立复核，不能把单次评价当成稳定结论。
- 本轮 `stream_data` 都正确把流号填在 `record_id`，没有出现参数误用；双字段命名的易用性问题只算潜在风险，不是本次已证实的绕路。
- 未识别流的可读文本视图是高价值候选，但本轮尚无直接对照运行证明它能让结论更稳定。

WCF `net.tcp` 的协议识别和 framing 缺口属于独立的底层解析工作，已记录为 [PG-001](./pcap-analysis-parser-gaps.md#3-pg-001wcf-over-nettcp)，本轮不展开实现。

| 目标 | 为什么还没测 | 开测前要处理的事 |
| --- | --- | --- |
| 主机与用户识别的其余 4 个文件及跨文件归一 | `...3-of-5.pcap` 已由 `host-user-windows-01` 单独测试。DHCP/NBNS、Android 和 iOS 文件尚未跑。 | 当前 `run.py` 仍一次只接受一个捕获。跨文件的同一 IP、主机名和用户冲突不能由本轮冒充已验证。 |
| HTTPS keylog | `load_pcapfile` / `query_pcapdb` 目前不接受 keylog。 | 先让导入链路明确报告不支持，或实现 keylog 后再做有、无 keylog 的对比。不要用 `pcap-inspect` 的结果冒充 agent 持久化链路。 |

`public-corpus-index/` 仍只是语料接入建议，不是当前测试目标。

### 8.6 主机与用户识别本轮结论

运行记录：

- `host-user-windows-01`：样本 `Wireshark-tutorial-identifying-hosts-and-users-3-of-5.pcap`，900 秒预算，40 轮，实际 606 秒，`engine_finished=true`。固定使用 `redline-01/yak`，没有混入并行中的 DCE/RPC 工作树。
- `review-host-user-windows-01`：574 秒正常结束。完整报告在 `events.jsonl` seq 7218；最终 `answer.md` 有 1430 字节收尾，但仍把详细表留在中间答复。

已经好用：

- 单文件内形成了主机、域、机器账户、交互用户和服务账户的候选表，并区分了事实、推断、未知和弱线索。
- 导入后的持久化库包含 Kerberos、LDAP、SMB2、DHCP、DNS 和 HTTP 的结构化消息，说明这些协议并非整体不可解析。

值得改进：

- 18 次调用里只有 `load_pcapfile` 1 次、`query_pcapdb` 2 次，其余 15 次是 shell。agent 在 `sessions` 第一页后没有尝试 `protocols`、`fields` 或 `details`，而是重写了多个全量 Python 解析器。
- Kerberos 摘要大多只有协议名；主体名称埋在很深的 DER 字段树里。LDAP 摘要有操作类型，但用户值同样不在摘要中。这会让 agent 看不出记录是否含身份。
- UDP/137 的 NBNS 被分成 DNS，名称保持 first-level 编码；UDP/389 的 CLDAP 有 34 条 `unrecognized`。这两项已进入协议缺口清单，不在本轮实现。
- 写 Python 的直接原因已从 `streams.json` 的 thought stream 核实：它原本计划查 `protocols`；读完 `sessions` 第一页后认为 498 条会话要反复分页，且返回中没有 MAC。它检查到 tshark、scapy、dpkt 都不可用，任务又明确允许编写分析脚本，因此选择一次全量 Python 扫描。它提到过 `fields`/`packet_data`，但没有实际调用。

暂不确定：

- 15 次 shell 不能只归成一个根因：缺少 MAC/身份汇总入口和分页成本是真实工具问题，未尝试 `protocols`/`fields` 是 agent 的发现与成本判断问题，NBNS/CLDAP 则是协议缺口。
- 交互用户、机器账户和域的具体字符串来自 agent 自写解析，本轮没有用内置 `details` 逐项复核，因此这些值仍是候选，不是已由解析器稳定证明的实体；后来加入的端点查询只解决 MAC/IP 关联，实体字段复核由 TODO `PA-019` 跟踪。

### 8.7 MAC/IP 端点索引实现记录

实现范围：

- pcapdb schema 升到 5；新导入的 packet 行保存 `src_mac`、`dst_mac` 和 `vlan_stack`，同时保留原始帧 BLOB。
- `QueryMACSummariesPage` 按 `section/interface/link_type/vlan_stack/src_mac` 聚合，返回源 IP 数、包数、首末时间和首末包号。
- `QueryEndpointBindingsPage` 按上述捕获域加 `src_ip` 展开，可用 `source_mac`、`source_ip` 和时间范围筛选。
- `query_pcapdb` 增加 `macs` 与 `endpoint_bindings`；Skill 增加通用入口，不写具体协议字段路径。
- 目标地址不参与“归属映射”：跨路由时目的 MAC 经常只是下一跳网关。

已做验证：

- `go test ./common/pcapx/pcapdb -count=1` 通过，覆盖 MAC/VLAN 抽取、一 MAC 多 IP、VLAN 隔离、时间过滤、MAC/IP 筛选和两类分页。
- pcapdb Yak mustpass 脚本通过，验证 Page API 的字段、计数和游标。
- 对 `host-user-windows-01` 的 12,329 包样本用新 schema 重新导入，得到 3 个源 MAC：网关型 MAC 对应 95 个源 IP、6,158 包；域控对应 2 个源 IP、977 包；工作站对应 5 个源 IP、5,179 包。结果带首末时间和包号。

端点覆盖边界：

- 剩余 15 个包没有同时具备源 MAC 和源 IP，端点查询不会强行制造对应关系；需要时可继续查 packets、ARP 或原始包。

真实 agent 复测：

- `host-user-windows-02` 使用同一自然语言任务和第 3 个样本，900 秒预算内 877 秒完成，`engine_finished=true`。
- 45 次调用为 1 次 `ls/file`、1 次 `load_pcapfile`、43 次 `query_pcapdb`，没有 Python 解析脚本。
- 第 7 次调用主动使用 `macs`，得到 3 个源 MAC；第 43/45 次调用使用 `endpoint_bindings` 展开工作站和域控。网关判断采用“单 MAC 多源 IP”，没有把 MAC 当主机身份的权威依据。
- 最终 `answer.md` 为 6909 字节，完整报告已写入最终回答，不再只留“报告见上一轮”的收尾。
- 独立复盘确认“MAC/IP 底层查询已使用，未手写 pcap 解析”；同时指出协议直方图和批量 details 仍是下一次工具缺口。

需要保留的边界：

- 这次没有证明整体更快：上一轮 606 秒，本轮 877 秒，查询次数从 18 增至 45。改善的是确定性底层工作不再由模型手工实现，不是单次运行的总耗时。
- agent 仍把 NBNS 说成“不存在”，因为 UDP/137 仍被归到 DNS；这是已记录的 PG-003，不应由 MAC/IP 查询的结果掩盖。

### 8.8 捕获概况实现记录

实现范围：

- `CaptureOverview()` 从已索引协议消息执行精确 `GROUP BY`，返回协议、状态和传输层计数，不返回消息行。
- `query_pcapdb` 的 `metadata` 合并概况，并区分 `protocol_message_count` 与 `protocol_kind_count`；保留旧 `protocol_count` 兼容。
- 返回首末时间、link types、Section/Interface 数量，以及三类聚合列表各自的截断标记。
- 协议列表最多 256 项，状态/传输层各最多 32 项；`ProtocolKindCount` 始终为精确值。
- `Metadata()` 本身不做聚合，避免健康检查和普通登记查询承担额外扫描。

已做验证：

- `go test ./common/pcapx/pcapdb -count=1`：通过。
- PCAP 内置工具集成测试和 pcapdb Yak mustpass：通过。
- `host-user-windows-01` 样本回放返回 17 个协议类别、5,516 条协议消息和 1,924 字节概况 JSON；协议/状态/传输层三组计数都精确闭合到 5,516。
- 同一概况为：TLS 3,101、QUIC 786、DNS 731、IP 415、HTTP 94、未解析 71 等；状态为 decoded 4,233、context-required 770、incomplete 419、unrecognized 71、malformed 23。

真实 agent 复测：

- 第一次 `protocol-overview-01` 使用 300 秒预算，在模型服务排队阶段没有发出任何查询就超时，不能作为功能结果。
- `protocol-overview-02` 使用 600 秒预算，实际 130 秒完成；只有 `load_pcapfile + query_pcapdb(metadata)` 两次业务调用，没有任何 `protocols` 消息翻页。
- 回答精确返回 17 个协议类别、5,516 条协议消息、5 种解析状态和 3 种传输层；三组计数都自行校验为 5,516，并明确 `summary_truncated=false`。
- 原先主机识别运行中的 20 次协议消息探查，在这个专门任务里被压缩为一次 metadata 调用。
- 独立复盘第一次 300 秒超时；第二次 600 秒预算实际 220 秒完成，确认概况使用和计数正确。

复盘发现的边界：

- Agent 对 `context-required` 和“不支持”的语义解释偏离解析器定义。SKILL 和 `query_pcapdb` 说明现已补上 `decoded/incomplete/malformed/unrecognized/context-required` 的权威语义，并明确不存在独立的 `unsupported` 消息状态。
- metadata 当前分别聚合协议和状态，没有 `protocol × status` 交叉计数；若要知道“Kerberos 中有多少 context-required”，仍需后续查询。交叉视图由 TODO `PA-003` 跟踪。
- 概况只提供协议类别和数量，不提供具体消息；确认存在某类身份线索后仍要使用 `protocols/fields/details`。

### 8.9 批量详情实现与真实 agent 验证

实现范围：

- 新增 `ProtocolDetailsBatchPage` 和 `query_pcapdb(query_type=details_batch)`，最多接受 50 个唯一正整数消息 ID。
- 单次只读快照和一次有界 `IN` 查询取代逐消息独立详情事务；输出顺序与输入完全一致。
- 批项携带 protocol、transport、status、completeness、session/stream/transaction/response_to 关联。
- 缺失记录写为该 ID 的 `not_found`；单项无法装入剩余预算时写为该 ID 的 `result_too_large`，同页其他小项继续返回。
- 重复、零 ID、空数组、超过 50 项、混用 record_id 和 details_batch 不接受 cursor 都在入口拒绝。

确定性验证：

- `go test ./common/pcapx/pcapdb -count=1`：通过，覆盖顺序、缺失、预算降级、重复和取消传播。
- `TestPCAPAnalysisTools`：通过，覆盖 Yak 数组参数、成功批量、缺失项和无效参数。
- `TestMustPass/pcapdb.yak`：通过。

真实 agent 分布验证：

- 固定输入：同一捕获 SHA256 `7ce9a37aa6d8f0121d23f9370d11f17d4de7111cf19629d92ff630abd74064af`、同一任务文件、同一 profile/模型路由、900 秒/40 轮、串行执行。
- 基线组使用旧二进制 SHA256 `b3a64ef7dac7cbbcfd292b1a121adeef9269d125c402ac73d9ff0fbb8719cc41` 新跑 3 次；新实现组共 5 次，其中固定新二进制 SHA256 `115d6b12ba961d564c1a5b1615680a74b8c2029816267b1d64c64a0da811128a` 跑 4 次。
- 新实现 5 次全部使用 `details_batch`，单条 details 分别为 5、3、0、3、2；旧二进制成功样本的单条 details 为 17（历史）、15、6、11，两组范围不重叠。

| 组别 | 完成率 | 全样本耗时中位数 | 成功样本耗时中位数 | 单条 details 中位数 | details_batch 中位数 | `query_pcapdb` 中位数 | 业务工具中位数 | AI 调用中位数 | React 中位数 | tool-call-reason 中位数 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 旧二进制固定 3 次 | 3/3 | 785.0s | 785.0s | 11 | 0 | 32 | 39 | 36 | 29 | 7 |
| 新二进制固定 4 次 | 3/4 | 581.9s | 493.7s | 2.5 | 5 | 18.5 | 25.5 | 53.5 | 32.5 | 11.5 |
| 新实现全部 5 次 | 3/5 | 670.2s | 493.7s | 3 | 5 | 19 | 26 | 54 | 36 | 22 |

结果解释：

- 工具级改善稳定：旧成功样本至少 6 次单条 details，新样本最多 5 次；说明 agent 确实用批量读取替代了多记录逐条读取。
- 端到端改善不稳定：新实现全部样本只有 3/5 完成；固定新二进制为 3/4，旧二进制为 3/3。样本太小，不能宣称完成率提升，也不能用“成功样本中位数 493.7 秒”掩盖两次 900 秒超时。
- 所有样本的工具执行总耗时远小于总运行时间：旧固定组中位数约 11.2 秒，新固定组约 17.7 秒。批量读取即使增加单次返回，也不是超时主因。
- 新实现的模型编排没有同步减少：固定组 AI 调用中位数 36→53.5，React 29→32.5，tool-call-reason 7→11.5；新实现全部样本的 tool-call-reason 中位数为 22。输入 token 中位数在固定组约为 343 万→346 万，说明差异主要来自更多顺序决策/验证轮次，而非单纯上下文膨胀。
- 批量单次响应约 7–12 KiB，单条 details 约 1.3–5.4 KiB；首轮 5 个 batch 共选 26 个记录槽位，一次带入多份 Fields/Session，可能改变 agent 的后续阅读和验证策略，但工具执行本身仍只占秒级。
- 新实现内还有一个稳定分界：3 次成功运行分别批量取回 15、15、24 条记录；2 次失败运行分别取回 26、33 条记录。也就是说 batch 把“更多次小查询”变成了“更少次大结果”，agent 会把本不需要的 Fields/Session 一次装进上下文，随后花更多轮次验证和消化。
- 外层 AI 工具结果存在独立的 16,384-token 上限。两次失败均命中该边界：`details-batch-01` 的 8-ID 批次原始结果为 84,352 字节，被中间裁掉 1,671 tokens；`stability-03` 的 9-ID 批次也被裁掉 1,353 tokens，并出现 raw truncation marker。3 次成功运行没有外层截断。pcapdb Page 自身仍显示 `truncated=false`，说明两层预算语义不一致。
- `details-batch-01` 在 900 秒时已关闭 7 个 TODO，但没来得及产生最终回答；`stability-03` 只完成 5 个 TODO，仍有 D/E/report 三项未结，并在同一轮发生 10 次 verification 调用。两者分别是“收尾超时”和“验证/计划循环未收敛”，都不是 pcapdb 查询等待。
- 基线也会选择不同替代路径：固定基线第 2 次只用 6 次单条 details，但增加了 5 次 endpoint_bindings、packet_data 等查询。因此评价不能只看单一 details 数字，必须同时看 query/tool 总数和成功率。
- 因果边界：旧/新二进制同时包含协议索引规模、metadata、Skill、query schema 和 AI runtime 变化，因此成功率差异不能全部归因于 batch。当前已经定位到的直接工程缺陷是 batch 的 Page 预算未考虑外层 16K-token 硬上限；该截断会把完整 JSON 变成非合法结构，随后触发 artifact 读取、重查或额外 verification。
- 修复：`ProtocolDetailsBatchPage` 增加 10K-token Page 硬预算，并保留 6K-token 给外层 tool envelope；全部请求 ID 和关联元数据始终保留，预算不足时按最大的 Fields/Session 项逐项降级为结构化 `result_too_large`。跨层测试直接引用 `aicommon.ToolResultTokenLimit`，pretty JSON 也必须位于外层限制减去 1K reserve 之下。
- 修复后真实回归 `/tmp/pcap-details-token-safe-20261103`：当前工作树构建，374920 ms 完成，最终回答 1443B；13 次 `details_batch` 均没有外层截断 marker。仍有 1 次单条 `details(record_id=587)` 命中 16K 保护，说明通用保护机制未被绕过；这属于单条详情边界的后续独立问题，不由 batch 修复冒充解决。

首轮回归原因：

- 工具本身不是瓶颈。多轮业务工具总耗时中位数约 11–18 秒，而模型 React/verification 占用主要 wall time。
- 首轮模型编排调用共 81 次，其中 React 49 次、tool-call-reason 26 次；固定旧基线中位数为 36 次，其中 React 29 次、tool-call-reason 7 次。
- 首轮 React 输入约 574.7 万 token，context 末尾达到 238026；固定新组输入 token 中位数与旧组接近，但仍出现 4 次运行耗时跨度 477.6–900.0 秒，说明模型决策路径和验证次数的方差主导结果。
- 成功轮更早按任务 A-E 组织批量读取；失败轮出现过度探索（DHCPv6/NBT-dg/更多 Kerberos details/HTTP 第二页）或验证循环（10 次 verification 后仍未完成 D/E/report）。
- 因此有效结论是：`details_batch` 稳定减少详情调用，但当前实现缺少与 AI 工具输出上限的闭环约束，可能把完整 Page 截成不可直接解析的中间片段。下一步必须先让批量结果在最终工具边界内保持结构化，再讨论 agent 编排优化。

结论边界：

- 多次运行证明批量接口被 agent 稳定采用，旧成功样本单条 details 为 6–17 次，新样本为 0–5 次；外层 16K-token 截断缺陷已通过 batch 内部结构化预算修复，PA-004 完成。
- 新实现组仍有 2/5 次 900 秒未完成，不能把功能级调用改善写成端到端稳定性改善；整体实体正确性仍由 `PA-019` 跟踪。

### 8.10 当前收口入口

不再在本文维护“下一轮做什么”。选择下一批任务、更新优先级、记录完成条件和真实 agent 复测结果，统一进入 [PCAP 分析 Agent 单一优化 TODO](./pcap-analysis-todo.md)。

截至本节最后一次更新，`smb-03` 主运行和独立复盘、`details_batch` 多轮分布测试与 16K-token 边界修复均已完成。批量 Page 现在会在进入通用工具层前完成 token-aware 收敛，`ToolResultTokenLimit` 保持不变并继续作为最终安全边界。单条 details 仍可能触发外层截断，后续若持续阻塞分析，应按同一原则单独处理。
