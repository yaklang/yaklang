package reactloops

import (
	"bytes"
	"sync"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type ReActLoopCoreGenerateCode func(
	userInput string,
	contextResult string,
	contextFeedback string,
) (string, error)

type ReActLoopOption func(r *ReActLoop)

type ContextProviderFunc func(loop *ReActLoop, nonce string) (string, error)
type FeedbackProviderFunc func(loop *ReActLoop, feedback *bytes.Buffer, nonce string) (string, error)

type ReActLoop struct {
	invoker aicommon.AIInvokeRuntime
	config  aicommon.AICallerConfigIf
	emitter *aicommon.Emitter

	maxIterations int

	loopName string

	persistentInstructionProvider   ContextProviderFunc
	reflectionOutputExampleProvider ContextProviderFunc
	reactiveDataBuilder             FeedbackProviderFunc

	allowAIForge      func() bool
	allowPlanAndExec  func() bool
	allowRAG          func() bool
	allowToolCall     func() bool
	allowUserInteract func() bool

	toolsGetter         func() []*aitool.Tool
	loopPromptGenerator ReActLoopCoreGenerateCode

	// store variable
	vars *omap.OrderedMap[string, any]

	// ai loop once
	actions      *omap.OrderedMap[string, *LoopAction]
	loopActions  *omap.OrderedMap[string, LoopActionFactory]
	streamFields *omap.OrderedMap[string, *LoopStreamField]
	aiTagFields  *omap.OrderedMap[string, *LoopAITagField]

	// execution state
	taskMutex   *sync.Mutex
	currentTask aicommon.AIStatefulTask

	// memory management
	memoryTriage aimem.MemoryTriage

	// task status control
	onTaskCreated       func(task aicommon.AIStatefulTask)
	onAsyncTaskFinished func(task aicommon.AIStatefulTask)
	onAsyncTaskTrigger  func(ins *LoopAction, task aicommon.AIStatefulTask)
	onPostIteration     func(loop *ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any)

	// 启动这个 loop 的时候马上要执行的事情
	initHandler func(task aicommon.AIStatefulTask)
}

func (r *ReActLoop) getRenderInfo() (string, map[string]any, error) {
	var tools []*aitool.Tool
	if r.toolsGetter == nil {
		tools = []*aitool.Tool{}
	} else {
		tools = r.toolsGetter()
	}
	temp, info, err := r.invoker.GetBasicPromptInfo(tools)
	if err != nil {
		return "", nil, err
	}
	if r.allowUserInteract != nil && r.allowUserInteract() {
		info["AllowAskForClarification"] = true
	} else {
		info["AllowAskForClarification"] = false
	}
	info["AskForClarificationCurrentTime"] = r.GetInt("ask_for_clarification_call_count")

	allowPlanRaw, ok := info["AllowPlan"]
	if ok && utils.InterfaceToBoolean(allowPlanRaw) {
		if r.allowPlanAndExec != nil && r.allowPlanAndExec() {
			if r.GetCurrentTask() != nil && r.GetCurrentTask().IsAsyncMode() {
				info["AllowPlan"] = false
				info["PlanInProgress"] = true
			} else {
				info["PlanInProgress"] = false
				info["AllowPlan"] = true
			}
		}
	}

	if r.allowRAG != nil && r.allowRAG() {
		info["AllowKnowledgeEnhanceAnswer"] = true
	} else {
		info["AllowKnowledgeEnhanceAnswer"] = false
	}

	result, err := utils.RenderTemplate(temp, info)
	if err != nil {
		return "", nil, err
	}
	return result, info, nil
}

func (r *ReActLoop) DisallowAskForClarification() {
	r.allowUserInteract = func() bool {
		return false
	}
}

func (r *ReActLoop) GetCurrentTask() aicommon.AIStatefulTask {
	r.taskMutex.Lock()
	defer r.taskMutex.Unlock()
	return r.currentTask
}

func (r *ReActLoop) SetCurrentTask(t aicommon.AIStatefulTask) {
	r.taskMutex.Lock()
	defer r.taskMutex.Unlock()
	r.currentTask = t
}

func (r *ReActLoop) GetInvoker() aicommon.AIInvokeRuntime {
	return r.invoker
}

func (r *ReActLoop) GetEmitter() *aicommon.Emitter {
	return r.emitter
}

func (r *ReActLoop) GetConfig() aicommon.AICallerConfigIf {
	return r.config
}

func NewReActLoop(name string, invoker aicommon.AIInvokeRuntime, options ...ReActLoopOption) (*ReActLoop, error) {
	if utils.IsNil(invoker) {
		return nil, utils.Error("invoker is nil in ReActLoop")
	}

	config := invoker.GetConfig()

	r := &ReActLoop{
		invoker:       invoker,
		loopName:      name,
		config:        config,
		emitter:       config.GetEmitter(),
		maxIterations: 100,
		actions:       omap.NewEmptyOrderedMap[string, *LoopAction](),
		loopActions:   omap.NewEmptyOrderedMap[string, LoopActionFactory](),
		streamFields:  omap.NewEmptyOrderedMap[string, *LoopStreamField](),
		aiTagFields:   omap.NewEmptyOrderedMap[string, *LoopAITagField](),
		vars:          omap.NewEmptyOrderedMap[string, any](),
		taskMutex:     new(sync.Mutex),
	}

	for _, action := range []*LoopAction{
		loopAction_DirectlyAnswer,
		loopAction_Finish,
	} {
		r.actions.Set(action.ActionType, action)
	}

	for _, streamField := range []*LoopStreamField{
		{
			FieldName: "human_readable_thought",
			AINodeId:  "re-act-loop-thought",
		},
	} {
		r.streamFields.Set(streamField.FieldName, streamField)
	}

	for _, opt := range options {
		opt(r)
	}

	if _, ok := r.actions.Get(schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL); !ok {
		toolcall, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
		if !ok {
			return nil, utils.Errorf("loop action %s not found", schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
		}
		r.actions.Set(toolcall.ActionType, toolcall)
	}

	if r.allowRAG == nil || r.allowRAG() {
		// allow tool call, must have tools
		ins, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE)
		if !ok {
			return nil, utils.Errorf("loop action %s not found", schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
		}
		r.actions.Set(ins.ActionType, ins)
	}

	if r.allowAIForge == nil || r.allowAIForge() {
		aiforge, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT)
		if !ok {
			return nil, utils.Errorf("loop action %s not found", schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT)
		}
		r.actions.Set(aiforge.ActionType, aiforge)
	}

	if r.allowPlanAndExec == nil || r.allowPlanAndExec() {
		plan, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION)
		if !ok {
			return nil, utils.Errorf("loop action %s not found", schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION)
		}
		r.actions.Set(plan.ActionType, plan)
	}

	if r.allowUserInteract == nil || r.allowUserInteract() {
		ac, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION)
		if !ok {
			return nil, utils.Errorf("loop action %s not found", schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION)
		}
		r.actions.Set(ac.ActionType, ac)
	}

	if r.emitter == nil {
		return nil, utils.Error("loop's emitter is nil in ReActLoop")
	}

	return r, nil
}

