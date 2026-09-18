# 非阻塞子Agent：派发、观察和收尾

## 用户能得到什么

上下文继承及各类退出条件的逐项复核见[SubAgent上下文与退出控制复核](21-subagent-context-and-exit-audit.md)。
每个子任务的可选上下文策略见[上下文策略实现](22-subagent-context-policy.md)。
可见汇报、等待计时与一次真实会话的事件复核见[状态事件复核](23-subagent-status-event-audit.md)。

“让两个子Agent分析支付、库存日志，你检查部署配置。”

启用多Agent模式后，dispatch立即返回两个job ID；父模型下一轮就能读取部署配置。支付先完成，其摘要与原文引用先进入父下一轮输入，不必等待库存。父没有独立工作时可以选择wait，最多等30秒再根据进度决定下一步。

30秒是一次观察期限，不是子任务的执行期限。不会自动每30秒调用模型，也不会因观察超时取消子任务。

## 用一个完整案例理解改动

这里的ReAct是“模型选择动作 → 程序执行动作 → 把结果交给模型 → 再选择”的循环；父Agent是接收用户任务的运行体，子Agent是有独立任务身份和历史的另一个循环。timeline是模型执行历史，emitter是给客户端发送事件的通道，两者用途不同。

用户输入：“支付和库存分别派子Agent分析，你检查部署配置，最后综合说明故障原因。”以下文件和结论是控制流示例，不代表真实故障分析结果。

1. **用户提交任务。** `StartAIReAct`通过session runtime接收自由输入，`handleFreeValue`创建task并排队；`processReActFromQueue`取出task，`processReActTask`选择默认loop，最终进入`ExecuteWithExistedTask`。任务保存输入和附件引用，客户端收到排队、执行事件。这部分沿用原流程，收到消息不等于模型已开始执行。
2. **父模型选择派发。** `generateLoopPrompt`把任务、历史、可用动作及上一轮反馈组装为输入。模型返回`dispatch_sub_react_agents`及两份任务书；执行器解析、验证，再调用`handleDispatchSubReactAgents`。改动前这个handler同步等全部子任务；现在它调用`SubmitSubAgents`，把配置和历史快照入队，立即向父模型反馈两项job ID和accepted回执。回执只表示接收成功，不表示分析成功。
3. **父子各自执行。** 子worker取得并发名额，创建子task、子invoker及子loop，后台扩写任务书并执行自己的“模型→工具→反馈”。父handler已经Continue，因此父模型下一轮可以选择读取部署配置。子历史写入各自timeline，子UI事件携带任务身份；父模型不会自动读取子模型的每一段流式输出。改动前用户也能收到子输出，但父循环仍停在派发调用内。
4. **父暂时没有独立工作。** 父模型选择`wait_sub_react_agents`。默认30秒内任何选中任务有未读终态便提前返回；否则返回当前状态、最近活动、轮数、工具调用数和阶段输出。比如库存仍running，父模型可以继续检查、再等或取消。旧派发没有这个中间决策机会。这里没有猜测完成百分比，也没有每30秒强制唤醒模型的后台定时器。
5. **支付先完成。** worker将完整结论写到固定产物文件，再在manager登记摘要、文件引用和递增的结果版本。父下一轮组装输入时，将新记录放入正式动态分区；组装完成后再记入父timeline。这样首次投递只有一份正文，之后又有可追溯的历史。父此时可以结合支付证据与部署配置继续判断，无需等待库存；客户端仍沿原事件通道接收输出。
6. **父收齐证据并结束。** 假设库存结果在父模型已经拿到输入之后才到达，父这轮选择finish会被拒绝：其输入版本尚未包含库存结果。下一轮收到库存摘要与文件引用后，父可读取原文、综合回答并finish。正常结束要求没有活跃子任务且终态结果已投递；显式用户停止则取消子任务并有界清理。程序只保证证据进入模型输入，不声称模型已正确理解或完成业务验收。

因此改动前后最核心的区别是：**派发动作的返回值从整批结果改为任务回执，结果通过后续循环单独送达。** 仅把原调用放进goroutine仍不够，还必须解决任务归属、进度读取、取消、结果投递及父任务结束条件。

## 模型协议

沿用现有WithEnableMultiAgentMode(true)或WithEnableDispatchSubReactAgent(true)能力开关。未启用时不注册这些动作；子Agent不能调用这些动作。原dispatch动作现在返回accepted回执，不再返回整批业务结果。

| 动作 | 参数 | 行为 |
| --- | --- | --- |
| dispatch_sub_react_agents | dispatches：goal、identifier、task_name、loop_name、result_contract、context_mode（fork默认/task_only） | 按job上下文策略准备、入队、返回batch_id和job_id及实际context_mode |
| inspect_sub_react_agents | job_ids，可省略 | 立即读取当前父任务所属子任务的值快照 |
| wait_sub_react_agents | job_ids，可省略；timeout_ms默认30000，范围0..600000（最大10分钟） | 新终态提前返回；期限到时返回observation_timeout，子任务继续 |
| cancel_sub_react_agents | job_ids，可省略 | 请求取消；直到worker退出才发布终态 |

省略job_ids表示当前父loop的全部子任务；未知或其他父任务的ID报错。相同identifier可以出现在不同批次，观察和取消必须使用回执的job_id。

