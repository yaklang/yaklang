# Agent 默认异步抓包与 pcapdb 实时查询实施计划

日期：2026-11-05

状态：已按用户反馈收敛实现；验证结果见末尾。

## 1. 实现原则

文件导入与网卡捕获共用数据库结构和协议/session/stream 写入逻辑。区别在输入终止方式：文件到 EOF，网卡到取消。已提交数据始终通过原来的 query_pcapdb 查询。

- 不新增 capturing 数据库状态；抓包生命周期由现有异步执行管理。
- 不新增捕获元数据列，不调整子库 schema 版本。
- 不修改查询 SQL、筛选、分页、详情、查询结果状态或 query_pcapdb 脚本。
- 不扩展工具框架的 PartialResult、取消字段或输出观察器。
- 抓包持续运行，不设置默认时长、包数或内存历史保留窗口。
- 内存缓冲和事务批次有上限，数据持续写入 SQLite。
- 停止后保持同一个 dataset_id 和证据记录 ID，不重新导入。
- 首版单设备单库；跨任务常驻、多设备混合、变化订阅和自动分库不在本次范围。

## 2. 对外功能

### 网卡枚举

list_pcap_interfaces 返回设备名称、说明和地址，保留无 IP 的设备。枚举不承诺有打开权限，启动时返回实际错误。

### 默认异步抓包

start_pcap_capture 接受 interface、bpf、snaplen、promisc 和可选 duration_seconds。省略时长时，持续运行到取消。使用现有 WithDefaultAsync(true) 调度。

异步提交取得 execute_id。设备和数据库启动成功后，stdout 输出一行 JSON：

```json
{"dataset_id":"<dataset-uuid>"}
```

通过 manage_async_execution_runtime 的 inspect 读取现有 stdout 即可取得身份，无需通用中间结果扩展。随后调用原来的查询工具：

```json
{"id":"<dataset-uuid>","query_type":"protocols","protocol":"http"}
```

库的 ready 表示已提交内容可查询，不表示网卡捕获已经结束。空页和 has_more=false 也不表示捕获结束。执行是否仍运行由 execute_id 对应的任务状态表达。

### 停止

使用现有 stop、wait、inspect。异步 Resource 已经等待 callback 清理完成，直接复用，不新增 JoinOnCancel 等工具元数据。

停止顺序为：停止输入、排空解析、提交剩余记录、释放设备和 writer。原库保留。所属任务结束同样会取消抓包，不承诺跨任务驻留。

## 3. Go 层改动

### 公共协议批次写入

从 pcap_protocol.go 的离线回放函数抽出 protocolBatch，供离线回放与实时捕获共用：

- 协议事件转存储记录；
- session、stream 和原始包关联；
- JSONB 字段转换；
- 有界批次及事务写入。

复用 sessionIndexer，保持跨提交的 TCP 和协议上下文。同批次尚未提交的关联先查内存，再查已提交数据库。

### 持续写入入口

pcap_live.go 提供以下职责：

- CreateLiveDataset 创建同结构的自包含捕获库。
- Feed 将原始包保存为包记录及可导出的捕获 framing，再交给同一个 PacketAnalyzer。
- Flush 在一个事务内提交包、分析结果、关联和对应 manifest。
- Close 排空分析器并完成最后一次提交。
- RunLiveCapture 对接单个原生 pcap 设备，持有实际捕获生命周期。

采用一个 writer 和独立 reader，复用 SQLite WAL。按批次数量、字节预算或最长等待时间提交；时间间隔只影响可见延迟，不限制捕获时长。

解析回调与定时提交的共享数据需要串行保护。输入取消后，收尾 SQL 使用独立的有限等待 context，避免最后提交直接继承已取消的 context。

### 只读打开

Open/Acquire 原来会进入需要 writer 锁的独占校验。实时写入持续持有该锁，因此对此来源使用只读 manifest 校验，再复用原来的 reader pool。

保留 schema、dataset 身份、路径、删除标记和 reader lifetime lock 检查。查询本身保持原样。

### 捕获摘要与离线去重

复用现有 FingerprintMode/Fingerprint 字段保存 live-sha256 和当前已提交捕获的 SHA256。FullSHA256 继续只用于离线文件的内容去重，避免两个相同的实时捕获（包括空捕获）发生唯一索引冲突或改变身份。

只在 manifest 完整性校验、导出和重建处统一选择实际捕获摘要。新建实时库不新增列、不改唯一索引，也不放宽离线库的校验条件。

实时捕获的 SourcePath 为空，导出必须给出明确目标路径。活动 writer 继续阻止删除、全量校验、导出和破坏性重建。

### 错误和进程退出

