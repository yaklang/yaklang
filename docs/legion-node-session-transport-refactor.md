# Legion Node Session And Job Bridge Refactor

更新时间：2026-03-28

## 本轮已经落地的边界

- `yak-node` 仍然是单进程、单 `node_id`、单 exe。
- Yak 执行内核没有重写，仍然落在现有 `scannode` 执行链里。
- 节点和平台的入会、续租已经切到 Legion 平台 HTTP API：
  - `POST /v1/nodes/bootstrap`
  - `POST /v1/node-sessions/{sessionID}/heartbeats`
- 平台到节点的最小命令闭环已经切到 JetStream：
  - `plugin.sync`
  - `job.dispatch`
  - `job.cancel`
  - `capability.apply`
- 节点到平台的最小 plugin 事件闭环已经落地：
  - `plugin.sync.status`
  - `plugin.sync.failed`
- 节点到平台的最小作业事件闭环已经落地：
  - `job.claimed`
  - `job.started`
  - `job.progressed`
  - `job.asset`
  - `job.risk`
  - `job.report`
  - `job.succeeded`
  - `job.failed`
  - `job.cancelled`
- 节点到平台的最小 capability 事件闭环已经落地：
  - `capability.status`
  - `capability.failed`

这意味着节点现在已经同时具备：

- 新的 session 入会链
- 新的平台命令接入链
- capability desired state 持久化链
- 复用现有 Yak 执行链的单进程执行面

## 当前代码分层

### `common/node`

- `BaseConfig`
  - 收敛节点基础配置：`node_id`、`enrollment_token`、`api_url`、心跳周期等。
- `SessionTransport`
  - 抽象节点如何和 Legion 平台建立 session。
- `HTTPTransport`
  - 当前默认实现，通过 HTTP bootstrap/heartbeat 与平台交互。
- `NodeBase`
  - 负责 session 生命周期、心跳续租、节点基础运行态，以及兼容当前扫描执行链需要的底座能力。

### `scannode`

- `ScanNode`
  - 继续承载本地 Yak 执行入口和运行时任务管理。
- `legionJobBridge`
  - 基于当前 session 下发的 JetStream subject 消费 `plugin.sync` / `job.dispatch` / `job.cancel` / `capability.apply`。
- `jobEventPublisher`
  - 把作业事实回传为结构化 `job.*` protobuf 事件。
- `PluginCacheManager`
  - 下载、校验并持久化 plugin release 缓存。
- `pluginEventPublisher`
  - 把插件同步结果回传为 `plugin.sync.status` / `plugin.sync.failed`。
- `CapabilityManager`
  - 校验 capability key 和 desired spec，并把目标配置落盘到本地。
- `capabilityEventPublisher`
  - 把 capability 应用结果回传为 `capability.status` / `capability.failed`。
- `TaskManager`
  - 继续作为本地运行任务的索引与取消入口。

这里的关键点是：

- 平台入口已经切到新命令模型
- 节点内部执行已经收敛为本地脚本执行服务
- capability 配置入口已经独立出来
- 没有拆出第二个 client 进程

## 当前真实通信链路

### 1. 节点启动与入会

`yak node` 启动后，节点会：

1. 调用 `POST /v1/nodes/bootstrap`
2. 拿到：
   - `node_session_id`
   - `session_token`
   - `nats_url`
   - `command_subject`
   - `event_subject_prefix`
3. 保存当前 session，并开始周期性发送 heartbeat

当前节点不会硬编码具体 subject 前缀，而是完全使用平台 bootstrap 返回的：

- `command_subject`
- `event_subject_prefix`

### 2. 平台下发 `plugin.sync`

平台发布到：

- `<command_subject>.plugin.sync`

节点处理过程是：

1. 校验 `command_id/target_node_id/release_id/entry_kind/artifact_uri/artifact_sha256`
2. 下载 artifact
3. 校验 `sha256` 和文件大小
4. 把 release 持久化到本地缓存目录
5. 回传 `plugin.sync.status` 或 `plugin.sync.failed`

