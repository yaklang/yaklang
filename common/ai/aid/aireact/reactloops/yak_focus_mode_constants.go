package reactloops

// 此文件统一收口 Yak 专注模式（Yak Focus Mode）所需的所有约定常量名。
// 关键词: yak focus mode, dunder constants, focus hooks, ai-focus.yak
//
// 设计目标：
//   - 把 Go 端可配置的 ReActLoop 能力以 "声明式 dunder + 行为式 focusXxx 钩子" 的
//     形式暴露给 Yak 脚本作者，确保 Yak 写出的专注模式 **不被阉割**。
//   - 静态可序列化的配置走 __DUNDER__ 大写常量；运行期需要闭包/数据流的能力走
//     focusXxx 风格的 hook 函数；自定义 Action 走 __ACTIONS__ 列表。
//   - 文件命名采用 *.ai-focus.yak，便于 CLI / 加载器识别主入口；同级目录中其它
//     *.yak 文件作为 sidekick 插件被自动拼接到主脚本一起执行。

// FocusModeFileSuffix 是专注模式入口文件的标准后缀。
// 例如：comprehensive_showcase.ai-focus.yak
const FocusModeFileSuffix = ".ai-focus.yak"

// FocusModeYakFileSuffix 是同目录下 sidekick 插件文件的后缀。
// 与主入口同级、且文件后缀为 .yak（但不是 .ai-focus.yak）的文件会被自动拼接进
// 主脚本，作为复用工具/工具函数。
const FocusModeYakFileSuffix = ".yak"

// ----------------------------------------------------------------------------
// 1. metadata 类 dunder：对应 LoopMetadataOption（boot 期读取，注入到注册表）
// ----------------------------------------------------------------------------

const (
	// FocusDunder_Name 显式指定专注模式名（loops 注册表中的 key）。
	// 若不设置，则取 *.ai-focus.yak 文件名（去掉后缀）作为默认名称。
	FocusDunder_Name = "__NAME__"

	// FocusDunder_Description 英文功能描述，对应 WithLoopDescription。
	FocusDunder_Description = "__DESCRIPTION__"

	// FocusDunder_DescriptionZh 中文功能描述，对应 WithLoopDescriptionZh。
	FocusDunder_DescriptionZh = "__DESCRIPTION_ZH__"

	// FocusDunder_VerboseName 英文展示名，对应 WithVerboseName。
	FocusDunder_VerboseName = "__VERBOSE_NAME__"

	// FocusDunder_VerboseNameZh 中文展示名，对应 WithVerboseNameZh。
	FocusDunder_VerboseNameZh = "__VERBOSE_NAME_ZH__"

	// FocusDunder_IsHidden 是否对用户隐藏，对应 WithLoopIsHidden。
	FocusDunder_IsHidden = "__IS_HIDDEN__"

	// FocusDunder_OutputExample 示例输出，对应 WithLoopOutputExample。
	FocusDunder_OutputExample = "__OUTPUT_EXAMPLE__"

	// FocusDunder_UsagePrompt schema 使用说明，对应 WithLoopUsagePrompt。
	FocusDunder_UsagePrompt = "__USAGE_PROMPT__"
)

// ----------------------------------------------------------------------------
// 2. 静态配置类 dunder：在 ReActLoop 创建时直接 With* 进去
// ----------------------------------------------------------------------------