状态包括queued、preparing、running、cancelling、completed、failed、cancelled、timed_out。观察返回的timed_out布尔值与任务状态timed_out不同：后者表示独立执行期限到期。

进度包含开始/结束/最近活动时间、最近事件节点、已完成轮数和工具调用计数、最近答复片段、完整结果引用及错误。它们是执行事实，不是完成百分比或结果质量评价；结果契约仍需要父模型或专业业务逻辑验收。

## 调用路径和数据归属

1. loopinfra的派发handler解析任务书，调用ReActLoop.SubmitSubAgents。
2. 父线程冻结timeline、配置值、独立SessionPromptState和结果目录。能力管理器等原有共享资源保持既有权限边界；排队子任务启动时只继承共享工具管理器的当前策略，不回写派发时的旧策略。
3. 当前loop拥有SubAgentManager；多轮、多次派发共享配额。worker拿到名额之后创建子运行体、扩写目标并执行子ReActLoop。
4. worker只更新自己的历史和manager快照。它不写父operator、vars或父timeline，不根据正在变化的父迭代号生成产物路径。
5. 子结果原文先写入固定目录，再提交终态、结果版本并通知等待者。取消确认与活跃监控注销在该worker收尾时完成。
6. 父generateLoopPrompt取出未投递结果，最多8项，放进ReactiveData动态分区；模型提示组装完成后才写父timeline，避免首次投递出现两份正文。
7. 父记录本轮模型输入覆盖的结果版本。finish既检查活跃子任务，也检查最新结果是否已进入产生该动作的模型输入。

提交回执、UI流事件、父模型输入是三个独立环节。UI收到子输出不表示父模型已经看过它。

## 生命周期约束

- 父task的正常结束会取消其context，因此有活跃子任务或未见结果时，普通finish/成功Exit被拒绝并Continue。
- 同样覆盖simple_query自动结束、post-iteration退出；在蓝图、计划和focus交接之前检查，防止其内部提前完成父task。
- 用户在工具审批中明确要求立即直接回答时，先取消并有界清理子任务，随后按用户要求答复、结束。
- 错误、父取消、迭代预算耗尽、Release都关闭owner并请求取消。
- 清理最多等待1秒；不响应context的worker仍标记cancelling/cleanup_pending，不谎报已经停止。其晚到结果只属于原manager。
- 清理回调发生panic会记录错误，继续尝试其他清理、发布终态并释放名额，避免一个回调异常打断整个进程。
- 正常结束的父任务不会因子消息自动创建新根任务；进程重启不会自动恢复这些worker。
- 新终态结果允许父在阶段答复之后补充综合答复；同一结果版本下无TODO变化的重复答复仍受原保护。

## 并发和预算

默认运行上限沿用MaxSubAgents（默认5，硬上限20）。当前owner最多20项未终结任务，总计最多提交100项；完成后保留状态和结果引用。

连续20次没有新阶段输出、终态结果或真实工作进展的inspect/wait之后，下一次无变化观察触发有界停止。真实工具执行、有效TODO变更、新子答复/终态会重置计数；取消不消耗此预算。观察动作不增加goal有效推进。

公共DispatchSubAgents仍同步返回完整结果，供审计Phase2、Phase3和fast_context使用。其内部同步执行不占用外层后台池的新名额，避免“所有父worker占满名额、再等待内部搜索名额”的死锁。首版不承诺整个递归任务树严格只有MaxSubAgents个运行者。

## 代码入口

- subagent_manager.go：后台owner、队列、观察、取消、结果版本与结束检查。
- subagent_pipeline.go / subagent_timeline.go：共用执行原语、派发快照、稳定身份、完整结果保存。
- loopinfra/dispatch_sub_react_agents.go / subagent_controls.go：模型动作。
- prompt.go / exec.go：安全投递、结束与预算检查。
- directly_answer_helper.go / tool_call_common.go：阶段答复与显式用户中断。

## 验证

新增测试使用channel控制子任务顺序，并沿真实ExecuteWithExistedTask运行模拟模型：

- 子任务尚未完成时父执行本地动作；
- 文本动作/function-call均走真实dispatch、wait、finish；
- 观察超时不取消任务，执行期限独立生效；
- 结果恰在父prompt之后到达，首次finish被拒绝；
- 原文超过4000字符仍完整保存；
- 排队取消、共享配额、派发时会话快照、固定身份；
- 排队子任务不恢复旧工具策略，清理回调panic后继续回收名额；
- 拒绝格式错误的job_ids，避免误当作取消全部；
- 内部同步搜索在外层并发1/2时正常完成；
- 显式用户结束、无变化观察预算、迟到结果隔离和cleanup_pending。

建议运行：

    go test ./common/ai/aid/aireact/reactloops ./common/ai/aid/aireact/reactloops/loopinfra -run 'TestBackgroundSubAgents|TestBackgroundDispatchActions' -count=1
    go test -race ./common/ai/aid/aireact/reactloops ./common/ai/aid/aireact/reactloops/loopinfra -run 'TestBackgroundSubAgents|TestBackgroundDispatchActions|TestProgressRegistry|TestDispatchSubAgents' -count=1

这些测试不使用真实模型服务，也不验证客户端卡片渲染。
