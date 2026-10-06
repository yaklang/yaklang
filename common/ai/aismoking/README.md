# AI 冒烟测试

`memory_search.yak` 验证 `aim.InvokeReAct` 两种协议下的记忆工具调用、当前 memory 集默认值、隔离、过期/删除过滤，以及 Timeline 冻结保存。用 `AISMOKING_CASE=memory_search` 从总入口运行，只供本地开发，不进 CI。

库入口：`aimemory.SearchMemory(query, aimemory.memoryNamespace(id), aimemory.memoryTokenLimit(1500), aimemory.memorySearchMode("hybrid"))`。仅一个 namespace/id，指向当前项目数据库里的 `ai-memory-<id>` 集合；不引入 workspace。AI 工具中默认绑定当前运行时实际使用的 memory 集，独立脚本默认 `default`。不假设 persistent session ID 就是 memory 集 ID。

库复用 RAG BM25/向量查询及已有 memory 表接口，返回现有 `AIMemoryEntity` 行，限量、去重、过滤过期/删除项并限制正文 token；输出格式在 `search_memory.yak` 中维护。`bm25` 还补充正文/标签关键词匹配，缺少索引可回退；`hybrid` 向量不可用时保留关键词结果并记录英文诊断，`vector` 明确报错。无匹配返回空数组，不创建 memory 集、不初始化 triage。`smart_qa` 仍是已注册的专注模式，其 memory action 转发至同一脚本。

自动 injection 只在意图识别结果落地时复用现有 `SearchMemoryWithoutAI` 检索一次，最多 5 条、1200 tokens；没有意图识别不自动检索。同一意图的空结果也不重搜，后续工具步骤和记忆写入不刷新快照。显式搜索仅进入普通工具 Timeline 及冻结块，不修改 injection。自动沉淀仅来自 Timeline 压缩产生的 memory entities；执行中沿用原阈值，完整用户任务正常结束且有新增业务内容时额外收尾。审核等待、idle wait、阶段切换不触发抽取。取消/断连只记下待处理来源，恢复后补处理；手动创建记忆及管理接口保留。

所有 AI mock、冒烟和模型观测脚本集中在这里。业务流程、模型夹具、审核回复和断言写在 Yak 中，不需要 Go 注入变量或启动 Go 业务运行器。Go 单元与集成测试继续验证内部状态机、并发、恢复、权限及 VM 绑定。

从仓库根目录执行：

```text
yak common/ai/aismoking/run.yak
```

总入口逐个以当前 Yak 可执行文件启动独立进程，相当于 `yak xxx.yak`。每例拥有独立的 `YAKIT_HOME`、材料目录、日志和采样，避免会话污染及 SQLite 锁冲突。总入口用 `yak tiered-ai-config --enable --config-file` 写入测试专用 profile：全局模型指向关闭的 loopback 端口，并禁用 fallback，使后台价值评估快速失败，不访问外部模型。不能用空模型列表或已过时的 `enabled=false` 实现隔离，启动会自动补齐默认模型；这些设置只影响临时 profile，不改变开发者自己的配置。业务模型通过正常工作的本地 mock provider，不需要 API key。模型回答可控；协议投影、HTTP/SSE、循环、工具、审核、调度、Timeline 和 artifacts 使用真实实现。