func (r *ReActLoop) Set(i string, result any) {
	r.vars.Set(i, result)
}

func (r *ReActLoop) Get(i string) string {
	result, ok := r.vars.Get(i)
	if ok {
		return utils.InterfaceToString(result)
	}
	return ""
}

func (r *ReActLoop) GetVariable(i string) any {
	result, ok := r.vars.Get(i)
	if ok {
		return result
	}
	return nil
}

func (r *ReActLoop) GetStringSlice(i string) []string {
	resultRaw := r.GetVariable(i)
	result := utils.IsNil(resultRaw)
	if !result {
		return utils.InterfaceToStringSlice(resultRaw)
	}
	return []string{}
}

func (r *ReActLoop) GetInt(k string) int {
	resultRaw := r.GetVariable(k)
	result := utils.IsNil(resultRaw)
	if !result {
		return utils.InterfaceToInt(resultRaw)
	}
	return 0
}

func (r *ReActLoop) RemoveAction(actionType string) {
	r.actions.Delete(actionType)
	r.loopActions.Delete(actionType)
}

func (r *ReActLoop) OnTaskCreated(f func(task aicommon.AIStatefulTask)) {
	r.onTaskCreated = f
}

func (r *ReActLoop) OnAsyncTaskTrigger(f func(ins *LoopAction, task aicommon.AIStatefulTask)) {
	r.onAsyncTaskTrigger = f
}

func (r *ReActLoop) OnAsyncTaskFinished(f func(task aicommon.AIStatefulTask)) {
	r.onAsyncTaskFinished = f
}

func (r *ReActLoop) FinishAsyncTask(t aicommon.AIStatefulTask, err error) {
	if utils.IsNil(t) {
		log.Error("FinishAsyncTask: task is nil")
		return
	}
	if !t.IsAsyncMode() {
		return
	}
	t.Finish(err)
	if r.onAsyncTaskFinished != nil {
		r.onAsyncTaskFinished(t)
	}
}

func (r *ReActLoop) GetActionHandler(actionName string) (*LoopAction, error) {
	ac, ok := r.actions.Get(actionName)
	if ok {
		return ac, nil
	}
	fac, ok := r.loopActions.Get(actionName)
	if ok {
		ac, err := fac(r.GetInvoker())
		if err != nil {
			return nil, utils.Errorf("cannot create loop action[%s] instance: %v", r.loopName, err)
		}
		return ac, nil
	}
	return nil, utils.Errorf("action handler[%s] not found in loop or actions", r.loopName)
}

func (r *ReActLoop) GetAllActionNames() []string {
	actionNames := r.actions.Keys()
	for _, actionName := range r.loopActions.Keys() {
		if !r.actions.Have(actionName) {
			actionNames = append(actionNames, actionName)
		}
	}
	return actionNames
}

func (r *ReActLoop) NoActions() bool {
	return r.actions.Len() == 0 && r.loopActions.Len() == 0
}

func (r *ReActLoop) GetAllActions() []*LoopAction {
	var actions []*LoopAction
	actions = append(actions, r.actions.Values()...)
	for _, actionName := range r.loopActions.Keys() {
		if r.actions.Have(actionName) {
			continue
		}
		actionFac, ok := r.loopActions.Get(actionName)
		if !ok {
			log.Errorf("loopAction factory[%s] not found when getting all actions", actionName)
			continue
		}
		actionInstance, err := actionFac(r.GetInvoker())
		if err != nil {
			log.Errorf("create loopAction[%s] instance failed when getting all actions: %v", actionName, err)
			continue
		}
		actions = append(actions, actionInstance)
	}
	return actions
}
