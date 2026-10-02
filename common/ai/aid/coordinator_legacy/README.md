# coordinator_legacy：旧 PLAN 引擎

这个目录集中保存旧引擎的实现和专属资源，供后续按模块移除。ReAct / gRPC PLAN 已默认使用新版，旧专注入口不再开放，历史旧计划禁止继续执行。`aid` 根包不再代理旧构造器或提供类型别名；新 `coordinator` 不依赖此包。

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

## 宿主适配和移除边界

[aireact/coordinator_legacy.go](../aireact/coordinator_legacy.go) 暂存旧 ReAct 宿主适配，供后续清理；当前 detached、恢复、PLAN-only 路由均不调用它。旧执行机制归本目录所有。公共任务观测接收 `CollectPlanExecutionSnapshots` 的结果，不依赖具体旧任务类型。

`aiforge`、`aireducer`、`yak/aiagent`、部分 gRPC 和扫描入口目前仍显式调用旧引擎。它们的 Go import 已改为 `coordinator_legacy`；Yak 导出名、前端事件、数据库结构没有随目录迁移改名。

未来移除旧版时，需要先迁移这些显式调用方和旧历史数据恢复入口，再删除宿主旧适配、`reactinit` 中旧循环的注册和本目录。不能只删除目录而保留调用方。新增 PLAN 行为应实现于兄弟目录 `coordinator`，不往公共层添加旧版兼容别名。

审批校验和执行入口统一使用 `BuildRootTaskFromPlanData`，同时接受模型的 `@action: plan` 和前端/存储的 `name/subtasks` 任务树。不能只在发布或入队时转换：detached 确认默认读取已保存的任务树，队列实际执行时也必须能初始化它。

## 验证

```sh
go test ./common/ai/aid/coordinator_legacy ./common/ai/aid/coordinator_legacy/loop_plan ./common/ai/aid/coordinator_legacy/loop_task
go test ./common/ai/aid/coordinator_legacy/integration
```

当前默认入口的审核、执行和 gRPC 回归位于新版 [coordinator](../coordinator/README.md)，使用真实队列和执行器，检查两个依赖任务完成及原连接继续可用。

CI 的旧集成测试分组已跟随迁移到 `coordinator_legacy/integration`。新引擎的 Yak/aim 原生函数调用冒烟在 [coordinator](../coordinator/README.md)，不经过旧状态机。