const (
	// FocusDunder_MaxIterations 最大迭代次数，对应 WithMaxIterations。
	FocusDunder_MaxIterations = "__MAX_ITERATIONS__"

	// FocusDunder_MemorySizeLimit 内存上限（字节），对应 WithMemorySizeLimit。
	FocusDunder_MemorySizeLimit = "__MEMORY_SIZE_LIMIT__"

	// FocusDunder_PeriodicVerificationInterval 周期性 verification 触发间隔，
	// 对应 WithPeriodicVerificationInterval。
	FocusDunder_PeriodicVerificationInterval = "__PERIODIC_VERIFICATION_INTERVAL__"

	// FocusDunder_SameActionTypeSpinThreshold 相同 Action 类型自旋阈值，
	// 对应 WithSameActionTypeSpinThreshold。
	FocusDunder_SameActionTypeSpinThreshold = "__SAME_ACTION_TYPE_SPIN_THRESHOLD__"

	// FocusDunder_SameLogicSpinThreshold 相同逻辑自旋阈值，
	// 对应 WithSameLogicSpinThreshold。
	FocusDunder_SameLogicSpinThreshold = "__SAME_LOGIC_SPIN_THRESHOLD__"

	// FocusDunder_MaxConsecutiveSpinWarnings 连续 spin 警告上限，
	// 对应 WithMaxConsecutiveSpinWarnings。
	FocusDunder_MaxConsecutiveSpinWarnings = "__MAX_CONSECUTIVE_SPIN_WARNINGS__"

	// FocusDunder_AllowRAG / AIForge / PlanAndExec / ToolCall / UserInteract 静态开关。
	// 对应 WithAllowRAG / WithAllowAIForge / WithAllowPlanAndExec /
	// WithAllowToolCall / WithAllowUserInteract。
	FocusDunder_AllowRAG          = "__ALLOW_RAG__"
	FocusDunder_AllowAIForge      = "__ALLOW_AI_FORGE__"
	FocusDunder_AllowPlanAndExec  = "__ALLOW_PLAN_AND_EXEC__"
	FocusDunder_AllowToolCall     = "__ALLOW_TOOL_CALL__"
	FocusDunder_AllowUserInteract = "__ALLOW_USER_INTERACT__"

	// FocusDunder_UseSpeedPriorityAI 是否使用 Speed Priority AI 回调，
	// 对应 WithUseSpeedPriorityAICallback。
	FocusDunder_UseSpeedPriorityAI = "__USE_SPEED_PRIORITY_AI__"

	// FocusDunder_EnableSelfReflection 是否开启自我反思，
	// 对应 WithEnableSelfReflection。
	FocusDunder_EnableSelfReflection = "__ENABLE_SELF_REFLECTION__"

	// FocusDunder_DisableLoopPerception 是否禁用 perception 层，
	// 对应 WithDisableLoopPerception。
	FocusDunder_DisableLoopPerception = "__DISABLE_LOOP_PERCEPTION__"

	// FocusDunder_NoEndLoadingStatus 对应 WithNoEndLoadingStatus。
	FocusDunder_NoEndLoadingStatus = "__NO_END_LOADING_STATUS__"

	// FocusDunder_PersistentInstruction 持久指令模板字符串，对应
	// WithPersistentInstruction（自动按 RenderTemplate 渲染）。
	FocusDunder_PersistentInstruction = "__PERSISTENT_INSTRUCTION__"

	// FocusDunder_ReflectionOutputExample 反思输出示例（与 OutputExamplePrompt
	// 不同，这里是 ReActLoop 实例级的渲染模板），对应 WithReflectionOutputExample。
	FocusDunder_ReflectionOutputExample = "__REFLECTION_OUTPUT_EXAMPLE__"

	// FocusDunder_ToolCallIntervalReviewExtraPrompt 对应
	// WithToolCallIntervalReviewExtraPrompt。
	FocusDunder_ToolCallIntervalReviewExtraPrompt = "__TOOL_CALL_INTERVAL_REVIEW_EXTRA_PROMPT__"

	// FocusDunder_Vars Yak 字典 (map[string]any)，会被批量塞入 ReActLoop.vars，
	// 对应 WithVars。
	FocusDunder_Vars = "__VARS__"

	// FocusDunder_AITagFields 列表，元素为 dict：
	//   {"tag": "...", "var": "...", "node_id": "...", "content_type": "..."}
	// 自动转化为 WithAITagField / WithAITagFieldWithAINodeId。
	FocusDunder_AITagFields = "__AI_TAG_FIELDS__"
)

// ----------------------------------------------------------------------------
// 3. Action 注册类 dunder
// ----------------------------------------------------------------------------