| 脚本 | 覆盖内容 |
| --- | --- |
| [mainloop.yak](mainloop.yak) | 默认 `aim.InvokeReAct` × 两种协议；require 只加载 Schema、下一轮显式执行、实际读文件、session Evidence、冻结提升和纯动态区不重复展示用户输入 |
| [retry.yak](retry.yak) | `aim.InvokeReAct`、`liteforge.Execute`、`ai.FunctionCall` × 两种协议；回放真实 DSML 格式错误后重试，核对原始 arguments/content、中文纠正、schema 和缓存前缀不变，采样六份重试提示 |
| [liteforge.yak](liteforge.yak) | `liteforge.Execute`、`ai.FunctionCall` × 默认文本/显式原生/显式文本；HTTP/SSE 首字节屏障、arguments/text 增量流、tools/tool_choice、开放 map/任意 JSON；抽取/分类/总结 × 父循环双协议 × aim/public 两个入口，验证 aim 辅助请求始终为文本流，共 18 次请求 |
| [liteforgeapp.yak](liteforgeapp.yak) | 迁移后的公共应用入口，两种协议各一次请求，未知键及嵌套任意 JSON |
| [rag_applications.yak](rag_applications.yak) | 真实文件读取、问题索引、知识分片、临时入库、关联与检索；模型和 embedding 使用 mock |
| [planning.yak](planning.yak) | 自由探索生成、preset、mocker × 两种协议；读来源、Evidence、文档和任务修改、无效 DAG 原子回滚、稳定 task ID、编辑后一次审核；停在批准后的交接点，不启动业务 worker |
| [exploration.yak](exploration.yak) | 两种协议；fork 调查子 Agent、真实只读工具、共享 Evidence、发现唤醒、运行中 submit 拒绝、实际退出后一次审核、私有 Timeline 不泄漏 |
| [coordinator.yak](coordinator.yak) | `aim.InvokeReAct` 手动选择 Coordinator；native/text × 人工/YOLO；A/B 独立、C 依赖 A、D 依赖 B/C、真实工具、前置验收、一次计划批准、push/pop、报告创建/patch/提交 |
| [notifications.yak](notifications.yak) | 两种协议；不调用 wait action，idle 自动休眠、发现先保存再唤醒、重复 Evidence 不唤醒、完成后自动验收；休眠屏障确认没有模型轮询 |
| [controls.yak](controls.yak) | 两种协议；inspect、reject、retry、cancel、唯一新尝试、取消任务不启动、最终交付保留拒绝/取消事实 |
| [forge.yak](forge.yak) | 自由规划/preset/mocker × 两种协议；新 coordinator 底层、依赖任务、持久业务指令传入协调员/worker/结果上下文、共享 Evidence、一次业务格式化、开放结果对象、一次结果回调及重复 Run 幂等；附带默认 aim 入口 |
| [hostscan.yak](hostscan.yak) | 默认 `aim.InvokeReAct` 的 load_capability / require_ai_blueprint × 两种协议；父会话启用 detached PLAN，内置 hostscan 保持实时审核、八个预置 DAG 任务及前置验收、真实只读工具、共享 Evidence、验收和一次业务报告；提供方失败向调用者返回错误。模型用本地夹具模拟结果，不实际扫描主机 |
| [cache/selftest.yak](cache/selftest.yak) | usage 与 dump 的 correlation ID 对齐、缺失与取消、模型分组、加权缓存统计，不把缺失用量当作零命中 |

每份脚本均可独立执行，例如（直接运行时沿用当前 profile 的全局后台价值评估配置；需要完全隔离时用总入口的 `AISMOKING_CASE`）：

```text
yak common/ai/aismoking/planning.yak
yak common/ai/aismoking/coordinator.yak
yak common/ai/aismoking/liteforge.yak
yak common/ai/aismoking/forge.yak
```

总入口可通过环境变量 `AISMOKING_CASE` 只运行表中的一个文件名（不含 `.yak`，缓存例为 `cache/selftest`）。`AISMOKING_OUTPUT` 指定输出根目录；缺省保存在系统临时目录。`results.json` 记录各例结果和耗时，子目录含 `run.log`、实际 provider 请求采样及上下文检查数据。总入口要求子例输出完成标记且没有 Yak panic，任一失败以非零退出；这也防止旧 CLI 在脚本 panic 后仍返回 0 被误算为通过。

`hostscan.yak` 默认运行四条完整链路，限定总时长 360 秒、单次调用 120 秒，失败传播例不执行扫描。复测单路时，可设置 `AISMOKING_HOSTSCAN_MODE=true/false` 和 `AISMOKING_HOSTSCAN_ENTRY=load_capability/require_ai_blueprint`；结果写入 `hostscan-results.json`，事件和 provider 请求均保留。共享夹具默认总时长 240 秒，`AISMOKING_TIMEOUT_SECONDS` 可指定 1–600 秒的有限预算。

缓存断言检查固定分区和同阶段工具声明稳定，保留请求采样；native replay 可能拆成更多 messages，按分区标记查找内容。mock 不报告实际 provider 缓存利用率。真实用量缺失时保留 `null`，只有 live provider usage 能计算实际缓存比例。

