# AI Forge

本包维护多步 Forge 蓝图、注册、模板、参数绑定与结果格式化，底层计划和任务执行由 `common/ai/aid/coordinator` 负责。`ForgeExecution` 保留 Forge 的调用边界；规划、一次审核、DAG 自动调度、任务验收、session Evidence 与交付使用新版实现。

单步结构化输出与应用在 [LiteForge](../aid/liteforge/README.md) 和其 `liteforgeapp` 子包；本包不承载 LiteForge 应用实现。

完整迁移契约见 [LEGACY_INTERFACE.md](../aid/coordinator/LEGACY_INTERFACE.md)。Yak 模块名 `aiagent` 保持不变，Go 导入路径为 `github.com/yaklang/yaklang/common/ai/aiforge`。

验证从仓库根目录启动：

```text
yak common/ai/aismoking/forge.yak
yak common/ai/aismoking/run.yak
```

[统一 AI 冒烟](../aismoking/README.md) 覆盖生成/preset/mocker、双协议、批准、依赖任务、Evidence、一次格式化、结果回调及重复 Run 幂等。生产蓝图仍位于 `buildinforge/`，不是测试脚本。
