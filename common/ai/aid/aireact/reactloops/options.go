package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func WithLoopPromptGenerator(generator ReActLoopCoreGenerateCode) ReActLoopOption {
	return func(r *ReActLoop) {
		r.loopPromptGenerator = generator
	}
}

func WithAllowRAGGetter(allowRAG func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowRAG = allowRAG
	}
}

func WithAllowAIForgeGetter(allowAIForge func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowAIForge = allowAIForge
	}
}

func WithOnLoopInstanceCreated(fn func(loop *ReActLoop)) ReActLoopOption {
	return func(r *ReActLoop) {
		r.onLoopInstanceCreated = fn
	}
}

func WithAllowPlanAndExecGetter(allowPlanAndExec func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowPlanAndExec = allowPlanAndExec
	}
}

func WithAllowPlanAndExec(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithAllowPlanAndExecGetter(func() bool {
			return b[0]
		})
	}
	return WithAllowPlanAndExecGetter(func() bool {
		return true
	})
}

func WithAllowAIForge(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithAllowAIForgeGetter(func() bool {
			return b[0]
		})
	}
	return WithAllowAIForgeGetter(func() bool {
		return true
	})
}

func WithAllowRAG(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithAllowRAGGetter(func() bool {
			return b[0]
		})
	}
	return WithAllowRAGGetter(func() bool {
		return true
	})
}

func WithAllowToolCallGetter(allowToolCall func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowToolCall = allowToolCall
	}
}

func WithActionFilter(filter func(action *LoopAction) bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.actionFilters = append(r.actionFilters, filter)
	}
}

func WithAllowToolCall(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithAllowToolCallGetter(func() bool {
			return b[0]
		})
	}
	return WithAllowToolCallGetter(func() bool {
		return true
	})
}

func WithToolsGetter(getter func() []*aitool.Tool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.toolsGetter = getter
	}
}

func WithUserInteractGetter(allowUserInteract func() bool) ReActLoopOption {
	return func(r *ReActLoop) {
		r.allowUserInteract = allowUserInteract
	}
}

func WithAllowUserInteract(b ...bool) ReActLoopOption {
	if len(b) > 0 {
		return WithUserInteractGetter(func() bool {
			return b[0]
		})
	}
	return WithUserInteractGetter(func() bool {
		return true
	})
}

func WithRegisterLoopAction(actionName string, desc string, opts []aitool.ToolOption, verifier LoopActionVerifierFunc, handler LoopActionHandlerFunc) ReActLoopOption {
	return WithRegisterLoopActionWithStreamField(actionName, desc, opts, nil, verifier, handler)
}

// WithRegisterLoopActionFromTool converts an aitool.Tool to a LoopAction and registers it.
// This is a convenience wrapper around ConvertAIToolToLoopAction.
func WithRegisterLoopActionFromTool(tool *aitool.Tool) ReActLoopOption {
	return func(r *ReActLoop) {
		if tool == nil {
			return
		}
		name := tool.GetName()
		if r.actions.Have(name) {
			log.Errorf("loop action %s already registered", name)
			return
		}
		action := ConvertAIToolToLoopAction(tool)
		r.actions.Set(name, action)
	}
}

func WithRegisterLoopActionWithStreamField(actionName string, desc string, opts []aitool.ToolOption, fields []*LoopStreamField, verifier LoopActionVerifierFunc, handler LoopActionHandlerFunc) ReActLoopOption {
	return func(r *ReActLoop) {
		if r.actions.Have(actionName) {
			log.Errorf("loop action %s already registered", actionName)
			return
		}
		r.actions.Set(actionName, &LoopAction{
			AsyncMode:      false,
			ActionType:     actionName,
			Description:    desc,
			Options:        opts,
			ActionVerifier: verifier,
			ActionHandler:  handler,
			StreamFields:   fields,
		})
	}
}

// WithOverrideLoopAction replaces an existing loop action (e.g. to add custom validation).
// Use when a loop needs to override a built-in action like directly_answer.
func WithOverrideLoopAction(action *LoopAction) ReActLoopOption {
	return func(r *ReActLoop) {
		if action != nil {
			r.actions.Set(action.ActionType, action)
		}
	}
}

