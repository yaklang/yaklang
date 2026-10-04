# coordinator_legacy：旧 PLAN 引擎

这个目录整包保留旧引擎的实现和专属资源，不删除旧执行代码。包外生产代码和测试不再导入本包或其子包。ReAct / gRPC PLAN、Forge、Yak 工厂、扫描入口与任务观测均使用新版，旧专注入口不再开放，历史旧计划禁止继续执行。`aid` 根包不再代理旧构造器或提供类型别名。

## 包内职责

- `Coordinator`、`AiTask`、任务树/DAG、调度 runtime、规划、审阅、重做、报告和恢复。
- 旧上下文构造、任务摘要、Timeline fork/merge、计划状态持久化及运行快照适配。
- `detached_plan.go`：旧审批任务树的安全解码及 Timeline 展示；客户端数据先解码为普通 DTO，再由 Coordinator 初始化可执行任务。
- `loop_plan`、`loop_task`：仅供旧 PLAN 使用的规划循环和执行任务初始化。
- `promptloader/data`：旧专属提示词与响应 schema，直接随包嵌入。新旧共用的审批字段 schema 仍在 `aicommon/promptloader`。
- `integration`、`cmd`：旧集成测试和命令行示例。

```go
import "github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"

cod, err := coordinator_legacy.NewCoordinatorContext(ctx, query, options...)
if err != nil { return err }
return cod.Run()
```

## 包外隔离边界

旧 ReAct 宿主适配已删除，旧执行机制仅保留在本目录。公共任务观测接收新版 `coordinator.CollectPlanExecutionSnapshots` 的结果，不读取旧任务或旧注册表。

Yak 导出名、前端事件和数据库结构保留原有契约；必要适配由新版或公共边界承担，不构造旧对象。旧 focus 名称只映射到新版，旧数据库运行快照继续返回明确拒绝恢复错误。旧 gRPC `StartAITask`、`StartAITriage` 已停用，保留协议签名并立即返回过时错误，执行统一使用 `StartAIReAct`。

新增 PLAN 行为应实现于兄弟目录 `coordinator`，不往公共层添加旧版兼容别名，也不从包外导入本包以获取测试夹具、常量或格式化函数。旧库自身的测试和 `cmd` 示例仍可在本目录内显式调用旧实现，正式入口不加载旧循环注册。

审批校验和执行入口统一使用 `BuildRootTaskFromPlanData`，同时接受模型的 `@action: plan` 和前端/存储的 `name/subtasks` 任务树。不能只在发布或入队时转换：detached 确认默认读取已保存的任务树，队列实际执行时也必须能初始化它。

## 验证

```sh
go test ./common/ai/aid/coordinator_legacy ./common/ai/aid/coordinator_legacy/loop_plan ./common/ai/aid/coordinator_legacy/loop_task
go test ./common/ai/aid/coordinator_legacy/integration
```

当前默认入口的审核、执行和 gRPC 回归位于新版 [coordinator](../coordinator/README.md)，使用真实队列和执行器，检查两个依赖任务完成及原连接继续可用。

正式 CI 不再运行本目录测试。两项新版 Forge 持久上下文回归已迁到 `common/ai/aiforge/forge_prompt_markers_test.go`，继续交叉验证 function call / text stream；包外 detached 回归直接验证新版发布和快照。新引擎的 Yak/aim 冒烟在 [aismoking](../../aismoking/README.md)，仅供本地开发运行。
