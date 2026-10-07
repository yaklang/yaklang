# 辅助请求的后续审计

自动记忆从 Timeline 压缩的 memory_entities 可靠保存；正常任务收尾默认有界等待 120 秒。主循环和 Coordinator 不再按迭代自动抽取记忆。`CreateAIMemoryEntity` 的手动抽取及记忆管理接口保留。

工具只接受主循环提供的显式参数；Schema 加载不执行工具。构参失败回到所属循环修正，Forge 蓝图参数生成独立保留。

仍可逐项评估：

- 统一辅助请求观测口径：逻辑操作、provider 尝试、重试、协议错误、成功应用、取消、缓存及实际模型；不能仅靠可能丢失的消费事件计数。
- 评估价值反馈、工具说明、会话标题的必要频率、复用、缓存、输入上限和有界生命周期。
- 按场景审计人工记忆去重、附件观察、风险审批和知识检索；未触发某能力不能作为零开销证明。
- 性能比较固定输入、模型、provider 和观测方式，同时衡量正确性、未缓存 token、请求数量、延迟及有效产出。

当前实现与使用方式见 [Coordinator](README.md)、[接口契约](../coordinator_interface_contract.md) 和 [Timeline](../aicommon/timeline.go)。