func WithMaxIterations(maxIterations int) ReActLoopOption {
	return func(r *ReActLoop) {
		r.maxIterations = maxIterations
	}
}

// WithAITagField 行为变化！！！：现在VariableName 不仅仅是在loop中get数据的key，也是tag set到action的field的key
func WithAITagField(tagName, variableName string) ReActLoopOption {
	return func(r *ReActLoop) {
		if r.aiTagFields == nil {
			r.aiTagFields = omap.NewEmptyOrderedMap[string, *LoopAITagField]()
		}
		r.aiTagFields.Set(tagName, &LoopAITagField{
			TagName:      tagName,
			VariableName: variableName,
		})
	}
}

func WithAITagFieldWithAINodeId(tagName, variableName, nodeId string, contentType ...string) ReActLoopOption {
	return func(r *ReActLoop) {
		if r.aiTagFields == nil {
			r.aiTagFields = omap.NewEmptyOrderedMap[string, *LoopAITagField]()
		}
		ct := ""
		if len(contentType) > 0 {
			ct = contentType[0]
		}
		if ct != "" {
			log.Infof("Register AITagField [%v/%v] with content type: %s", tagName, variableName, ct)
		}
		r.aiTagFields.Set(tagName, &LoopAITagField{
			TagName:      tagName,
			VariableName: variableName,
			AINodeId:     nodeId,
			ContentType:  ct,
		})
	}
}

func WithReflectionOutputExampleContextProvider(provider ContextProviderFunc) ReActLoopOption {
	return func(r *ReActLoop) {
		r.reflectionOutputExampleProvider = provider
	}
}

func WithPersistentContextProvider(provider ContextProviderFunc) ReActLoopOption {
	return func(r *ReActLoop) {
		r.persistentInstructionProvider = provider
	}
}

func WithReflectionOutputExample(example string) ReActLoopOption {
	return WithReflectionOutputExampleContextProvider(func(loop *ReActLoop, nonce string) (string, error) {
		_, result, err := loop.getRenderInfo()
		if err != nil {
			return "", utils.Errorf("get basic prompt info failed: %v", err)
		}
		result["Nonce"] = nonce
		baseExample, err := utils.RenderTemplate(example, result)
		if err != nil {
			return "", err
		}

		// Append loop-specific output examples from registered loops and actions
		var loopExamples string
		for _, actionName := range loop.loopActions.Keys() {
			// First, try to get OutputExamples from the registered LoopAction
			if action, ok := GetLoopAction(actionName); ok && action.OutputExamples != "" {
				rendered, err := utils.RenderTemplate(action.OutputExamples, result)
				if err == nil && rendered != "" {
					loopExamples += "\n" + rendered
				}
			} else if meta, ok := GetLoopMetadata(actionName); ok && meta.OutputExamplePrompt != "" {
				// Fallback to LoopMetadata if LoopAction doesn't have OutputExamples
				rendered, err := utils.RenderTemplate(meta.OutputExamplePrompt, result)
				if err == nil && rendered != "" {
					loopExamples += "\n" + rendered
				}
			}
		}

		if loopExamples != "" {
			return baseExample + loopExamples, nil
		}
		return baseExample, nil
	})
}

func WithPersistentInstruction(instruction string) ReActLoopOption {
	return WithPersistentContextProvider(func(loop *ReActLoop, nonce string) (string, error) {
		_, result, err := loop.getRenderInfo()
		if err != nil {
			return "", utils.Errorf("get basic prompt info failed: %v", err)
		}
		result["Nonce"] = nonce
		return utils.RenderTemplate(instruction, result)
	})
}

func WithReactiveDataBuilder(provider FeedbackProviderFunc) ReActLoopOption {
	return func(r *ReActLoop) {
		r.reactiveDataBuilder = provider
	}
}

func WithOnTaskCreated(fn func(task aicommon.AIStatefulTask)) ReActLoopOption {
	return func(r *ReActLoop) {
		r.onTaskCreated = fn
	}
}

func WithOnAsyncTaskTrigger(fn func(i *LoopAction, task aicommon.AIStatefulTask)) ReActLoopOption {
	return func(r *ReActLoop) {
		r.onAsyncTaskTrigger = fn
	}
}

