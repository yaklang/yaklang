# AI 冒烟测试

所有 AI mock、冒烟和模型观测脚本集中在这里。业务流程、模型夹具、审核回复和断言写在 Yak 中，不需要 Go 注入变量或启动 Go 业务运行器。Go 单元与集成测试继续验证内部状态机、并发、恢复、权限及 VM 绑定。

从仓库根目录执行：

```text
yak common/ai/aismoking/run.yak
```

总入口逐个以当前 Yak 可执行文件启动独立进程，相当于 `yak xxx.yak`。每例拥有独立的 `YAKIT_HOME`、材料目录、日志和采样，避免会话污染及 SQLite 锁冲突。总入口用 `yak tiered-ai-config --enable --config-file` 写入测试专用 profile：全局模型指向关闭的 loopback 端口，并禁用 fallback，使后台价值评估快速失败，不访问外部模型。不能用空模型列表或已过时的 `enabled=false` 实现隔离，启动会自动补齐默认模型；这些设置只影响临时 profile，不改变开发者自己的配置。业务模型通过正常工作的本地 mock provider，不需要 API key。模型回答可控；协议投影、HTTP/SSE、循环、工具、审核、调度、Timeline 和 artifacts 使用真实实现。

| 脚本 | 覆盖内容 |
| --- | --- |
| [mainloop.yak](mainloop.yak) | 默认 `aim.InvokeReAct` × 两种协议；实际读文件、session Evidence、冻结提升和纯动态区不重复展示用户输入 |
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

- [live/memory_capture_selftest.yak](live/memory_capture_selftest.yak)：本地 HTTP/SSE 夹具检查完整 content、reasoning、分片 arguments、空记忆、错误协议、截断参数和无输出的诊断采样；不访问真实模型。

- [live/memory_protocol.yak](live/memory_protocol.yak)：沿用配置中的轻量模型，对比记忆筛选与 Timeline 摘要的 function call / 文本流；覆盖短约束、已有记忆不重复收录、临时日志、长期约束、范围纠正、待返回调用、引用中的旧协议和 DAG 衔接，默认重复 3 轮。设置 `AISMOKING_OUTPUT` 保存无认证头的实际请求、完整响应体、分别还原的 content / function call arguments、finish reason、评分及语义断言；每次响应和最终结果均落盘，超时、协议失败与语义错误分别记录。`AISMOKING_MEMORY_SOURCE` 可附加已脱敏历史回放，`AISMOKING_MEMORY_KIND=triage/summary`、`AISMOKING_MEMORY_CASE`、`AISMOKING_MEMORY_MODE=function-call/text-stream` 可单独复测；`AISMOKING_MEMORY_EXAMPLES=0` 关闭原生参数示例以做对照，`AISMOKING_MEMORY_SPEED=0` 使用主模型。脚本不持久化记忆；请求或语义检查失败会返回非零，并保留失败采样。
- [live/default_task.yak](live/default_task.yak)：普通对账任务，使用 `aim.InvokeReAct`，校验金额、去重、异常及报告，采样 usage/cache。要求 `LITEFORGE_SMOKE_API_KEY`，模型和输出目录由脚本列出的环境变量设置。
- [live/coordinator.yak](live/coordinator.yak)：读取本地目录与 README，使用已配置的 provider，自动计划/执行/报告。
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
