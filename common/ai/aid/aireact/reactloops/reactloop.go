package reactloops

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
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

// SatisfactionRecord 记录满意度验证的结果，包含验证状态、原因、已完成任务索引和下一步行动计划
type SatisfactionRecord struct {
	Satisfactory       bool                          `json:"satisfactory"`         // 是否满足用户需求
	Reason             string                        `json:"reason"`               // 满意/不满意的原因分析
	CompletedTaskIndex string                        `json:"completed_task_index"` // AI 判断已完成的任务索引，如 "1-1" 或 "1-1,1-2"
	NextMovements      []aicommon.VerifyNextMovement `json:"next_movements"`       // AI 下一步行动计划，用于任务执行中状态追踪
	Evidence           string                        `json:"evidence"`             // 运行期新增的证据 Markdown (legacy)
	EvidenceOps        []aicommon.EvidenceOperation  `json:"evidence_ops"`         // 结构化证据增量操作
	OutputFiles        []string                      `json:"output_files"`         // 本轮验证识别出的交付文件
}

// ActionRecord 记录每次迭代执行的 Action 信息
type ActionRecord struct {
	ActionType     string                 `json:"action_type"`
	ActionName     string                 `json:"action_name"`
	ActionParams   map[string]interface{} `json:"action_params"`
	IterationIndex int                    `json:"iteration_index"`
}

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

	// allowSkill... are the internal getter for the skills context manager
	// don't use them directly, use GetSkillsContextManager() instead
	allowSkillLoading    func() bool
	allowSkillViewOffset func() bool
	actionFilters        []func(action *LoopAction) bool

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
	memorySizeLimit int
	currentMemories *omap.OrderedMap[string, *aicommon.MemoryEntity]
	memoryTriage    aicommon.MemoryTriage

	// task status control
	onTaskCreated         func(task aicommon.AIStatefulTask)
	onAsyncTaskFinished   func(task aicommon.AIStatefulTask)
	onAsyncTaskTrigger    func(ins *LoopAction, task aicommon.AIStatefulTask)
	onPostIteration       []func(loop *ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator)
	onLoopInstanceCreated func(loop *ReActLoop)

	// onRelease 在 loop 任务执行结束时（无论成功失败）按注册顺序调用，
	// 用于回收 caller / engine / 文件句柄等资源
	onReleaseMutex sync.Mutex
	onRelease      []func()
	released       bool

	// 启动这个 loop 的时候马上要执行的事情
	// operator 用于控制 init 后的行为：Done/Failed/Continue/NextAction/RemoveNextAction
	initHandler func(loop *ReActLoop, task aicommon.AIStatefulTask, operator *InitTaskOperator)

	// use speed priority AI callback for main AI calls in the loop
	useSpeedPriorityAI bool

	// 自我反思功能开关
	enableSelfReflection bool

	// 记录历史 satisfaction 状态
	historySatisfactionReasons []*SatisfactionRecord

	// action history tracking
	actionHistory      []*ActionRecord
	actionHistoryMutex *sync.Mutex

	// verificationRuntimeSnapshot stores the last verification gate baseline so
	// generic auto-verification can compare the current loop state against the
	// previous accepted checkpoint.
	periodicVerificationInterval int // when == 0 , trigger every iteration;
	verificationRuntimeSnapshot  *VerificationRuntimeSnapshot
	verificationMutex            *sync.Mutex
	verificationWatchdogTimer    *time.Timer

	// timeline differ for tracking changes during task execution
	timelineDiffer        *aicommon.TimelineDiffer
	currentIterationIndex int

	// SPIN detection thresholds
	sameActionTypeSpinThreshold int // 相同任务自旋阈值
	sameLogicSpinThreshold      int // 相同逻辑自旋阈值

	// SPIN force-exit: consecutive spin warnings counter and threshold
	consecutiveSpinWarnings    int
	maxConsecutiveSpinWarnings int

	// Init handler action constraints
	// These are set by the init handler and cleared after first iteration
	initActionMustUse  []string // Actions that MUST be used (set by init)
	initActionDisabled []string // Actions that are DISABLED (set by init)
	initActionApplied  bool     // Whether the init constraints have been applied

	// Skills context manager for on-demand skill loading
	skillsContextManager *aiskillloader.SkillsContextManager

	// Extra capabilities discovered via intent recognition
	// Rendered as a dedicated prompt section, separate from core tools
	extraCapabilities *ExtraCapabilitiesManager

	noEndLoadingStatus bool

	// Perception layer: continuous awareness of what the user is doing,
	// producing Topics/Keywords/Summary that dynamically adjust the
	// possibility space throughout the loop lifecycle.
	perception *perceptionController
}