func WithActionFactoryFromLoop(name string) ReActLoopOption {
	return func(r *ReActLoop) {
		factory, ok := GetLoopFactory(name)
		if !ok {
			log.Errorf("reactloop[%v] not found", name)
			return
		}
		actionFac := ConvertReActLoopFactoryToActionFactory(name, factory)
		r.loopActions.Set(name, actionFac)
	}
}

func WithOnAsyncTaskFinished(fn func(task aicommon.AIStatefulTask)) ReActLoopOption {
	return func(r *ReActLoop) {
		r.onAsyncTaskFinished = fn
	}
}

// WithOnPostIteraction sets a callback function that is called after each iteration of the ReAct loop.
// The operator parameter allows the callback to control whether the loop should end by calling operator.EndIteration().
func WithOnPostIteraction(fn ...func(loop *ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator)) ReActLoopOption {
	return func(r *ReActLoop) {
		r.onPostIteration = append(r.onPostIteration, fn...)
	}
}

// WithInitTask sets a callback function that is called when the loop task initializes.
// The operator parameter allows the callback to control loop behavior:
//   - operator.Done(): Exit the loop immediately (early routing, init handled everything)
//   - operator.Failed(err): Exit the loop with an error
//   - operator.Continue(): Continue with normal loop execution (default)
//   - operator.NextAction(actions...): Require specific actions for next iteration
//   - operator.RemoveNextAction(actions...): Disable specific actions for next iteration
//
// If nothing is called on operator, it defaults to Continue() behavior.
func WithInitTask(initHandler func(loop *ReActLoop, task aicommon.AIStatefulTask, operator *InitTaskOperator)) ReActLoopOption {
	return func(r *ReActLoop) {
		r.initHandler = initHandler
	}
}

func WithMemoryTriage(triage aicommon.MemoryTriage) ReActLoopOption {
	return func(r *ReActLoop) {
		r.memoryTriage = triage
	}
}

func WithMemoryPool(pool *omap.OrderedMap[string, *aicommon.MemoryEntity]) ReActLoopOption {
	return func(r *ReActLoop) {
		if utils.IsNil(pool) {
			return
		}
		r.currentMemories = pool
	}
}

func WithMemorySizeLimit(sizeLimit int) ReActLoopOption {
	return func(r *ReActLoop) {
		r.memorySizeLimit = sizeLimit
		if r.memorySizeLimit <= 0 {
			r.memorySizeLimit = 10 * 1024 // 默认 10 KB
		}
	}
}

// WithUseSpeedPriorityAICallback makes the ReAct loop use config.CallSpeedPriorityAI
// instead of config.CallAI for its main AI calls in CallAITransaction.
func WithUseSpeedPriorityAICallback(b ...bool) ReActLoopOption {
	return func(r *ReActLoop) {
		if len(b) > 0 {
			r.useSpeedPriorityAI = b[0]
		} else {
			r.useSpeedPriorityAI = true
		}
	}
}

// WithEnableSelfReflection 启用自我反思功能
// 启用后，每次 action 执行后会根据策略进行自我反思分析
// action 可以通过 operator.SetReflectionLevel() 自定义反思级别
func WithEnableSelfReflection(enable ...bool) ReActLoopOption {
	return func(r *ReActLoop) {
		if len(enable) > 0 {
			r.enableSelfReflection = enable[0]
		} else {
			r.enableSelfReflection = true
		}
	}
}

// WithSameActionTypeSpinThreshold 设置相同任务自旋阈值
// 当连续执行相同 Action 类型的次数达到此阈值时，触发 SPIN 检测
// 默认值为 3
func WithSameActionTypeSpinThreshold(threshold int) ReActLoopOption {
	return func(r *ReActLoop) {
		if threshold > 0 {
			r.sameActionTypeSpinThreshold = threshold
		}
	}
}

// WithSameLogicSpinThreshold 设置相同逻辑自旋阈值
// 当连续执行相同 Action 类型的次数达到此阈值时，使用 AI 进行深度 SPIN 检测
// 默认值为 3
func WithSameLogicSpinThreshold(threshold int) ReActLoopOption {
	return func(r *ReActLoop) {
		if threshold > 0 {
			r.sameLogicSpinThreshold = threshold
		}
	}
}

