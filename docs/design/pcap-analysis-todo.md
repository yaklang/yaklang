# PCAP 分析 Agent 单一优化 TODO

> 状态：当前 PCAP AI 分析分支的唯一执行清单
> 最后更新：2026-11-03
> 当前分支：`rookie/enhance/ai-traffic-analysis`（已 rebase 到 `origin/main`）
> 当前范围：PCAP AI 分析流程、通用查询工具、测试脚手架和真实 agent 验证
> 工作流与历史证据：[PCAP 分析 Agent 的 TDD 开发工作流](./pcap-analysis-agent-tdd.md)
> 协议解析独立清单：[PCAP 协议解析独立分支 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md)

本文只维护当前 AI 流量分析目标的执行队列，不包含应用协议 parser 的开发。任何 WCF、CLDAP、NBNS、SMB、DCE/RPC、LDAP、Kerberos 等协议识别、framing、字段解码或协议专属摘要实现，都必须在独立分支按 [协议解析 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md) 推进。真实 agent 的运行过程、长轨迹和问题分析保留在 TDD 文档；企业平台级的长期需求仍以 [企业流量分析需求](./enterprise-traffic-analysis-requirements.md) 为产品背景。

## 1. 当前快照

- HEAD：`d705e2a1a refactor(syntaxflow): drop include-cycle gate entirely`；本地分支与 `origin/main` 指向同一提交。
- 当前分支依赖的通用 PCAP 能力已进入 main：`0de514f86`（退役旧工具）和 `8250cf3ff` + `d65004006`（CaptureOverview、MAC/VLAN 和端点摘要）。
- `238faf87f` 恢复了企业流量语料，5 个 host-user 文件现在都可直接回放，供当前分支做工具和 agent 验证。
- 应用协议 parser 和协议专属字段/摘要属于独立分支，不在当前工作区目标内；已完成和待办状态统一迁移到 `../work3` 工作树的 [协议解析 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md)。
- 工作区没有已跟踪的未提交改动；`pcap_lab` 脚手架及三份 PCAP 设计文档仍按约定保持未跟踪。
- 最近确定性验证（2026-11-03）：`details_batch` 已增加 10K-token Page 硬预算和 6K-token 外层包装余量，并通过引用 `aicommon.ToolResultTokenLimit` 的跨层测试。修复后 host-user 真实运行 374.9 秒完成，13 次批量详情均未发生外层中间截断，最终回答 1443 字节。详细边界见 TDD 8.9。
- 最近成功的真实 agent 复测：`host-user-windows-02`、`protocol-overview-02`、`smb-03` 及其独立复盘。`smb-02` 只作为失败基线保留。
- 旧分支上的真实 agent 结论仍可复用，但代码和提交证据全部以当前 main 为准；不能继续引用旧 feature 分支的提交号。

## 2. 维护规则

1. 本文是当前 `rookie/enhance/ai-traffic-analysis` 分支中 PCAP Agent、`pcap-analysis` Skill、通用 pcapdb 查询能力和真实 agent 测试脚手架的**唯一活动 TODO 清单**。
2. 应用协议 parser、协议专属字段和协议专属摘要不得进入本文或当前分支；这些内容统一由 `../work3` 工作树的 [PCAP 协议解析独立分支 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md) 管理。
3. 当前分支发现协议缺口时，只记录复现证据并移交协议分支，不顺手修改 parser，也不把 parser 修复作为当前任务完成条件。
4. 每个任务必须写明优先级、状态和完成条件；状态为 `NEXT` 时必须同时写明本轮下一步。没有验收条件的想法先留在“待确认”，不要伪装成已排期任务。
5. 状态只使用以下值：
   - `NEXT`：已选入当前批次，最多同时保留 3 项。
   - `DOING`：当前正在执行，必须记录执行人、开始时间和运行目录。
   - `READY`：边界清楚，可随时进入下一批。
   - `VERIFY`：实现或脚手架已经存在，等待确定性测试或真实 agent 复测。
   - `BLOCKED`：缺少明确前置能力、用户决策或外部条件。
   - `DONE`：完成条件全部满足，保留在当前上下文附近一段时间后归档。
   - `REJECTED`：明确决定不做，并记录原因，避免重复提出。
