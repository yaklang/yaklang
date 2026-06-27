# Controller ReAct Extra 注入清单（验收对照表）

每个 `phase1_ctrl_verify_<Controller>` 子 ReAct 在 **PersistentInstruction** 中应包含下列块；动态块在每轮 **ReactiveData** 中刷新。

| # | 块 ID | 来源函数/常量 | 是否截断 | 验收要点 |
|---|--------|---------------|----------|----------|
| 1 | `playbook` | `phase1_controller_verify_playbook.txt` | 否 | 任务为单 Controller；必须先发包或 mark rejected |
| 2 | `controller_task` | `buildControllerVerifyTaskBlock` | 否 | 含 `controller_file`、`feature_id`、静态 hint 表 |
| 3 | `auth_surface_map` | `embeddedArtifactsForAgent` | ≤6000 字符 | `ssa_discovery/auth_surface_map.json` |
| 4 | `auth_calibration` | `embeddedArtifactsForAgent` | ≤6000 字符 | `ssa_discovery/auth_calibration.json` |
| 5 | `failure_semantics` | `embeddedArtifactsForAgent` | ≤6000 字符 | `ssa_discovery/failure_semantics.json` |
| 6 | `routing_profile` | `embeddedArtifactsForAgent` | ≤6000 字符 | `ssa_discovery/routing_profile.json` |
| 7 | `probe_context` | `buildPhase1VerifyEmbeddedContext` | 部分截断 | auth_evidence、routing、credentials、multi_auth |
| 8 | `user_credential_groups` | `FormatUserCredentialGroupsInstruction` | 否 | 用户凭证组轮换说明 |
| 9 | `fs_tool_params` | `ssaDiscoveryFSBuiltinToolParamsHint` | 否 | read_file 参数名必须为 `file` |
| 10 | `http_tool_params` | `ssaDiscoveryHTTPBuiltinToolParamsHint` | 否 | do_http_request、auth_credential_id |

## 每轮 ReactiveData（`WithReactiveDataBuilder`）

| 字段 | 来源 | 验收要点 |
|------|------|----------|
| `session` | `discovery_session_uuid` | 与当前 discovery session 一致 |
| `target_base` | `EffectiveTargetBaseURL` | 靶场 base URL |
| `code_root` | `Session.CodeRootPath` | **完整代码根目录**（可 read 任意项目内文件） |
| `target_reachable` | `Session.TargetReachable` | 可达时 verified 须含 probe |
| `controller_file` | loop 变量 | 本任务主 Controller 相对路径 |
| `feedback` | 上轮动作结果 | 非空时含 probe/validation 提示 |

## Loop 运行时变量（`WithInitTask` 写入）

| Key | 值 | 验收要点 |
|-----|-----|----------|
| `discovery_session_uuid` | session UUID | |
| `discovery_sqlite_path` | sqlite 路径 | |
| `discovery_code_root` | code root | 与 ReactiveData 一致 |
| `discovery_controller_scope_json` | ControllerVerifyScope JSON | 读库按 controller 过滤 |
| `discovery_controller_file` | controller 相对路径 | 与 ReactiveData 一致 |

## 验收文件路径

- 清单本文：`prompts/phase1_controller_verify_extra_checklist.md`
- 代码常量：`ControllerVerifyExtraManifest`（`phase1_controller_verify_extra.go`）
- 进度文件：`ssa_discovery/controller_verify_progress.json`
- 子任务目录：`task_*-phase1_ctrl_verify_<ControllerName>/`

## 并发生产配置

| 环境变量 | 默认 | 说明 |
|----------|------|------|
| `YAK_SSA_API_DISCOVERY_CONTROLLER_VERIFY_CONCURRENT` | `4` | 同时运行的 Controller ReAct 数 |
| `YAK_SSA_API_DISCOVERY_FEATURE_BATCH_SIZE` | `8` | 每批 Feature job 数（CoverageSignal 间隔） |
