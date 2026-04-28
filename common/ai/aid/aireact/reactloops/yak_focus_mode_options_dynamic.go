package reactloops

import (
	"bytes"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 此文件负责把 Yak 专注模式中"动态"的 focusXxx 钩子函数桥接成 ReActLoopOption。
// 这些钩子运行时才被调用，所有调用都通过 FocusModeYakHookCaller 进入 yak engine
// 并接受 ctx 超时 + panic 捕获。
//
// 关键词: yak focus mode dynamic hooks, focusXxx bridge, MITM-style callbacks

// CollectFocusModeDynamicOptions 读取 caller 中已注册的 focusXxx 钩子函数，
// 转换为对应的 ReActLoopOption 列表。caller 应在整个 loop 生命周期内存活。
//
// 注意：动态钩子的优先级高于同名静态 dunder（例如 focusAllowRAG > __ALLOW_RAG__）。
// 调用方可以先 append 静态选项再 append 动态选项以覆盖。
//
// 关键词: dynamic hook bridging, focusXxx to With*Option
func CollectFocusModeDynamicOptions(caller *FocusModeYakHookCaller) []ReActLoopOption {
	if caller == nil {
		return nil
	}
	var opts []ReActLoopOption

	// ---- 任务初始化与生命周期 ----
	if caller.HasHook(FocusHook_InitTask) {
		opts = append(opts, WithInitTask(func(loop *ReActLoop, task aicommon.AIStatefulTask, operator *InitTaskOperator) {
			callFocusHookSafe(caller, FocusHook_InitTask, loop, task, operator)
		}))
	}

	if caller.HasHook(FocusHook_PostIteration) {
		opts = append(opts, WithOnPostIteraction(func(loop *ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator) {
			callFocusHookSafe(caller, FocusHook_PostIteration, loop, iteration, task, isDone, reason, operator)
		}))
	}

	if caller.HasHook(FocusHook_OnTaskCreated) {
		opts = append(opts, WithOnTaskCreated(func(task aicommon.AIStatefulTask) {
			callFocusHookSafe(caller, FocusHook_OnTaskCreated, task)
		}))
	}

	if caller.HasHook(FocusHook_OnAsyncTaskTrigger) {
		opts = append(opts, WithOnAsyncTaskTrigger(func(action *LoopAction, task aicommon.AIStatefulTask) {
			callFocusHookSafe(caller, FocusHook_OnAsyncTaskTrigger, action, task)
		}))
	}

	if caller.HasHook(FocusHook_OnAsyncTaskFinished) {
		opts = append(opts, WithOnAsyncTaskFinished(func(task aicommon.AIStatefulTask) {
			callFocusHookSafe(caller, FocusHook_OnAsyncTaskFinished, task)
		}))
	}

	// ---- prompt / context 提供器 ----
	if caller.HasHook(FocusHook_PromptGenerator) {
		opts = append(opts, WithLoopPromptGenerator(func(userInput string, contextResult string, contextFeedback string) (string, error) {
			ret, err := caller.CallByName(FocusHook_PromptGenerator, userInput, contextResult, contextFeedback)
			if err != nil {
				return "", err
			}
			return utils.InterfaceToString(ret), nil
		}))
	}

	if caller.HasHook(FocusHook_PersistentContext) {
		opts = append(opts, WithPersistentContextProvider(func(loop *ReActLoop, nonce string) (string, error) {
			ret, err := caller.CallByName(FocusHook_PersistentContext, loop, nonce)
			if err != nil {
				return "", err
			}
			return utils.InterfaceToString(ret), nil
		}))
	}

	if caller.HasHook(FocusHook_ReflectionOutputExample) {
		opts = append(opts, WithReflectionOutputExampleContextProvider(func(loop *ReActLoop, nonce string) (string, error) {
			ret, err := caller.CallByName(FocusHook_ReflectionOutputExample, loop, nonce)
			if err != nil {
				return "", err
			}
			return utils.InterfaceToString(ret), nil
		}))
	}

	if caller.HasHook(FocusHook_ReactiveData) {
		opts = append(opts, WithReactiveDataBuilder(func(loop *ReActLoop, feedback *bytes.Buffer, nonce string) (string, error) {
			ret, err := caller.CallByName(FocusHook_ReactiveData, loop, feedback, nonce)
			if err != nil {
				return "", err
			}
			return utils.InterfaceToString(ret), nil
		}))
	}

	// ---- action 过滤器 ----
	if caller.HasHook(FocusHook_ActionFilter) {
		opts = append(opts, WithActionFilter(func(action *LoopAction) bool {
			ret, err := caller.CallByName(FocusHook_ActionFilter, action)
			if err != nil {
				log.Warnf("yak focus mode action filter failed: %v", err)
				return true
			}
			return utils.InterfaceToBoolean(ret)
		}))
	}

	// ---- 动态权限 getter，覆盖静态同名 dunder ----
	if caller.HasHook(FocusHook_AllowRAG) {
		opts = append(opts, WithAllowRAGGetter(makeBoolGetter(caller, FocusHook_AllowRAG)))
	}
	if caller.HasHook(FocusHook_AllowAIForge) {
		opts = append(opts, WithAllowAIForgeGetter(makeBoolGetter(caller, FocusHook_AllowAIForge)))
	}
	if caller.HasHook(FocusHook_AllowPlanAndExec) {
		opts = append(opts, WithAllowPlanAndExecGetter(makeBoolGetter(caller, FocusHook_AllowPlanAndExec)))
	}
	if caller.HasHook(FocusHook_AllowToolCall) {
		opts = append(opts, WithAllowToolCallGetter(makeBoolGetter(caller, FocusHook_AllowToolCall)))
	}
	if caller.HasHook(FocusHook_AllowUserInteract) {
		opts = append(opts, WithUserInteractGetter(makeBoolGetter(caller, FocusHook_AllowUserInteract)))
	}

	return opts
}

// callFocusHookSafe 调用一个 focus hook，错误打印 log 但不向上传播
// （生命周期类 hook 的调用方一般是 ReActLoop 内部的 ForEach 风格，不接受 error）
func callFocusHookSafe(caller *FocusModeYakHookCaller, name string, args ...interface{}) {
	if caller == nil {
		return
	}
	_, err := caller.CallByName(name, args...)
	if err != nil {
		log.Errorf("yak focus mode hook %q failed: %v", name, err)
	}
}

// makeBoolGetter 构造一个返回 bool 的 getter 闭包，调用 caller.CallByName。
// 出错时回退为 false。
func makeBoolGetter(caller *FocusModeYakHookCaller, name string) func() bool {
	return func() bool {
		if caller == nil {
			return false
		}
		ret, err := caller.CallByName(name)
		if err != nil {
			log.Warnf("yak focus mode getter %q failed: %v", name, err)
			return false
		}
		return utils.InterfaceToBoolean(ret)
	}
}