// AddOnReleaseHook 添加 loop 释放阶段的清理回调。多次调用按注册顺序执行。
// 关键词: loop release hook, cleanup callback
func (r *ReActLoop) AddOnReleaseHook(fn func()) {
	if fn == nil {
		return
	}
	r.onReleaseMutex.Lock()
	defer r.onReleaseMutex.Unlock()
	r.onRelease = append(r.onRelease, fn)
}

// Release 触发所有 onRelease 回调，仅生效一次。后续重复调用是 no-op。
// 关键词: loop release execution
func (r *ReActLoop) Release() {
	if r == nil {
		return
	}
	r.onReleaseMutex.Lock()
	if r.released {
		r.onReleaseMutex.Unlock()
		return
	}
	r.released = true
	hooks := append([]func(){}, r.onRelease...)
	r.onReleaseMutex.Unlock()

	for _, h := range hooks {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Errorf("react loop[%v] on-release hook panic: %v", r.loopName, rec)
				}
			}()
			h()
		}()
	}
}

func (r *ReActLoop) IncrementSpinWarning() {
	r.consecutiveSpinWarnings++
	log.Infof("consecutive spin warnings incremented to %d (max: %d)", r.consecutiveSpinWarnings, r.maxConsecutiveSpinWarnings)
}

func (r *ReActLoop) ResetSpinWarning() {
	if r.consecutiveSpinWarnings > 0 {
		log.Infof("consecutive spin warnings reset from %d to 0", r.consecutiveSpinWarnings)
	}
	r.consecutiveSpinWarnings = 0
}

func (r *ReActLoop) ShouldForceExitDueToSpin() bool {
	return r.maxConsecutiveSpinWarnings > 0 && r.consecutiveSpinWarnings >= r.maxConsecutiveSpinWarnings
}

func (r *ReActLoop) PushSatisfactionRecord(satisfactory bool, reason string) {
	r.historySatisfactionReasons = append(r.historySatisfactionReasons, &SatisfactionRecord{
		Satisfactory: satisfactory,
		Reason:       reason,
	})
}

// PushSatisfactionRecordWithCompletedTaskIndex 推送满意度记录，并同时记录已完成的任务索引和下一步行动计划
func (r *ReActLoop) PushSatisfactionRecordWithCompletedTaskIndex(satisfactory bool, reason string, completedTaskIndex string, nextMovements []aicommon.VerifyNextMovement, evidence string, outputFiles []string, evidenceOps ...[]aicommon.EvidenceOperation) {
	record := &SatisfactionRecord{
		Satisfactory:       satisfactory,
		Reason:             reason,
		CompletedTaskIndex: completedTaskIndex,
		NextMovements:      nextMovements,
		Evidence:           evidence,
		OutputFiles:        append([]string(nil), outputFiles...),
	}
	if len(evidenceOps) > 0 {
		record.EvidenceOps = evidenceOps[0]
	}
	r.historySatisfactionReasons = append(r.historySatisfactionReasons, record)
}

func (r *ReActLoop) GetLastSatisfactionRecord() (bool, string) {
	if len(r.historySatisfactionReasons) == 0 {
		return false, ""
	}
	lastRecord := r.historySatisfactionReasons[len(r.historySatisfactionReasons)-1]
	return lastRecord.Satisfactory, lastRecord.Reason
}