写入失败时回滚本批次，保留最后一个一致提交及原有可读状态；通过已有 LastError 和工具执行错误说明失败，不新增捕获失败状态分支。

进程异常退出后，SQLite 回滚未提交事务；已提交记录仍可通过同一个 ID 查询。数据库不维护是否仍有网卡捕获，不能从 ready 推断任务仍运行，也不能从 ready 推断采集完整。

## 4. 文件范围

| 文件或目录 | 用途 |
| --- | --- |
| common/pcapx/pcapdb/pcap_live.go | 实时包源与持续写入 |
| common/pcapx/pcapdb/pcap_protocol.go | 共用协议批次写入 |
| common/pcapx/pcapdb/pcapdb_instance_manager.go | 活动 writer 下只读打开 |
| common/pcapx/pcapdb/metadata.go | 在现有字段中标识摘要方式 |
| common/pcapx/pcapdb/pcap_database.go | 复用捕获摘要做完整性检查 |
| common/pcapx/pcapdb/pcap_export.go、pcap_rebuild.go、exports.go | 摘要校验与导出路径处理 |
| common/ai/aid/aitool/buildinaitools/pcaptools/ | 网卡枚举与默认异步工具 |
| common/ai/aid/aitool/buildinaitools/buildin.go | 注册工具 |
| common/ai/aid/aireact/skills/pcap-analysis/SKILL.md | 同库查询与停止说明 |
| 对应测试文件 | 持续写入、并发、异步和脚本回归 |

查询相关 Go 文件、query_pcapdb 脚本、通用工具运行时和观察器已恢复原实现。

## 5. 验证清单

- 创建空库后原查询立即可用。
- 独立 manager 在 writer 仍运行时打开和查询库。
- 低流量未满批次也按时间提交。
- TCP/HTTP 跨多次提交正确重组，请求响应及包关联有效。
- 原有各 query_type 在持续写入时可用，停止后 ID 不变。
- 实际 SQL 写入失败保留上次提交及可读性。
- 主任务取消和 manager shutdown 等待 writer 收尾。
- 进程退出后，只保留已经提交的数据。
- 相同内容和空捕获可以各自建立独立库。
- 默认异步和已有 stop/wait 的清理语义正常。
- 原 query_pcapdb 脚本能查询当前库及停止后的库。
- 离线导入、去重、导出、校验和查询回归通过。

优先用可控包源、真实 SQLite 事务和并发检查验证。真实网卡测试是 opt-in；本机此前因 /dev/bpf0 权限不足未完成实际网卡验证，不把这次失败写成通过。

## 6. 验证记录

清理后的验证已通过：

```sh
go test ./common/pcapx/pcapdb -count=1
go test -race ./common/pcapx/pcapdb ./common/ai/aid/aitool/buildinaitools/pcaptools -run '^TestLive|^TestCaptureTool' -count=1
go test ./common/ai/aid/aitool/buildinaitools/yakscripttools/test -run '^TestPCAP|^TestPcap' -count=1
go test ./common/ai/aid/aireact ./common/ai/aid/aicommon -run '^TestToolDefaultAsyncAndExplicitOverride$|^TestToolDefaultAsyncBatchOverrides$|^TestDirectToolAsyncActionIntegration$|^TestToolCallerInspectorWithAsyncManager$' -count=1
```

真实网卡用例仍因本机 BPF 权限不足未完成；可控包源、实际 SQLite 读写及脚本查询测试通过。race 链接阶段出现 macOS LC_DYSYMTAB 警告，测试退出码为零，未报告数据竞争。

提交范围仅包含本任务代码、测试和本文档。工作区原有的 docs/session-skill-sync-api.md 未改动。

## 7. 网卡权限授权修正（2026-11-06）

参考 lowtun 的 privileged.Executor 授权方式，复用仓库已有 pcapfix；打开设备发生权限错误时请求系统授权，再重试一次。正常打开、网卡不存在和驱动错误不发起授权，用户拒绝或取消任务后不会继续重试。

为 pcapfix 增加保持原 Fix() 兼容的 FixWithContext(ctx)，让授权进程跟随抓包任务取消。macOS 直接使用系统管理员授权框，沿用现有 BPF 权限修复脚本。抓包原生测试的启动等待延长为一分钟，以允许人工完成授权。

授权分支使用注入的打开/授权函数验证，不在自动化测试中改变系统权限。真实授权弹窗与授权后的原生抓包需在运行引擎的桌面会话内手工验证。

修正后的回归通过：

```sh
go test ./common/pcapx/pcapdb ./common/utils/pcapfix ./common/ai/aid/aitool/buildinaitools/pcaptools -count=1
go test -race ./common/pcapx/pcapdb ./common/utils/pcapfix -run '^TestLiveCapturePermission|^TestFixWithContext' -count=1
```