当前本地缓存目录在：

- `<YAKIT_HOME>/legion/plugins/releases/<release_id>/`

### 3. 平台下发 `job.dispatch`

平台发布到：

- `<command_subject>.job.dispatch`

节点侧 `legionJobBridge` 会用：

- `<command_subject>.>`

做 pull consumer 订阅，然后按 subject suffix 分发命令。

当前 `job.dispatch` 要求：

- `plugin_release_id`
- `execution_kind = yak_script`

处理过程是：

1. 校验 `command_id/job_id/subtask_id/attempt_id/target_node_id/plugin_release_id`
2. 按 `plugin_release_id` 从本地 Legion 插件缓存读取脚本内容
3. 先发 `job.claimed`
4. 再发 `job.started`
5. 调用本地脚本执行服务

桥接关系如下：

- `job.job_id -> ScriptExecutionRequest.TaskID`
- `job.attempt_id -> ScriptExecutionRequest.RuntimeID`
- `job.subtask_id -> ScriptExecutionRequest.SubTaskID`
- `plugin_release_id -> 本地 cached release script`
- `input_json -> ScriptExecutionRequest.ScriptJSONParam`

也就是说，这一轮不是重写 Yak 执行器，而是把新平台命令直接桥到节点内部的本地执行服务。

### 4. 平台下发 `job.cancel`

平台发布到：

- `<command_subject>.job.cancel`

节点收到后按 `subtask_id` 映射本地任务 ID：

- `script-task-<subtask_id>`

然后通过 `TaskManager` 找到任务，写入取消原因并调用 `Cancel()`。

### 5. 节点回传 `job.* / plugin.*`

节点发布到：

- `<event_subject_prefix>.plugin.sync.status`
- `<event_subject_prefix>.plugin.sync.failed`
- `<event_subject_prefix>.job.claimed`
- `<event_subject_prefix>.job.started`
- `<event_subject_prefix>.job.progressed`
- `<event_subject_prefix>.job.asset`
- `<event_subject_prefix>.job.risk`
- `<event_subject_prefix>.job.report`
- `<event_subject_prefix>.job.succeeded`
- `<event_subject_prefix>.job.failed`
- `<event_subject_prefix>.job.cancelled`

事件元数据统一带：

- `event_id`
- `event_type`
- `causation_id`
- `correlation_id`
- `node_id`
- `node_session_id`
- `emitted_at`

其中：

- `causation_id = command_id`
- `correlation_id = attempt_id`

### 5. 平台下发 `capability.apply`

平台发布到：

- `<command_subject>.capability.apply`

节点收到后会：

1. 校验 `metadata.command_id`
2. 校验 `target_node_id`
3. 校验 `capability.capability_key`
4. 校验 `desired_spec_json` 必须是合法 JSON
5. 将期望配置落盘到：
   - `<base_dir>/legion/capabilities/<capability_key>.json`

当前 `CapabilityManager.Apply(...)` 返回的成功状态固定为：

- `status = stored`
- `message = desired spec persisted locally; runtime hook is not wired yet`

这个状态含义很明确：当前只完成了 capability desired state 的本地持久化，还没有把具体 HIDS runtime 启停逻辑接进去。

### 6. 节点回传 `capability.*`

节点发布到：

- `<event_subject_prefix>.capability.status`
- `<event_subject_prefix>.capability.failed`

事件元数据继续沿用统一结构：

- `event_id`
- `event_type`
- `causation_id`
- `correlation_id`
- `node_id`
- `node_session_id`
- `emitted_at`

其中 capability 事件当前固定关系为：

- `causation_id = command_id`
- `correlation_id = node_id + ":" + capability_key`

## 当前已经剔除的旧上报面

这轮仍然保留的只有 Yak 执行本体：

- 本地脚本执行服务仍通过临时脚本文件执行 Yak
- 执行期的进度、风险、指纹、报告等仍走现有 `YakitServer webhook`

旧结果上报链这轮已经直接删除：