## 可选真实模型与专注模式

这些入口直接 `yak xxx.yak`，不会由默认套件自动执行：

- [live/scan_port.yak](live/scan_port.yak)：原硬编码私人靶机的 SYN/点对点路由实验，直接以 `yak` 运行生产工具。需显式设置 `AISMOKING_SCAN_TARGET` 为已准备好 FTP/SSH/HTTP 服务的单个实验室 IPv4 主机；保留路由错误、开放端口、服务和完成断言，限时 60 秒。`AISMOKING_SCAN_PREFLIGHT=1` 只构造命令，不扫描。确定性的回环、TUN 降级与取消测试仍保留为 Go 回归。
- [live/forge_file_tasks.yak](live/forge_file_tasks.yak)：原 `aiforge/aibp/tests` 的解码、长文件定位及分块链接分析实验，改用公开 Forge Blueprint 的双协议入口；保留原编码串和 HTML 材料，增加完成状态、解码原文、字节位置、上下文及链接来源断言。使用已配置 provider，输出到新的 `AISMOKING_OUTPUT`。设置 `AISMOKING_FORGE_PREFLIGHT=1` 只校验材料和六次 Blueprint 构造，不请求模型，也不代表业务验证通过。旧 Go 实验依赖私人 `openrouter.txt`、忽略执行错误且没有业务断言，已移除；确定性 Forge 生命周期及文件工具回归仍由原 Go 测试和默认冒烟覆盖。
- [live/timeline_memory_finalization.yak](live/timeline_memory_finalization.yak)：`aim.InvokeReAct` 双协议 × 短任务、阈值压缩长任务、空记忆，共六组；正常返回前等待真实保存/索引完成，重复结束不再请求模型，内部进度不进入静态上下文或压缩输入。需可用 embedding 服务，使用临时 `YAKIT_HOME`，仅在本地运行。
- `AISMOKING_MEMORY_FINALIZATION=1 yak common/ai/aismoking/coordinator.yak`：双协议 × 人工/YOLO 的四任务 DAG，全程只在业务报告交付后收尾；验证真实保存、索引、检索及旧自动 triage 为零。Windows 可先在 PowerShell 设置 `$env:AISMOKING_MEMORY_FINALIZATION='1'` 再执行该 Yak 脚本。

- [live/timeline_memory_persistence.yak](live/timeline_memory_persistence.yak)：`aim.InvokeReAct` 的 function call/text stream × 正常、空记忆、部分失败自动重试、失败耗尽、全新 runtime 恢复，共十组；重复投递和恢复不重复写入或通知，已有候选恢复时仅重试保存；中断的压缩尾段从已持久化来源补提取。保存状态不进入模型上下文。使用本地模型夹具及真实记忆后端，需要可用 embedding 服务来验证问题索引。请使用临时 `YAKIT_HOME`；只供本地开发，不进 CI。

- [live/memory_capture_selftest.yak](live/memory_capture_selftest.yak)：本地 HTTP/SSE 夹具检查完整 content、reasoning、分片 arguments、空记忆、错误协议、截断参数和无输出的诊断采样；不访问真实模型。