6. 修改实现、测试或相关文档时，必须在同一个提交或同一轮工作中同步更新对应任务；不能只在聊天、复盘或提交说明里更新状态。
7. 关闭任务时必须记录：提交或运行 ID、验证命令、真实 agent 运行目录、结论和剩余边界。`finished=true` 或“没有报错”不能单独作为完成证据。
8. 新一轮真实 agent 测试前，先把 1～3 项设为 `NEXT`；测试完成后更新运行证据，再决定继续、拆分、阻塞或关闭。
9. 已关闭任务保留原 ID，不重新编号。跨文档迁移同样保留原 ID，避免历史轨迹失效。
10. Skill 仍只描述调查流程和证据边界。任何解析字段、JSON key 或必须与解析器一起修改的结构，都不能作为 Skill TODO。

## 3. 当前执行队列

| ID | 优先级 | 状态 | 任务 | 完成条件 |
| --- | --- | --- | --- | --- |
| PA-003 | P1 | READY | 为捕获概况增加 `protocol × status` 交叉统计 | metadata 或独立概况接口能直接返回各协议的解析状态计数；计数闭合到 `protocol_message_count`；有截断标记和确定性测试 |
| PA-004 | P1 | DOING | 多轮验证 16K-token 边界修复和端到端稳定性 | 固定新二进制至少运行 5 次，统计成功率、耗时、batch 截断、小批次数量、AI/React/token，并明确是否稳定优于旧组 |
| PA-009 | P1 | READY | 增加未识别 TCP 流的可读取证能力 | 提供有界 printable text 预览和流内子串定位，同时保留原始块和 offset；RedLine 任务能直接定位 WCF 可读内容，不要求 Skill 写固定字段 |
| PA-015 | P1 | READY | 支持一份真实 agent 任务关联多个捕获文件 | 企业语料已恢复，但 `pcap_lab/run.py` 仍只定义单个 `--capture`；需支持稳定顺序导入多个 capture、逐文件哈希/错误，并复测 host-user 其余 4 个文件及跨文件实体归一 |
| PA-016 | P2 | READY | 在持久化导入链路中加入 TLS keylog 能力或明确不可用语义 | `pcaputil.ParseTLSKeyLog` 和 `WithTLSSecrets` 已存在，但 `pcapdb.ImportOption` 及 `load_pcapfile` 尚未接入；需让 `load_pcapfile/query_pcapdb` 可查询能力边界，并完成 HTTPS 可见性对比，不能用 `pcap-inspect` 冒充持久化链路 |
| PA-018 | P2 | READY | 重跑 DNS TXT 隧道任务的独立复盘 | 复盘进程正常结束并产出 `feedback.md`；对已有 `dns-01` 的工具选择、证据保存和最终回答给出具体事件依据；超时不再作为未结论文本留在活动清单 |
| PA-019 | P1 | READY | 用内置查询复核主机与用户候选实体 | 对 `host-user-windows` 样本中的 IP、MAC、主机名、域和用户候选逐项关联 record ID、来源协议、时间和置信度；不能只沿用自写 Python 解析得到的字符串 |

## 4. 任务执行说明

### PA-003：交叉统计的语义

单独协议计数和状态计数不能回答“Kerberos 中有多少 `context-required`”。交叉视图必须保持聚合结果有界，不能退化成让 agent 拉取所有协议消息。若采用截断，必须同时返回精确总数和截断标记。

### PA-009：原始流优先

可打印文本视图是辅助索引，不能替代原始字节。所有文本片段必须带 chunk、offset 和长度，并明确它是有界预览；跨 chunk 搜索必须保留边界状态，不能逐块 search 后声称全流没有命中。