// GetLastSatisfactionRecordFull 获取最后一次满意度记录的完整结构，包括已完成的任务索引和下一步行动计划
// 返回 nil 表示没有记录
func (r *ReActLoop) GetLastSatisfactionRecordFull() *SatisfactionRecord {
	if len(r.historySatisfactionReasons) == 0 {
		return nil
	}
	return r.historySatisfactionReasons[len(r.historySatisfactionReasons)-1]
}

func (r *ReActLoop) GetMaxIterations() int {
	return r.maxIterations
}

func (r *ReActLoop) GetPeriodicVerificationInterval() int {
	return r.periodicVerificationInterval
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
		if r.allowPlanAndExec != nil {
			allowPE := r.allowPlanAndExec()
			if allowPE && r.GetCurrentTask() != nil && r.GetCurrentTask().IsAsyncMode() {
				allowPE = false
				info["PlanInProgress"] = true
			}
			info["AllowPlan"] = allowPE
		}
	}

	if r.allowRAG != nil && r.allowRAG() {
		info["AllowKnowledgeEnhanceAnswer"] = true
	} else {
		info["AllowKnowledgeEnhanceAnswer"] = false
	}

	if r.allowToolCall != nil && !r.allowToolCall() { // default allow tool call
		info["AllowToolCall"] = false
	} else {
		info["AllowToolCall"] = true
	}

	if r.actions != nil {
		_, hasLoadCap := r.actions.Get(schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY)
		info["HasLoadCapability"] = hasLoadCap
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
	t.SetReActLoop(r)
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

func (r *ReActLoop) GetMemoryTriage() aicommon.MemoryTriage {
	return r.memoryTriage
}

func (r *ReActLoop) GetEnableSelfReflection() bool {
	return r.enableSelfReflection
}

// GetSkillsContextManager returns the skills context manager, or nil if not configured.
func (r *ReActLoop) GetSkillsContextManager() *aiskillloader.SkillsContextManager {
	return r.skillsContextManager
}

// GetExtraCapabilities returns the extra capabilities manager for dynamically discovered capabilities.
func (r *ReActLoop) GetExtraCapabilities() *ExtraCapabilitiesManager {
	return r.extraCapabilities
}

// GetBaseFrameContext returns foundational context (time, OS, working directory, timeline)
// for use in prompt templates that need environmental awareness.
func (r *ReActLoop) GetBaseFrameContext() map[string]any {
	result := map[string]any{
		"CurrentTime": time.Now().Format("2006-01-02 15:04:05"),
		"OSArch":      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return result
	}
	if configImpl, ok := cfg.(*aicommon.Config); ok {
		if configImpl.Workdir != "" {
			result["WorkingDir"] = configImpl.Workdir
		}
		if t := configImpl.GetTimeline(); t != nil {
			result["Timeline"] = t.Dump()
		}
	}
	return result
}

// NewMinimalReActLoop creates a lightweight ReActLoop for unit testing action handlers.
// It sets up config, invoker, and emitter but skips full action registration.
func NewMinimalReActLoop(cfg aicommon.AICallerConfigIf, invoker aicommon.AIInvokeRuntime) *ReActLoop {
	return &ReActLoop{
		config:                     cfg,
		invoker:                    invoker,
		emitter:                    cfg.GetEmitter(),
		verificationMutex:          new(sync.Mutex),
		vars:                       omap.NewEmptyOrderedMap[string, any](),
		taskMutex:                  new(sync.Mutex),
		historySatisfactionReasons: make([]*SatisfactionRecord, 0),
		actionHistory:              make([]*ActionRecord, 0),
		actionHistoryMutex:         new(sync.Mutex),
	}
}

func NewReActLoop(name string, invoker aicommon.AIInvokeRuntime, options ...ReActLoopOption) (*ReActLoop, error) {
	if utils.IsNil(invoker) {
		return nil, utils.Error("invoker is nil in ReActLoop")
	}

	config := invoker.GetConfig()

	r := &ReActLoop{
		invoker:                      invoker,
		loopName:                     name,
		config:                       config,
		emitter:                      config.GetEmitter(),
		maxIterations:                100,
		periodicVerificationInterval: verificationIterationTriggerInterval,
		verificationMutex:            new(sync.Mutex),
		actions:                      omap.NewEmptyOrderedMap[string, *LoopAction](),
		loopActions:                  omap.NewEmptyOrderedMap[string, LoopActionFactory](),
		streamFields:                 omap.NewEmptyOrderedMap[string, *LoopStreamField](),
		aiTagFields:                  omap.NewEmptyOrderedMap[string, *LoopAITagField](),
		vars:                         omap.NewEmptyOrderedMap[string, any](),
		taskMutex:                    new(sync.Mutex),
		currentMemories:              omap.NewEmptyOrderedMap[string, *aicommon.MemoryEntity](),
		memorySizeLimit:              10 * 1024,
		enableSelfReflection:         true,
		historySatisfactionReasons:   make([]*SatisfactionRecord, 0),
		actionHistory:                make([]*ActionRecord, 0),
		actionHistoryMutex:           new(sync.Mutex),
		currentIterationIndex:        0,
		sameActionTypeSpinThreshold:  3, // 默认连续 3 次相同 Action 触发检测
		sameLogicSpinThreshold:       3, // 默认连续 3 次相同逻辑触发 AI 检测
		maxConsecutiveSpinWarnings:   3, // 默认连续 3 次 SPIN 警告后强制退出
		extraCapabilities:            NewExtraCapabilitiesManager(),
		perception:                   newPerceptionController(perceptionDefaultIterationInterval),
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

	// Config-level perception disable (e.g. test environments via WithDisablePerception)
	if config.GetConfigBool("DisablePerception") {
		r.perception = nil
	}

	// Auto-register perception context provider (nil-safe, skips if perception disabled)
	r.RegisterPerceptionContextProvider()

	// Auto-apply skillLoader from config if not already set via options
	// This allows users to configure skills via aicommon.WithSkillsLocalDir etc.
	if r.skillsContextManager == nil && config != nil {
		if realConfig, ok := config.(*aicommon.Config); ok {
			if loader := realConfig.GetSkillLoader(); loader != nil {
				mgr := aiskillloader.NewSkillsContextManager(loader)
				r.skillsContextManager = mgr
				r.allowSkillLoading = func() bool {
					return mgr.HasRegisteredSkills()
				}
				r.allowSkillViewOffset = func() bool {
					return mgr.HasTruncatedViews()
				}

				// Restore previously loaded skills from persistent session
				if realConfig.InitStatus.IsPersistentSessionRestored() {
					if names := realConfig.GetRestoredSkillNames(); len(names) > 0 {
						results := mgr.LoadSkills(names)
						var restored, failed []string
						for name, err := range results {
							if err != nil {
								failed = append(failed, name)
								log.Warnf("failed to restore skill %q from persistent session: %v", name, err)
							} else {
								restored = append(restored, name)
							}
						}
						if len(restored) > 0 {
							log.Infof("restored %d skills from persistent session: %v", len(restored), restored)
						}
						if len(failed) > 0 {
							log.Warnf("failed to restore %d skills from persistent session: %v", len(failed), failed)
						}
					}
				}
			}
		}
	}

	if _, ok := r.actions.Get(schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL); !ok {
		toolcall, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
		if !ok {
			return nil, utils.Errorf("loop action %s not found", schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
		}
		r.actions.Set(toolcall.ActionType, toolcall)
	}

	if _, ok := r.actions.Get(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL); !ok {
		if directCall, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL); ok {
			r.actions.Set(directCall.ActionType, directCall)
		}
	}

	if _, ok := r.actions.Get(schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE); !ok {
		if toolCompose, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE); !ok {
			log.Warn("loop action 'tool_compose' not found")
		} else {
			r.actions.Set(toolCompose.ActionType, toolCompose)
		}
	}

	if _, ok := r.actions.Get(schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY); !ok {
		if loadCap, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY); ok {
			r.actions.Set(loadCap.ActionType, loadCap)
		}
	}

	if _, ok := r.actions.Get(schema.AI_REACT_LOOP_ACTION_REQUEST_VERIFICATION); !ok {
		if verifyNow, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_REQUEST_VERIFICATION); ok {
			r.actions.Set(verifyNow.ActionType, verifyNow)
		}
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

	// Register skills actions conditionally
	if r.skillsContextManager != nil {
		if r.allowSkillLoading == nil || r.allowSkillLoading() {
			if loadSkill, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS); ok {
				r.actions.Set(loadSkill.ActionType, loadSkill)
			}
			if loadRes, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES); ok {
				r.actions.Set(loadRes.ActionType, loadRes)
			}
		}
		if r.allowSkillViewOffset == nil || r.allowSkillViewOffset() {
			if changeOffset, ok := GetLoopAction(schema.AI_REACT_LOOP_ACTION_CHANGE_SKILL_VIEW_OFFSET); ok {
				r.actions.Set(changeOffset.ActionType, changeOffset)
			}
		}
	}

	if r.emitter == nil {
		return nil, utils.Error("loop's emitter is nil in ReActLoop")
	}

	return r, nil
}