- [live/memory_protocol.yak](live/memory_protocol.yak)：沿用配置中的轻量模型，对比记忆筛选与 Timeline 摘要的 function call / 文本流；覆盖短约束、已有记忆不重复收录、临时日志、长期约束、范围纠正、待返回调用、引用中的旧协议和 DAG 衔接，默认重复 3 轮。设置 `AISMOKING_OUTPUT` 保存无认证头的实际请求、完整响应体、分别还原的 content / function call arguments、finish reason、评分及语义断言；每次响应和最终结果均落盘，超时、协议失败与语义错误分别记录。`AISMOKING_MEMORY_SOURCE` 可附加已脱敏历史回放，`AISMOKING_MEMORY_KIND=triage/summary`、`AISMOKING_MEMORY_CASE`、`AISMOKING_MEMORY_MODE=function-call/text-stream` 可单独复测；`AISMOKING_MEMORY_EXAMPLES=0` 关闭原生参数示例以做对照，`AISMOKING_MEMORY_SPEED=0` 使用主模型。脚本不持久化记忆；请求或语义检查失败会返回非零，并保留失败采样。
- [live/default_task.yak](live/default_task.yak)：普通对账任务，使用 `aim.InvokeReAct`，校验金额、去重、异常及报告，采样 usage/cache。要求 `LITEFORGE_SMOKE_API_KEY`，模型和输出目录由脚本列出的环境变量设置。
- [live/tool_protocol.yak](live/tool_protocol.yak)：通过 `aim.InvokeReAct` 执行文件对账、计划确认、依赖任务、Evidence 与验收报告；原样转发并断言生产原生请求的 `tool_choice` 为 `auto`，不由测试代理覆盖。采样实际请求、完整响应、非法函数名、重试和 usage。沿用本地已配置的 provider，设置 `AISMOKING_OUTPUT` 为新的采样目录；`AISMOKING_TOOL_MODEL` 可指定模型，`AISMOKING_TOOL_CHOICES=auto,text` 可选定组合。脚本仅确认自己的隔离测试计划，不持久化认证头。直接运行 `yak common/ai/aismoking/live/tool_protocol.yak`，仅供本地开发。
- [live/model_cache.yak](live/model_cache.yak)：沿用本地已配置的 provider，对照 DeepSeek/Qwen 的真实缓存用量。设置新的 `AISMOKING_OUTPUT` 后直接运行；`CACHE_PHASE=runtime` 用 `aim.InvokeReAct` 完成六批文件核对，`boundaries` 对照显式/隐式、独立 message/同 message 内 content block，`replay` 将 `CACHE_REPLAY_SOURCE` 中同一串真实原生请求重放到两个模型，仅观察响应，不执行返回的工具。`CACHE_MODELS` 可覆盖逗号分隔的模型名。`analyze` 离线重算已有目录，不请求模型。`summary.json` 分开主循环与辅助请求，按 provider token 加权，并单列剔除首轮的结果及缺失 usage；认证头不落盘。仅供本地开发，不进 CI。
- [live/coordinator.yak](live/coordinator.yak)：读取本地目录与 README，使用已配置的 provider，自动计划/执行/报告。
- [live/timeline_compression_workflow.yak](live/timeline_compression_workflow.yak)：真实模型通过 `aim.InvokeReAct` 逐个读取本地对账材料，在 pending 通知及最后一批数据到达后调低测试阈值，由正常 before-prompt 路径触发压缩；验证摘要与保留原文传入下一轮、选中 item 的序列化内容不变、历次压缩候选在 SESSION_MEMORY_CANDIDATES 中可见、Evidence 和最终业务结果；任务显式给出持续偏好，避免候选为空导致可见性检查失去意义。设置 `AISMOKING_OUTPUT` 为新目录，沿用已配置 provider，`AISMOKING_TOOL_MODEL` 默认 `deepseek-v4.1-flash`。直接运行 `yak common/ai/aismoking/live/timeline_compression_workflow.yak`，仅本地开发使用，不进 CI。保存样本可用同一 `AISMOKING_OUTPUT` 直接运行 `yak common/ai/aismoking/live/timeline_compression_verify.yak` 离线复验，不重新请求模型；异常数组兼容订单号字符串或含 `order_id` 的对象。
- [live/compression_memory_quality.yak](live/compression_memory_quality.yak)：本地真实模型对照旧版与当前压缩提示词，同一原始 AITAG 资料与 schema，默认各两轮；检查文档宣称不变成用户偏好、与首轮实际候选对照后旧规则不重复提取、不同约束分别维护、项目纠正保留范围，以及无依据的情绪/偏好评分。设置 `AISMOKING_OUTPUT`、`AISMOKING_MEMORY_BASELINE`（旧指令文件）、`AISMOKING_MEMORY_LEDGER`（workflow 样本目录）和 `AISMOKING_MEMORY_USER`（timeline_compression 样本目录），直接 `yak common/ai/aismoking/live/compression_memory_quality.yak`。保存无认证头的请求及完整响应，不入库；这是提示词语义回放，生产压缩与回调链路由前述 `aim.InvokeReAct` 脚本验证。仅供本地开发，不进 CI。
  `AISMOKING_MEMORY_VARIANT=baseline/revised`、`AISMOKING_MEMORY_CASE` 可单独复测；`AISMOKING_MEMORY_VERIFY` 指向已有样本目录时，只复验其 `results.json`，在新的输出目录写 `verification.json`，保留原失败记录，不重新请求模型。