### 协议 parser 边界

`PA-010`、`PA-012`、`PA-013`、`PA-014`、`PA-101`、`PA-102`、`PA-103` 和 `PA-107` 都属于应用协议专用能力，已迁移到 `../work3` 工作树的 [PCAP 协议解析独立分支 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md)。当前分支可以消费它们形成的稳定查询结果，但不得修改其 framing、字段、消息摘要或协议分发。

如果当前分支的真实 agent 测试暴露 parser 缺口，只做三件事：

1. 保留捕获、运行目录、record ID、预期字段和当前状态。
2. 在独立 TODO 更新复现条件和验收。
3. 继续使用明确的 `unrecognized`、`context-required` 或 `incomplete` 边界完成当前分析，不能猜测协议字段。

### PA-015 与 PA-016：已解除阻塞的实现项

这两项此前依赖语料或底层 TLS 能力，现已具备实现前提，因此从 `BLOCKED` 改为 `READY`。当前 `pcap_lab` 仍是单捕获运行器；`pcap-inspect` 已能使用 keylog，但持久化 pcapdb 导入还没有 TLS option。未完成前，不把“未测试”写成“没有能力”，也不能用 `pcap-inspect` 冒充 `load_pcapfile/query_pcapdb`。多文件任务还必须记录文件间时间、哈希、导入结果和关联依据。

### PA-018 与 PA-019：补足已有运行的验证闭环

DNS 主分析已经可用，但独立复盘在 600 秒预算内没有产出结论，不能把“主 agent 已完成”当成整个 TDD 循环完成。主机与用户识别也有类似问题：`host-user-windows-01` 的身份表主要来自自写 Python，后续端点复测证明了 MAC/IP 查询可用，但最终实体字段仍需逐项证明能由内置 `protocols/fields/details` 追溯。未解析协议造成的字段缺失移交协议分支，不在本轮补 parser。

这两项只补验证和证据链，不重新定义已经明确不做的 DNS TXT 解码，也不要求 Skill 写入身份字段路径。

## 5. 最近完成

| ID | 状态 | 已落地内容 | 证据与剩余边界 |
| --- | --- | --- | --- |
| PA-001 | DONE | CaptureOverview、MAC/VLAN 和 endpoint summaries 的配套测试、mustpass 与文档已提交 | main `8250cf3ff`、`d65004006`；pcapdb 和工具集成测试通过 |
| PA-002 | DONE | 青隼完成 `smb-03` 真实 agent 回归和独立复盘 | `smb-03` 516 秒正常结束，20 次调用、0 次 shell；`smb-03-review-01` 381 秒产出 `feedback.md`，确认三档结论和无手工解包；12 次单条 details 已由 PA-004 的批量接口收口 |
| PA-011 | DONE | 清理 RUNBOOK、HTTP 任务和需求文档中的退役 PCAP 工具兼容表述 | 文档统一为 `load_pcapfile → query_pcapdb → manage_pcapdb`；废弃策略和历史 fixture 保留原样 |
| PA-104 | DONE | MAC/VLAN 包索引、`macs`、`endpoint_bindings` 和源地址共现边界 | main `8250cf3ff`、`d65004006`；`host-user-windows-02` 已使用新查询且不再写 Python |
| PA-105 | DONE | CaptureOverview 和 metadata 协议/状态/传输层直方图 | main `8250cf3ff`；`protocol-overview-02` 130 秒完成，交叉统计仍另立 PA-003 |
| PA-106 | DONE | `pcap_lab` 真实运行、轨迹索引和独立复盘脚手架 | Python 单元测试通过；多文件支持另立 PA-015。按此前约定暂不提交 |
| PA-017 | DONE | 旧 feature 分支的 rebase/发布收尾已完成，当前开发基线切回 main | 当前分支 `rookie/enhance/ai-traffic-analysis` 与 `origin/main` 均为 `d705e2a1a` |

