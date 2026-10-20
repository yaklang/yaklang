# 知识增强模式验证

`knowledge_enhance` 保留专注模式入口，复用普通通用 loop，首次加载 `yaklang-system` 和已启用的知识库工具 Schema。Skill 与工具使用普通可淘汰缓存；用户附件中的精确库名和选择偏好会进入上下文。

从仓库根目录运行维护中的冒烟入口：

```text
AISMOKING_CASE=knowledge_base_management yak common/ai/aismoking/run.yak
AISMOKING_CASE=knowledge_callers yak common/ai/aismoking/run.yak
AISMOKING_CASE=knowledge_plan yak common/ai/aismoking/run.yak
```

这些入口模拟 AI 决策和 embedding，实际执行 Skill 加载、工具 Schema 加载、库及条目的增删改查、文件构建、预算查询与清理。Plan 案例包含三个依赖 worker、稳定条目 ID、共享 Evidence、报告和最终数据库状态检查。

真实模型入口为 `common/ai/aismoking/live/knowledge_natural.yak`、`live/knowledge_callers.yak` 和 `live/knowledge_plan.yak`。指定新的 `AISMOKING_OUTPUT` 后运行，使用当前配置的 AI/embedding，仅操作随机命名的自有测试库，退出时清理。详细参数与断言见 `common/ai/aismoking/README.md`。