- [live/comprehensive.yak](live/comprehensive.yak)：真实综合任务，直接 `aim.NewInputEvent` 回复一次计划卡，保存事件、流、Evidence、Timeline 和 usage。要求 `YAK_BENCH_API_KEY`、`YAK_BENCH_DIR/fixture/task.txt`；`YAK_BENCH_TEXT_STREAM=1` 选择文本协议。
- `live/benchmark/`：通用事件、工具 HTTP 黑盒、SPA 爬虫指导三个已有模型实验及 JSON cases；默认不执行外部目标，按各脚本参数显式启动。
- [cache/run_react.yak](cache/run_react.yak)、[cache/analyze.yak](cache/analyze.yak)：实际模型缓存实验及已有 dump/usage 分析，参数可用 `--help` 查看。分析库为 [cache/lib.inc](cache/lib.inc)。
- `live/video/`：2 个视频切片、Omni 理解和知识库案例，需 ffmpeg、输入视频及对应 provider；可用脚本参数控制只切片。
- `live/focused/loop_report_generating/`：7 个报告案例，包含初稿、修改、偏移、grep 引用、代码分析与多文件。
- `live/focused/loop_knowledge_enhance/`：7 个知识搜索/过滤/压缩/文档和 benchmark 案例，需要对应知识库和已配置 provider。
- `live/focused/loop_write_python_script/`：2 个 Python 脚本案例，需要 Python 执行环境与 provider。
- `live/focused/loop_infosec_recon/`：原提示词稳定性观测，保留其目标及输出要求，供专门实验使用。

可选案例已归档到新位置，不表示本次执行过真实模型或安装了其外部依赖。新冒烟例都添加到这里，并把确定性案例登记到 `run.yak`。`mock.inc` 和 `cache/lib.inc` 是 include 支持文件，不冒充可运行测试。

## 收集与清理范围

已收集原 Coordinator 的 6 个 Yak 入口、Go `live_runner` 的综合实验、LiteForge 的注入式及直接入口、liteforgeapp 的应用案例、Forge 全流程案例、缓存脚本、17 个循环示例、2 个视频案例和 3 个 AI engine benchmark。注入式 `.yak` 和 Go live runner 已删除；原内部断言保留为普通 Go 回归，公共调用矩阵由此套件承接。VM 配置绑定仍有窄范围 Go 单元测试，不再用外部注入式业务脚本执行计划。

`aiforge/buildinforge` 的生产蓝图、`yakscriptforai` 的生产工具、Yak 编译器/网络 mock fixtures 不属于 AI 冒烟，不移动或删除。旧评测文档保留历史数据，但入口统一指向本目录。

这套 Yak 冒烟仅用于本地开发与 review，不由 CI 执行，也不作为 CI gate。CI 继续运行已有 Go 单元和集成回归。

- [live/timeline_memory_longrun.yak](live/timeline_memory_longrun.yak)：真实配置模型的长程记忆对照，默认 DeepSeek v4.1 flash。通过 `aim.InvokeReAct` 逐批核对 18 份本地材料、逐批保存 Evidence、经历两次阈值压缩及正常任务收尾，检查异步回执与报告数字，记录原生主循环和文本流辅助请求、完整响应、provider usage、原文保留及真实记忆保存/索引。分别使用改动前、后 Yak 二进制直接运行同一脚本，设置 `AISMOKING_VARIANT=baseline/current` 和不同的新 `AISMOKING_OUTPUT`；会话与记忆 namespace 自动隔离。需要已配置的 AI 与 embedding 服务，单轮预算最多 1500 秒；收尾仍遵循生产的有界生命周期，待处理状态会使验收失败。仅本地使用，不进 CI。
- [live/timeline_memory_longrun_verify.yak](live/timeline_memory_longrun_verify.yak)：用同一 `AISMOKING_OUTPUT` 离线复核业务数字、摘要衔接、保留原文和公开 BM25 记忆查询，不重新调用 AI、不覆盖原始结果。业务/检索通过不能代替收尾通过，仍须查看 `result.json` 与运行日志。