const (
	// FocusDunder_Actions 自定义 Action 列表（dict 数组），每条形如：
	//   {
	//     "type":       "scan_target",
	//     "description": "扫描目标",
	//     "options":    [...aitool option desc...],
	//     "stream_fields":  [...],
	//     "output_examples": "...",
	//     "verifier":   func(loop, action) { ... },
	//     "handler":    func(loop, action, operator) { ... },
	//     "async":      false,
	//   }
	// 对应 WithRegisterLoopAction / WithRegisterLoopActionWithStreamField。
	FocusDunder_Actions = "__ACTIONS__"

	// FocusDunder_OverrideActions 与 __ACTIONS__ 同结构，但作用是替换内置/已有
	// 同名 Action（例如自定义 directly_answer 的校验）。
	// 对应 WithOverrideLoopAction。
	FocusDunder_OverrideActions = "__OVERRIDE_ACTIONS__"

	// FocusDunder_ActionsFromTools 字符串列表，元素为本地工具名，
	// 由 invoker 提供工具表（运行期），转换为 LoopAction。
	// 对应 WithRegisterLoopActionFromTool。
	FocusDunder_ActionsFromTools = "__ACTIONS_FROM_TOOLS__"

	// FocusDunder_ActionsFromLoops 字符串列表，元素是已注册 loop 的名称，
	// 把它包装成可调用的 sub-loop action。
	// 对应 WithActionFactoryFromLoop。
	FocusDunder_ActionsFromLoops = "__ACTIONS_FROM_LOOPS__"
)

// ----------------------------------------------------------------------------
// 4. 动态运行期钩子 focusXxx：闭包/与 loop 实例强绑定
// ----------------------------------------------------------------------------

const (
	// FocusHook_InitTask 对应 WithInitTask；签名：
	//   func focusInitTask(loop, task, operator)
	FocusHook_InitTask = "focusInitTask"

	// FocusHook_PostIteration 对应 WithOnPostIteraction；签名：
	//   func focusPostIteration(loop, iteration, task, isDone, reason, operator)
	FocusHook_PostIteration = "focusPostIteration"

	// FocusHook_OnTaskCreated 对应 WithOnTaskCreated；签名：
	//   func focusOnTaskCreated(task)
	FocusHook_OnTaskCreated = "focusOnTaskCreated"

	// FocusHook_OnAsyncTaskTrigger 对应 WithOnAsyncTaskTrigger；签名：
	//   func focusOnAsyncTaskTrigger(action, task)
	FocusHook_OnAsyncTaskTrigger = "focusOnAsyncTaskTrigger"

	// FocusHook_OnAsyncTaskFinished 对应 WithOnAsyncTaskFinished；签名：
	//   func focusOnAsyncTaskFinished(task)
	FocusHook_OnAsyncTaskFinished = "focusOnAsyncTaskFinished"

	// FocusHook_PromptGenerator 对应 WithLoopPromptGenerator；签名：
	//   func focusGeneratePrompt(userInput, contextResult, contextFeedback) -> string
	FocusHook_PromptGenerator = "focusGeneratePrompt"

	// FocusHook_PersistentContext 对应 WithPersistentContextProvider；签名：
	//   func focusPersistentContext(loop, nonce) -> string
	// 优先级高于 __PERSISTENT_INSTRUCTION__ 静态字符串。
	FocusHook_PersistentContext = "focusPersistentContext"

	// FocusHook_ReflectionOutputExample 对应
	// WithReflectionOutputExampleContextProvider；签名：
	//   func focusReflectionOutputExample(loop, nonce) -> string
	// 优先级高于 __REFLECTION_OUTPUT_EXAMPLE__ 静态字符串。
	FocusHook_ReflectionOutputExample = "focusReflectionOutputExample"

	// FocusHook_ReactiveData 对应 WithReactiveDataBuilder；签名：
	//   func focusReactiveData(loop, feedback, nonce) -> string
	FocusHook_ReactiveData = "focusReactiveData"

	// FocusHook_ActionFilter 对应 WithActionFilter；签名：
	//   func focusActionFilter(action) -> bool
	FocusHook_ActionFilter = "focusActionFilter"

	// FocusHook_AllowRAG / AIForge / PlanAndExec / ToolCall / UserInteract
	// 动态版本（getter），优先级高于同名静态 dunder。签名均为：
	//   func focusAllowXxx() -> bool
	FocusHook_AllowRAG          = "focusAllowRAG"
	FocusHook_AllowAIForge      = "focusAllowAIForge"
	FocusHook_AllowPlanAndExec  = "focusAllowPlanAndExec"
	FocusHook_AllowToolCall     = "focusAllowToolCall"
	FocusHook_AllowUserInteract = "focusAllowUserInteract"
)