func (r *ReActLoop) Delete(key string) {
	r.vars.Delete(key)
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
	if t.IsFinished() {
		r.stopVerificationWatchdogForTask(t)
	}
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
	return nil, utils.Errorf("loop handler[%s] action[%s] not found in loop or actions", r.loopName, actionName)
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

// GetLastAction 获取上一次执行的 Action 记录
func (r *ReActLoop) GetLastAction() *ActionRecord {
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()

	if len(r.actionHistory) == 0 {
		return nil
	}
	return r.actionHistory[len(r.actionHistory)-1]
}

// GetLastNAction 获取最近 N 次的 Action 记录
func (r *ReActLoop) GetLastNAction(n int) []*ActionRecord {
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()

	if n <= 0 {
		return []*ActionRecord{}
	}

	historyLen := len(r.actionHistory)
	if historyLen == 0 {
		return []*ActionRecord{}
	}

	start := historyLen - n
	if start < 0 {
		start = 0
	}

	// 返回从 start 到末尾的所有记录（最近 N 条）
	result := make([]*ActionRecord, historyLen-start)
	copy(result, r.actionHistory[start:])
	return result
}

// GetCurrentIterationIndex 获取当前迭代索引
func (r *ReActLoop) GetCurrentIterationIndex() int {
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()
	return r.currentIterationIndex
}

// GetAllExistedActionRecord 获取所有已存在的 Action 记录
func (r *ReActLoop) GetAllExistedActionRecord() []*ActionRecord {
	r.actionHistoryMutex.Lock()
	defer r.actionHistoryMutex.Unlock()

	if len(r.actionHistory) == 0 {
		return []*ActionRecord{}
	}

	// 返回副本，避免外部修改
	result := make([]*ActionRecord, len(r.actionHistory))
	copy(result, r.actionHistory)
	return result
}

// GetTimelineDiff calculates and returns the timeline diff from baseline to current state
// This captures all changes made during the task execution in this ReactLoop
func (r *ReActLoop) GetTimelineDiff() (string, error) {
	if r.timelineDiffer == nil {
		return "", nil
	}
	return r.timelineDiffer.Diff()
}

// GetTimelineDiffWithoutUpdate gets the timeline diff without updating the baseline
// Useful for peeking at the diff without affecting future diff calculations
func (r *ReActLoop) GetTimelineDiffWithoutUpdate() string {
	if r.timelineDiffer == nil {
		return ""
	}
	baseline := r.timelineDiffer.GetLastDump()
	current := r.timelineDiffer.GetCurrentDump()
	if baseline == current {
		return ""
	}
	// Return current content as diff representation since we don't want to update baseline
	return current
}
