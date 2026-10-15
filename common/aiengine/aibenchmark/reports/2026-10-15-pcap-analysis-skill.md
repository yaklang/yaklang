# PCAP 分析 Skill 与三个核心工具的验证报告

日期：2026-10-15。基于 main `979cdeec87815ff51d3d6ea075063a91b516fffd`，分支 `codex/traffic-skill-tests`，PR [#5298](https://github.com/yaklang/yaklang/pull/5298)。

## 交付内容与调用方式

新增一个内置 Skill `pcap-analysis`，以及三个注册的 Yak AI 工具：

| 工具 | 输入与行为 | 业务结果 |
| --- | --- | --- |
| `load_pcapfile` | `pcapfile`；调用 `pcapdb.GetOrCreatePCAPDatabase`，开启协议及 session/stream 分析，按完整内容去重复用 | 持久的 `dataset_id`、ready 状态、能力、精确计数、内容 hash |
| `pcapdb` | `id` 或 `pcapfile` 二选一；ID 打开已有库，文件路径自动导入/复用并补齐分析 | 有字节预算、游标、完整性标记和业务错误的结构化证据 |
| `manage_pcapdb` | `op=list/count/validate/export/delete`；按实际 DatasetID 管理已有库 | 有界目录、登记计数、校验状态、精确导出和删除/清理状态 |

自然语言问题由 AI 按 Skill 组织查询并回答。查询工具提供结构化操作：metadata、packets、protocols、fields、sessions、streams、details、packet_data、protocol_data、stream_data、protocol_packets、chunk_packets、packet_sessions。

例如，首次加载传 `{"pcapfile":"/absolute/path/capture.pcap"}`。取得 `dataset_id` 后，传 `{"id":"返回的 dataset_id","query_type":"protocols","protocol":"http","limit":20}`。路径方式可能创建或补建索引；源文件删除后继续使用 ID。

业务结果放在 Yak 的 `RESULT` 中，经真实工具适配器进入 `result` 语义通道。查询页保留原生 `error.code/message/retryable`，不会把能力缺失、记录不存在或详情超预算变成成功的空结果。参数和执行错误仍作为工具执行错误返回。

每次调用显式携带身份、查询模式及过滤条件。续页使用前页的 `next_cursor`，保持其他条件一致。默认每页 20 条、紧凑 JSON 预算 8192 字节、BLOB 预览 128 字节；工具允许的上限分别为 1000 条、65536 字节、4096 字节。原始 BLOB 不进入摘要列表。

## 确定性工具测试

`TestPCAPAnalysisTools` 读取三个真实嵌入脚本，经过正式的工具转换器和 Yak VM，操作隔离的 profile 与真实 SQLite 子库。未替换 pcapdb、解析器或工具回调。

HTTP 样本由测试生成，并与真实模型测试使用的固定 PCAP 逐字节比较：

- 709 字节；SHA-256 `685eb9d0d0c42a562be4bc791522049182b0a5d45634d8b1627a355cde687b17`。
- 8 个物理包、2 条 HTTP 消息、1 个完整 TCP 会话、2 个方向流。
- 请求为 `GET /pcap-skill-proof`，Host 为 `skill.example`；请求跨物理包 4、5。
- 包 6 承载 `HTTP/1.1 200 OK`，正文为 `OK`。请求和响应分别为 55、40 字节。
- DNS 数值字段另用独立合成捕获验证，避免将 HTTP 的字符串状态码误当整数。

十组行为测试通过，覆盖：

1. 同内容不同路径去重、ID/文件选择，以及 AI 参数 schema 的身份互斥约束。
2. 协议消息分页不遗漏、真实字段发现、详情核实后字符串搜索、missing 与 JSON null 的区别。
3. DNS 整数字段的类型匹配：数值命中，文本同值不命中。
4. 纳秒时间边界与 IP/端口筛选；时间通过十进制字符串传递，避免浮点损失。
5. session 完整性、双向 stream、截断预览、消息/块/包/会话索引关联。
6. 仅用查询工具自动导入，以及已有库未开启分析时返回错误、按文件补齐分析后可查询。
7. 单条详情超过预算时保留 `result_too_large` 和原游标；扩大合法预算后能读取完整详情。
8. 无效身份、过滤器组合、整数溢出、无效实数/布尔值和输出预算明确失败。
9. 删除源 PCAP 后，仍可按 ID 读取计数及保存的协议字节。
10. 管理操作的分页/计数、深度校验、字节一致导出、不覆盖已有文件、占用时拒绝删除、成功删除及幂等重试。

执行命令：

```sh
go test -p 2 ./common/ai/aid/aitool/buildinaitools/yakscripttools/test \
  ./common/ai/aid/aireact \
  -run '^(TestPCAPAnalysisTools|TestBuiltinSkillsFS_|TestBuiltinSkills_LoadedByReAct)' \
  -count=1 -timeout 3m

go test -p 2 ./common/yak ./common/pcapx/pcapdb \
  -run '^(TestPCAPDBYak|TestPCAPDBBoundedResultPages|TestPCAPDBFieldDiscovery|TestPCAPDBCallerLeases)' \
  -count=1 -timeout 3m
```

两组命令均通过；Skill 的 frontmatter 验证通过，当前源码的 Yak CLI 构建通过。测试时使用临时 `YAKIT_HOME`，没有向日常流量库导入测试数据。

macOS 最终构建使用下文的 `-ldflags='-s -w'`。一次默认链接在生成 dSYM 时因临时磁盘空间不足中断；清理本任务的重复二进制并关闭调试符号后构建通过，没有调整功能代码或测试断言。

## 首轮真实 AI 调用（改名前）

使用从本分支源码构建的 Yak CLI，执行：

```sh
go build -p 2 -ldflags='-s -w' -o /tmp/pcap-skill-yak ./common/yak/cmd/yak.go

# 事先通过环境配置 PCAP_SKILL_API_KEY、PCAP_SKILL_MODEL。
# API key 不写入脚本、命令参数、事件或报告。
export YAKIT_HOME="$(mktemp -d)"
export PCAP_SKILL_OUTPUT="$(mktemp -d)"
/tmp/pcap-skill-yak -c 'include "common/ai/aismoking/live/pcap_analysis.yak"'
```

首轮测试时查询工具名为 `query_pcap_in_db`；本次按要求改为 `pcapdb`，并增加 `manage_pcapdb`。以下三项为首轮记录，后文记录三个工具的新增生命周期验证。测试脚本内直接调用 `aim.InvokeReAct`；使用真实模型、真实 Skill 加载流程、真实工具及数据库。主循环使用本机配置的 AIBalance `memfit-qwen3.8-flash-free`；运行时辅助请求还观察到默认的 `memfit-light-free`。未注入模拟 AI 回答。每项最多 24 次迭代、300 秒，首轮工具范围为 load_pcapfile 和改名前的查询工具；当前测试入口包含三个核心工具。

| 场景 | 结果 | 实际工具调用 | 核实内容 |
| --- | --- | --- | --- |
| 导入并回答 HTTP 问题 | 通过 | load 1 次、query 2 次；128.1 秒 | 8 个物理包、2 条消息、请求路径、Host、200 状态码及消息 ID |
| 源文件删除后按 ID 分析 | 通过 | query 8 次；145.7 秒 | 完整性标记、两个方向流及有限预览、请求与包 4/5 的关联 |
| 按文件自动选择、发现字段并精确搜索 | 通过（改进指导后重测） | query 3 次；128.3 秒 | fields → details → protocols，以真实字符串路径精确命中消息 1 |

每个场景均观察到 `pcap-analysis` 的 Skill reference material，以及真实的 `tool_call_start/param/result` 事件。测试除了检查最终回答，还检查实际调用模式和返回的证据；会话标记、两个流的 BLOB 预览、包关联 ID、发现路径与搜索路径的一致性也有语义断言。

前两项保存的真实事件还通过加强后的语义断言复核，未额外调用模型来替代或重写原始结果。

首轮第三项未通过：模型使用 `fields` 加字段条件返回采样目录，未执行要求的 `protocols` 消息列表查询，并混淆了 `occurrences` 与目录条数。保留原来的失败断言，已在 Skill 和工具说明中明确“fields 发现 → details 核实 → protocols 筛选”，以及路径数、出现次数和扫描消息数的区别，使用新构建和新的隔离库重测。

第一次重测完成了正确的消息查询，但曾将 `protocol=http` 带到 details，被工具拒绝后自行恢复。严格检查仍判为失败。随后补充详情调用示例，要求切换模式时重新选取参数、详情省略列表筛选，继续保留工具执行错误断言。最后在新的隔离库中重测通过：仅执行 fields、details、protocols 三次查询，没有工具执行错误或业务错误，命中真实消息 ID 1。

## 本轮修正与边界

真实工具测试发现现有 `cli.Bool` 不采用声明的 true 默认值。加载工具因此固定开启协议和 stream 分析，避免 AI 只传文件路径时意外得到未分析的库；没有改动通用 CLI 行为。

Skill 要求先检查执行错误和业务 `error`，再解释 Items、分页及采样。详情必须完整返回或失败；base64 预览不等于完整正文，stream 游标不会补回前一块被截掉的字节。`session.complete` 仅表示库的规则成立，不证明已捕获每个 ACK、完整四次挥手或完整应用事务。

这些验证支持三个核心工具与 Skill 的离线分析闭环。合成 HTTP/DNS 样本和三项真实模型任务不能证明所有协议、任意规模捕获或每次模型决策都正确。报告中的秒数包括模型、辅助调用和运行时收尾，不能作为 SQLite 导入/查询性能基准；fields 仍是有界采样，字段条件沿用现有 pcapdb 的检索能力。


## 三个核心工具的生命周期补充验证

`query_pcap_in_db.yak` 已改为 `pcapdb.yak`。内置工具升级同步会移除旧名称的内置记录，保留用户编辑/创建的同名脚本；注册发现、旧名称退役及用户脚本保留均有测试。

`manage_pcapdb` 包含以下操作：

| op | 关键参数 | 结果约束 |
| --- | --- | --- |
| list | limit / after / result_bytes | 目录分页；使用 items[].dataset_id，数字 id 只用于游标 |
| count | 无 | registered_count；包含无效/未就绪登记 |
| validate | id / deep（默认 false） | 明确 valid、busy、state、error；deep 会遍历 BLOB |
| export | id / output | 恢复原捕获全部字节，不覆盖目标文件 |
| delete | 精确 id | deleted、cleanup_pending、error；保留源捕获和导出文件 |

原生删除使用 GORM 事务完成登记软删除和内容去重键释放，只删除该库 UUID 目录内的 index.sqlite/WAL/SHM/rollback journal。读池在生命周期内持有跨进程共享锁，删除必须同时取得实例写锁和独占生命周期锁；占用即返回 busy。删除意图先持久落盘，进程中断后启动续做清理；删除标记与锁 inode 保留，防止恢复登记或旧等待者造成复活。已完成删除的启动检查批量读取活跃登记 ID，并检查已知 SQLite 文件，不重开 SQLite 或反复提交事务。

原生测试覆盖独立 manager/reader、writer 占用、取消、重复删除、删除后重新导入生成新 ID、目录外路径拒绝，以及 durable intent 后中断/登记丢失的启动恢复。另用不可删除的模拟 sidecar 检查 cleanup_pending/error 保留、恢复诊断和解除阻塞后的重试。相关 race 检查通过（包含真实子进程保持 reader、父进程拒绝删除、reader 结束后删除成功）。

真实模型新增场景为 manage_lifecycle：仅在隔离的临时库中，授权 AI 导入本测试样本，按三工具完成列库、metadata 查询、校验、导出、删除、再次列库；检查源文件和导出文件均与 709 字节样本完全一致。实际模型完整执行 7 次工具调用：load_pcapfile 1 次、pcapdb 1 次、manage_pcapdb 5 次（list 两次、validate/export/delete 各一次），116.8 秒。8 个包、valid=true、deleted=true、cleanup_pending=false、最终 list 为空均与实际工具结果一致。源 PCAP 和导出文件仍各为 709 字节，SHA-256 均为上文样本值。

第一轮执行完成后的测试脚本曾因 exportPath 定义在局部 if 块中，导致收尾读取找不到变量并误报文件不一致；已将变量移到循环作用域。未修改字节一致性断言，使用最终检查逻辑重新读取原始真实调用事件、同一隔离库及实际源/导出文件，全部语义检查通过。第二轮真实调用在导出后被会话中断，不计作通过场景，也未替换第一轮原始记录。


本次验证命令：

```sh
go test -p 2 ./common/pcapx/pcapdb ./common/yak ./common/ai/aid/aireact \
  ./common/ai/aid/aitool/buildinaitools/yakscripttools/test \
  -run '^(TestPCAPDB|TestPCAPAnalysisTools|TestBuiltinSkillsFS_|TestBuiltinSkills_LoadedByReAct)' \
  -count=1 -timeout 5m

go test -p 2 ./common/pcapx/pcapdb \
  ./common/ai/aid/aitool/buildinaitools/yakscripttools \
  ./common/ai/aid/aitool/buildinaitools/yakscripttools/test \
  -run '^(TestPCAPDBDelete|TestPCAPCoreToolRename|TestRemoveRetiredBuiltInAIToolsPreservesCustomTools|TestPCAPAnalysisTools)' \
  -count=1 -timeout 3m

go test -p 2 -race ./common/pcapx/pcapdb -run '^TestPCAPDBDelete' -count=1 -timeout 3m

# 相同真实模型配置和新的隔离 YAKIT_HOME / PCAP_SKILL_OUTPUT 下运行管理场景：
PCAP_SKILL_CASES=manage_lifecycle /tmp/pcap-skill-yak \
  -c 'include "common/ai/aismoking/live/pcap_analysis.yak"'
```

上述本地回归及 race 检查通过。真实模型的管理场景使用了正式工具和 SQLite，保存的事件及实际文件已按修正后的最终断言复核通过；不把进程中断、测试脚本误报或未完成运行记作新的完整通过运行。