- 删除了 `ScanNode.feedback(...)`
- 删除了 `NodeBase.Notify(...)`
- 删除了 `common/node/script_engine.go` 中旧 script runtime 通道
- 删除了 `common/spec` 中仅服务于这条旧链的消息模型
- 删除了 `scannode/scanrpc` 整个 RPC 壳
- 删除了 `NodeBase` 上的 `mq.RPCServer` / `mq.RPCClient` / broker 运行时依赖
- 删除了 `common/node/baserpc` 与基础管理 RPC

现在节点对平台的结果回传只剩一套：

- `job.progressed`
- `job.asset`
- `job.risk`
- `job.report`
- `job.claimed`
- `job.started`
- `job.succeeded`
- `job.failed`
- `job.cancelled`

也就是说，这个 rewrite 分支里已经不存在“旧结果双写”的兼容状态。

当前没有落地的是：

- capability runtime 真正启停 / 热更新
- `capability.alert`

## 当前 CLI 入口

`yak node` 现在最少需要下面参数：

```bash
yak node \
  --api-url http://127.0.0.1:8080 \
  --enrollment-token <token> \
  --id scanner-node-01
```

可选参数：

- `--version`
- `--max-running-jobs`
- `--heartbeat-interval`

## 当前结论

节点这边已经不是“只有 session transport 重构”，而是已经进入下面这个过渡状态：

- session 生命周期已经切到 Legion HTTP API
- 平台命令入口已经切到 JetStream `job.dispatch/job.cancel`
- 节点内部执行面已经不再依赖 AMQP RPC 壳，而是直接走本地执行服务

这正符合当前阶段的目标：先把平台和节点的通信层重构出来，再决定后续是否继续收敛细粒度扫描结果事件和 capability/HIDS 事件。

## 当前验证结果

截至 2026-03-28，这一轮已经完成下面这些真实验证：

- `go test ./common/node ./scannode/...`
- `go test ./common/node ./common/node/cmd ./common/yak/cmd/yakcmds ./scannode/...`
- `go vet ./common/node ./scannode/...`
- `go vet ./common/node ./common/node/cmd ./common/yak/cmd/yakcmds ./scannode/...`
- 新增并通过了 `scannode` 侧桥接单测，覆盖：
  - `dispatch` 基础校验
  - `cancel` 到本地任务取消
  - `capability.apply` 落盘与非法 key 校验
  - 事件 metadata 注入
  - 进度单位换算
  - subject/context 辅助函数
- 旧 Notify/feedback 结果面清理后仍保持编译与单测通过
- 真实节点到平台链路已再次验证成功：
  - 节点：`legion-e2e-node-01`
  - 节点 session：`0eaddefe-2cbf-4cc4-aebe-cc3261547f9b`
  - 插件：`legion-e2e-bridge`
  - job：`56460878-f237-4853-9f22-5f390f7b8ea4`
  - attempt：`3957e7ea-77fa-4089-9c67-448845bd3a10`
  - 平台投影结果包含：
    - `job.claimed`
    - `job.started`
    - 3 条 `job.progressed`
    - 2 条 `job.asset`
    - 1 条 `job.risk`
    - 1 条 `job.report`
    - `job.succeeded`
  - 节点重启到新代码后，执行日志里不再出现 `publish palm-backend scanner failed`
- 当前代码检索已经确认：
  - `common/node` / `scannode` 中不再存在 `Notify`、`feedback`、`SCAN_ScanFingerprint`、`SCAN_ProxyCollector` 的实现与注册

当前还有一个明确暴露出来的历史问题：

- `go test -race ./scannode/...` 会失败
- 失败栈不在这轮新桥接代码，而是在：
  - `common/fuzztagx/parser/generator.go`
  - `common/mutate/fuzztag.go`
  - `common/mutate/fuzztag_argument.go`

这说明节点桥接代码本轮没有新增明显的编译或 vet 问题，但仓库里仍然存在更底层的并发竞态，后面如果要把 race 作为门禁，需要单独清掉这批历史问题。