// WithMaxConsecutiveSpinWarnings sets the max number of consecutive spin warnings
// before the loop is forcibly terminated. Default is 3.
// Set to 0 to disable force-exit on spin.
func WithMaxConsecutiveSpinWarnings(max int) ReActLoopOption {
	return func(r *ReActLoop) {
		r.maxConsecutiveSpinWarnings = max
	}
}

func WithVar(key string, value any) ReActLoopOption {
	return func(r *ReActLoop) {
		r.vars.Set(key, value)
	}
}

func WithVars(vars map[string]any) ReActLoopOption {
	return func(r *ReActLoop) {
		for key, value := range vars {
			r.vars.Set(key, value)
		}
	}
}

// WithSkillLoader sets a SkillLoader and creates the SkillsContextManager.
// The allowSkillLoading and allowSkillViewOffset getters are automatically
// configured based on the SkillsContextManager state.
func WithSkillLoader(loader aiskillloader.SkillLoader, managerOpts ...aiskillloader.ManagerOption) ReActLoopOption {
	return func(r *ReActLoop) {
		mgr := aiskillloader.NewSkillsContextManager(loader, managerOpts...)
		r.skillsContextManager = mgr
		r.allowSkillLoading = func() bool {
			return mgr.HasRegisteredSkills()
		}
		r.allowSkillViewOffset = func() bool {
			return mgr.HasTruncatedViews()
		}
	}
}

// WithSkillsContextManager sets the SkillsContextManager directly.
func WithSkillsContextManager(mgr *aiskillloader.SkillsContextManager) ReActLoopOption {
	return func(r *ReActLoop) {
		r.skillsContextManager = mgr
		if mgr != nil {
			r.allowSkillLoading = func() bool {
				return mgr.HasRegisteredSkills()
			}
			r.allowSkillViewOffset = func() bool {
				return mgr.HasTruncatedViews()
			}
		}
	}
}

func WithNoEndLoadingStatus(b ...bool) ReActLoopOption {
	return func(r *ReActLoop) {
		if len(b) > 0 {
			r.noEndLoadingStatus = b[0]
		} else {
			r.noEndLoadingStatus = true
		}
	}
}

// WithExtraCapabilities sets a custom ExtraCapabilitiesManager.
// If not set, NewReActLoop creates a default one with MaxExtraTools=50.
func WithExtraCapabilities(ecm *ExtraCapabilitiesManager) ReActLoopOption {
	return func(r *ReActLoop) {
		if ecm != nil {
			r.extraCapabilities = ecm
		}
	}
}

// WithDisableLoopPerception disables the perception layer for this specific loop instance.
// This is used by lightweight sub-loops (e.g. loop_intent) that should never run
// perception evaluations regardless of the config-level setting.
// For config-level (global) control, use aicommon.WithDisablePerception instead.
func WithDisableLoopPerception(disable ...bool) ReActLoopOption {
	return func(r *ReActLoop) {
		if len(disable) > 0 && !disable[0] {
			return
		}
		r.perception = nil
	}
}

// WithPeriodicVerificationInterval sets the  iteration interval used by
// loop-level periodic checkpoint behaviors
// auto-verification.
func WithPeriodicVerificationInterval(interval int) ReActLoopOption {
	return func(r *ReActLoop) {
		if r == nil {
			return
		}
		r.periodicVerificationInterval = interval
	}
}

// WithToolCallIntervalReviewExtraPrompt injects extra instructions into the prompt
// used by interval review while tools are running.
func WithToolCallIntervalReviewExtraPrompt(prompt string) ReActLoopOption {
	return func(r *ReActLoop) {
		if r == nil || r.config == nil {
			return
		}
		if cfg, ok := r.config.(*aicommon.Config); ok {
			_ = aicommon.WithToolCallIntervalReviewExtraPrompt(prompt)(cfg)
			return
		}
		r.config.SetConfig(aicommon.ConfigKeyToolCallIntervalReviewExtraPrompt, prompt)
	}
}
