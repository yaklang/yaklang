# AI 运行基础设施与 PLAN 引擎

新版 PLAN 与保留的旧源码位于同层目录。`coordinator_legacy` 整包保留，包外生产代码和测试均不导入它或其子包。`aid` 根包只保留公共工具搜索选项、Forge 注册入口和任务观测 DTO，不再导出旧 `Coordinator`、`AiTask` 或它们的兼容别名。

```text
aid/
├── coordinator/          新版 Session、Controller、coordinator / pe_task 循环
├── coordinator_legacy/   旧 Coordinator、AiTask、PLAN 状态机和恢复
│   ├── loop_plan/        旧版规划专注循环
│   ├── loop_task/        旧版 pe_task 工厂与任务初始化
│   ├── promptloader/    旧引擎专用提示词和响应 schema
│   ├── integration/     旧引擎集成测试
│   └── cmd/             旧版命令行示例
├── aicommon/             Config、Timeline、evidence、模型及事件公共契约
├── aireact/              会话宿主、入口分流、通用 ReAct 基础设施
└── aitool/               共享工具与执行设施
```

新 Go 入口是 `coordinator.NewSession`，旧入口是 `coordinator_legacy.NewCoordinatorContext`。两者没有互相引用或隐式回退；ReAct 的 PLAN 入口在 [aireact/coordinator.go](aireact/coordinator.go) 统一使用新版。未指定 focus 时由默认循环转交 coordinator，旧 `plan` / `coordinator_legacy` focus 名称也转入新版，旧记录拒绝执行。Yakit 事件、RPC 和数据库字段保持原有契约，显式调用旧 Go 库的其他调用方单独迁移。

公共任务观测通过 `BuildTaskRuntimeReport` 聚合新版 `coordinator.CollectPlanExecutionSnapshots` 提供的快照；ReAct、任务看板和 `/live` 都读取新版状态，不访问旧引擎的任务或全局注册表。

模块职责及隔离边界见 [新版说明](coordinator/README.md)、[旧版说明](coordinator_legacy/README.md)。旧包测试可在旧目录内单独运行，正式 CI 不再收集它们；新版回归位于 `coordinator`、`aireact` 和 `common/ai/aiforge`。架构和前端协议分别见 [架构文档](coordinator_architecture.md)、[接口契约](coordinator_interface_contract.md)。