## 6. 明确不做或不再重复提出

| ID | 决定 | 原因 |
| --- | --- | --- |
| N-001 | Skill 不写协议字段路径、JSON key 或具体服务方法映射 | 会与解析实现强耦合，字段变化会迫使 Skill 同步修改 |
| N-002 | 不为 DNS TXT 类型增加自动 base64 解码 | 需要解码的是某些载荷内部语义；普通 TXT、SPF 等不应被特殊处理 |
| N-003 | 不修改 `details` 返回正文的既定语义，也不让 `preview_bytes` 改变字段树 | 当前行为不构成已证实的分析阻塞；分类和正文摘要应由专用能力解决 |
| N-004 | 真实 agent 测试不限制为固定工具白名单或固定调用顺序 | 要观察 agent 是否主动发现并优先使用系统工具；命令行仍是合法能力 |
| N-005 | 不因为出现硬编码 IP、可疑上传或 TLS 会话就把 C2 写成已确认 | 网络证据只能支持候选、关联和未知边界；确认需要更多独立证据 |
| N-006 | 不把未识别协议直接等价为恶意协议或解析器崩溃 | `unrecognized` 只表示当前应用层能力未完成分类和消息化 |
| N-007 | 不把 `public-corpus-index/` 当作当前真实 agent 测试目标 | 它只是后续语料接入建议；当前先完成已有企业样本和脚手架闭环 |
| N-008 | 不把 `require_tool`、`directly_call_tool` 等通用工具调用体验纳入 PCAP TODO | 它们属于 AI 运行时框架能力，不是 PCAP 工具或 Skill 自身的问题；运行中的现象继续保留在 TDD 历史记录 |
| N-009 | 不把最终回答格式和通用敏感输出策略纳入 PCAP TODO | 它们属于 agent 通用输出与安全策略；PCAP 测试可以观察和汇报，但不在本清单单独排期 |

## 7. 文档职责

| 文档 | 职责 | 不应承担 |
| --- | --- | --- |
| 本文 | 当前 TODO、优先级、状态、验收、最近完成 | 长篇轨迹和协议格式分析 |
| [`../work3` 协议解析独立 TODO](../../../work3/docs/design/pcap-protocol-parsing-todo.md) | 应用协议识别、framing、字段解码、协议专属摘要及其验收 | 当前 AI 工具/agent 工作流排期 |
| [PCAP 分析 Agent 的 TDD 开发工作流](./pcap-analysis-agent-tdd.md) | 工作流、真实运行记录、复盘结论 | 再维护一份活动优先级 |
| [PCAP 真实测试中的协议解析缺口](./pcap-analysis-parser-gaps.md) | 协议缺口证据、技术边界、回归设计 | 判断全局优先级和排期 |
| [pcapdb README](../../common/pcapx/pcapdb/README.md) | pcapdb API、查询语义和使用约束 | 记录未决产品 TODO |
| [PCAP agent 开发脚手架](../../common/ai/aismoking/live/pcap_lab/README.md) | 如何运行、产物和安全边界 | 记录工具优化 backlog |
| [企业流量分析需求](./enterprise-traffic-analysis-requirements.md) | 产品目标和长期能力蓝图 | 代替当前测试驱动迭代的任务清单 |

## 8. 下一次维护的最小动作

每次开始或结束一轮工作，至少更新以下位置：

1. 文首“最后更新”。
2. “当前快照”中的 HEAD、工作区和最近验证。
3. 对应任务的 `状态`、`完成条件` 或证据链接。
4. 新增真实 agent 运行的 run ID、结论和是否改变优先级。
5. 已完成项从活动表移到“最近完成”，并保留测试命令或提交。

维护时优先保持任务少而准确。发现新问题时先判断它是否属于本文范围；若只是产品愿景或尚未形成可验证边界，保留在需求文档或“待确认”，不要为了数量把活动队列扩张成无法执行的清单。
