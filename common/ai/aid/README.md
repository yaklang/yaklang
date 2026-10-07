# AI 运行基础设施与 PLAN 引擎

PLAN 统一使用 `coordinator.Session`，`aid` 根包提供公共工具搜索选项、Forge 注册入口和任务观测 DTO。

```text
aid/
├── coordinator/          Session、Controller、coordinator / pe_task 循环
├── aicommon/             Config、Timeline、evidence、模型及事件公共契约
├── aireact/              会话宿主、通用 ReAct 基础设施
├── liteforge/            单次结构化请求与 liteforgeapp 应用
└── aitool/               共享工具与显式参数执行设施
```

Go 入口是 `coordinator.NewSession`；ReAct 的 PLAN 入口在 [aireact/coordinator.go](aireact/coordinator.go)。未指定 focus 时由默认循环转交 coordinator。客户端历史 focus 名称在外层归一化；不具备新版 snapshot 的旧计划记录拒绝执行，需要重新生成计划。Yakit 事件、RPC 和数据库字段保持原有契约。

公共任务观测通过 `BuildTaskRuntimeReport` 聚合 `coordinator.CollectPlanExecutionSnapshots` 提供的快照；ReAct、任务看板和 `/live` 读取同一运行状态。

回归测试位于 `coordinator`、`aireact` 和 `common/ai/aiforge`。模块说明见 [coordinator](coordinator/README.md)，架构和前端协议分别见 [架构文档](coordinator_architecture.md)、[接口契约](coordinator_interface_contract.md)。
