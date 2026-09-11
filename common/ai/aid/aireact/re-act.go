package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/rag/rag_search_tool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactinit"
)

const MainTaskQueueName = "react-main-queue"

// 同步类型常量
const (
	SYNC_TYPE_QUEUE_INFO                = "queue_info"
	SYNC_TYPE_TIMELINE                  = "timeline"
	SYNC_TYPE_KNOWLEDGE                 = "enhance_knowledge"
	SYNC_TYPE_UPDATE_CONFIG             = "update_config"
	SYNC_TYPE_MEMORY_CONTEXT            = "memory_sync"
	SYNC_TYPE_REACT_CANCEL_CURRENT_TASK = "react_cancel_current_task"
	SYNC_TYPE_REACT_JUMP_QUEUE          = "react_jump_queue"
	SYNC_TYPE_REACT_CANCEL_TASK         = "react_cancel_task"
	SYNC_TYPE_REACT_REMOVE_TASK         = "react_remove_task"
	SYNC_TYPE_REACT_CLEAR_TASK          = "react_clear_task"
	SYNC_TYPE_RECOVERY_PLAN_AND_EXEC    = "recovery_plan_and_exec"
	SYNC_TYPE_EXECUTE_DETACHED_PLAN     = "execute_detached_plan"
)

// ReactTaskItem 表示ReAct任务队列中的单个任务
type ReactTaskItem struct {
	ID        string                 // 任务唯一标识
	UserInput string                 // 用户输入
	Event     *ypb.AIInputEvent      // 原始输入事件
	Status    string                 // 任务状态: pending, processing, completed, failed
	CreatedAt time.Time              // 创建时间
	StartedAt *time.Time             // 开始处理时间
	EndedAt   *time.Time             // 完成时间
	Metadata  map[string]interface{} // 额外元数据
}

var (
	_ aicommon.AIInvokeRuntime = (*ReAct)(nil)
)

type ReAct struct {
	*aicommon.Emitter

	currentIteration            int
	currentUserInteractiveCount int64 // 当前用户交互次数
	knowledgeEmitCounter        int   // Counter for knowledge emit events
	// verificationHistory 已上移到 aicommon.SessionPromptState 作为 todoJSON 持久态,
	// 让 loop prompt 与 verify 路径共享同一份增量 TODO 状态; ReAct 内不再保存副本.
	// 关键词: verificationHistory 迁移, SessionPromptState todoJSON, 单一来源

	config        *aicommon.Config
	promptManager *PromptManager

	inputChanx *chanx.UnlimitedChan[*ypb.AIInputEvent]

	// 任务队列相关
	currentTaskMu sync.RWMutex
	currentTask   aicommon.AIStatefulTask // 当前正在处理的任务

	lastTask aicommon.AIStatefulTask // 上一个完成的任务

	// All Runtime Tasks
	RuntimeTasks           []aicommon.AIStatefulTask
	UpdateRuntimeTaskMutex sync.Mutex

	currentPlanExecutionMu sync.RWMutex
	currentPlanExecution   aicommon.AIStatefulTask
	taskQueue              *TaskQueue // 任务队列
	queueProcessor         sync.Once  // 确保队列处理器只启动一次
	mirrorMutex            sync.RWMutex
	mirrorOfAIInputEvent   map[string]func(*ypb.AIInputEvent)

	saveTimelineThrottle func(func())
	artifacts            *filesys.RelLocalFs

	wg           *sync.WaitGroup
	lifecycleWG  *sync.WaitGroup
	memoryTriage aicommon.MemoryTriage

	taskHandoffMu      sync.Mutex
	taskHandoffs       int
	taskHandoffVersion uint64

	midtermRecallMutex           sync.Mutex
	pendingMidtermTimelineRecall bool
	pendingMidtermPerception     *midtermPerceptionSnapshot

	pureInvokerMode bool // 纯调用者模式，不启动事件循环和队列处理器

	browserSessionsMu sync.Mutex
	browserSessionIDs map[string]struct{}
}

func (r *ReAct) SetCurrentTask(task aicommon.AIStatefulTask) {
	r.setCurrentTask(task)
}

func (r *ReAct) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	return r.promptManager.GetBasicPromptInfo(tools)
}

const SKIP_AI_REVIEW = "skip_ai_review"

func (r *ReAct) GetConfig() aicommon.AICallerConfigIf {
	return r.config
}

func (r *ReAct) SaveTimeline() {
	if r == nil || r.config == nil || r.config.PersistentSessionId == "" {
		return
	}
	ins := r.config.Timeline
	if ins == nil || ins.IsBranchTimeline() {
		return
	}
	r.saveTimelineThrottle(func() {
		tl, err := aicommon.MarshalTimeline(ins)
		if err != nil {
			log.Errorf("ReAct: marshal timeline failed: %v", err)
			return
		}
		result := strconv.Quote(tl)
		if err := yakit.UpdateAIAgentRuntimeTimeline(r.config.GetDB(), r.config.Id, result); err != nil {
			log.Errorf("ReAct: save timeline to db failed: %v", err)
			return
		}
		last1 := ins.ToTimelineItemOutputLastN(1)
		if len(last1) > 0 {
			log.Debugf("ReAct: save timeline to db success timeline last updated time: %v", last1[0].Timestamp.String())
		}
	})
}

func (r *ReAct) DumpTimeline() string {
	if r == nil || r.config == nil || r.config.Timeline == nil {
		return ""
	}
	return r.config.Timeline.Dump()
}

func (r *ReAct) SetCurrentPlanExecutionTask(t aicommon.AIStatefulTask) {
	if r == nil {
		return
	}
	r.currentPlanExecutionMu.Lock()
	r.currentPlanExecution = t
	r.currentPlanExecutionMu.Unlock()
}

func (r *ReAct) GetCurrentPlanExecutionTask() aicommon.AIStatefulTask {
	if r == nil {
		return nil
	}
	r.currentPlanExecutionMu.RLock()
	task := r.currentPlanExecution
	r.currentPlanExecutionMu.RUnlock()
	return task
}

func (r *ReAct) RegisterMirrorOfAIInputEvent(id string, f func(*ypb.AIInputEvent)) {
	r.mirrorMutex.Lock()
	defer r.mirrorMutex.Unlock()
	r.mirrorOfAIInputEvent[id] = f
}

func (r *ReAct) CallMirrorOfAIInputEvent(event *ypb.AIInputEvent) {
	r.mirrorMutex.RLock()
	defer r.mirrorMutex.RUnlock()
	for _, f := range r.mirrorOfAIInputEvent {
		f(event)
	}
}

func (r *ReAct) UnregisterMirrorOfAIInputEvent(id string) {
	r.mirrorMutex.Lock()
	defer r.mirrorMutex.Unlock()
	delete(r.mirrorOfAIInputEvent, id)
}

func NewReAct(opts ...aicommon.ConfigOption) (*ReAct, error) {
	configLoadingStart := time.Now()
	cfg := aicommon.NewConfig(context.Background(), opts...)

	// Extract built-in skills to ~/yakit-projects/ai-skills/ only when auto-skills
	// are enabled, then load from the local directory so users can modify them on disk.
	if !cfg.IsAutoSkillsDisabled() {
		aiSkillsDir := consts.GetDefaultAISkillsDir()
		if err := ExtractBuiltinSkillsToDir(aiSkillsDir); err != nil {
			log.Warnf("failed to extract built-in skills to %s: %v", aiSkillsDir, err)
		}
		if err := cfg.LoadBuiltinSkillsFromDir(aiSkillsDir); err != nil {
			log.Warnf("failed to load skills from %s: %v", aiSkillsDir, err)
		}
		// Re-scan the canonical built-in directory last. Legacy top-level copies
		// may share a name, but edits made through the recommended-skill API must
		// be the version that a newly-created ReAct session loads.
		builtinSkillsDir := filepath.Join(aiSkillsDir, "builtin")
		if utils.IsDir(builtinSkillsDir) {
			if err := cfg.LoadBuiltinSkillsFromDir(builtinSkillsDir); err != nil {
				log.Warnf("failed to prioritize built-in skills from %s: %v", builtinSkillsDir, err)
			}
		}
	}

	if du := time.Since(configLoadingStart); du > 500*time.Millisecond {
		log.Warnf("loading ReAct config took %s, too long, maybe some events happened.", du.String())
	}

	// artifacts directory is lazily created when user input arrives (ensureWorkDirectory)
	// artifacts field starts as nil and is initialized in ensureWorkDirectory or getArtifacts
	react := &ReAct{
		config:               cfg,
		Emitter:              cfg.Emitter, // Use the emitter from config
		taskQueue:            NewTaskQueue(MainTaskQueueName),
		mirrorOfAIInputEvent: make(map[string]func(*ypb.AIInputEvent)),
		saveTimelineThrottle: utils.NewThrottleEx(3, true, true),
		artifacts:            nil, // lazy: created in ensureWorkDirectory
		wg:                   new(sync.WaitGroup),
		lifecycleWG:          new(sync.WaitGroup),
		browserSessionIDs:    make(map[string]struct{}),
	}

	cfg.SetBrowserSessionTracker(react)

	if cfg.PersistentSessionId != "" && cfg.GetDB() != nil {
		meta, err := yakit.EnsureAISessionMeta(cfg.GetDB(), cfg.PersistentSessionId, cfg.SessionSource)
		if err != nil {
			log.Warnf("ensure ai session meta failed for %s: %v", cfg.PersistentSessionId, err)
		} else {
			react.restoreInitializedSessionTitle(meta)
		}
	}

	memoryLoadStart := time.Now()
	if cfg.DisableMemoryTriage {
		react.memoryTriage = aicommon.NewNoOpMemoryTriage()
		react.config.MemoryTriage = react.memoryTriage
		log.Infof("memory triage disabled (no-op) for ReAct instance: %s", react.config.Id)
	} else if cfg.MemoryTriage != nil {
		react.memoryTriage = cfg.MemoryTriage
	} else {
		memoryTriageId := cfg.MemoryTriageId
		if memoryTriageId == "" {
			memoryTriageId = "default"
		}
		react.memoryTriage = aimem.NewAsyncAIMemory(cfg.GetContext(), memoryTriageId, aimem.WithInvoker(react))
		react.config.MemoryTriage = react.memoryTriage
	}
	memoryLoad := time.Now().Sub(memoryLoadStart)
	if memoryLoad.Milliseconds() > 500 {
		log.Warnf("loading memory triage took %s, too long, maybe some events happened.", memoryLoad.String())
	}

	log.Infof("memory triage id: %s", react.memoryTriage.GetSessionID())

	if cfg.TimelineArchiveStore == nil && strings.TrimSpace(cfg.PersistentSessionId) != "" {
		midtermSessionID := aimem.PersistentSessionToMidtermMemorySessionID(cfg.PersistentSessionId)
		cfg.TimelineArchiveStore = aimem.NewAsyncAIMemoryForQuery(cfg.GetContext(), midtermSessionID,
			aimem.WithDatabase(cfg.GetDB()), aimem.WithMidtermArchiveMode())
	}
	cfg.EnhanceKnowledgeManager.SetEmitter(cfg.Emitter)
	if cfg.Timeline == nil {
		cfg.Timeline = aicommon.NewTimeline(cfg, nil)
	}
	if cfg.TimelineDiffer == nil {
		cfg.TimelineDiffer = aicommon.NewTimelineDiffer(cfg.Timeline)
	}
	// Initialize prompt manager (workdir does not depend on artifacts, which is lazy)
	workdir := cfg.Workdir
	if workdir == "" {
		workdir = filepath.Join(consts.GetDefaultYakitBaseDir(), "code")
		if utils.GetFirstExistedFile(workdir) == "" {
			os.MkdirAll(workdir, os.ModePerm)
		}
	}
	react.promptManager = NewPromptManager(react, workdir)

	cfg.SetHotpatchCurrentTaskIdResolver(func() string {
		return react.GetCurrentTaskId()
	})

	cfg.SetCapabilityHotpatchHandler(func(enable bool, caps []aicommon.EnabledCapability) {
		loop := react.GetCurrentLoop()
		if loop == nil {
			return
		}
		ecm := loop.GetExtraCapabilities()
		if ecm == nil {
			return
		}
		toolMgr := react.config.GetAiToolManager()

		for _, cap := range caps {
			switch cap.Type {
			case aicommon.EnabledCapabilityTypeTool, aicommon.EnabledCapabilityTypePlugin, aicommon.EnabledCapabilityTypeMCPTool:
				if toolMgr == nil {
					continue
				}
				// Hotpatch affects prompt-level suggestions only: resolve tool object from current manager snapshot.
				if enable {
					if tool, err := toolMgr.GetToolByName(cap.Name); err == nil && tool != nil {
						ecm.AddTools(tool)
					}
				} else {
					ecm.RemoveToolByName(cap.Name)
				}
			case aicommon.EnabledCapabilityTypeForge:
				if enable {
					reactloops.LoadEnabledForges(react.config, loop, []string{cap.Name})
				} else {
					ecm.RemoveForgeByName(cap.Name)
				}
			case aicommon.EnabledCapabilityTypeSkill:
				// Hotpatch skill is inventory-only. We do NOT load/unload SKILLS_CONTEXT here.
				if enable {
					ecm.AddSkills(reactloops.ExtraSkillInfo{Name: cap.Name})
				} else {
					ecm.RemoveSkillByName(cap.Name)
				}
			}
		}
	})

	cfg.SetSkillHotloadHandler(func(skillNames []string) {
		if len(skillNames) == 0 {
			return
		}
		if loop := react.GetCurrentLoop(); loop != nil {
			if mgr := loop.GetSkillsContextManager(); mgr != nil {
				for _, name := range skillNames {
					added, err := mgr.LoadForcedSkill(name)
					if err != nil {
						log.Warnf("hotload skill %q failed: %v", name, err)
						continue
					}
					if added {
						// 用户强制加载: timeline 明确记录 + 命中反馈.
						react.AddToTimeline("user_loaded_skill", fmt.Sprintf("User forced load: %s", name))
						aicommon.SubmitSkillHit(react.config, name, aicommon.StatsSourceSkillUserForce)
					}
				}
			}
		}
	})

	cfg.SetForgeHotloadHandler(func(forgeNames []string) {
		if len(forgeNames) == 0 {
			return
		}
		if loop := react.GetCurrentLoop(); loop != nil {
			reactloops.LoadEnabledForges(react.config, loop, forgeNames)
		}
	})

	cfg.SetSkillUnloadHandler(func(skillNames []string) {
		if len(skillNames) == 0 {
			return
		}
		if loop := react.GetCurrentLoop(); loop != nil {
			if mgr := loop.GetSkillsContextManager(); mgr != nil {
				for _, name := range skillNames {
					if mgr.UnloadSkill(name) {
						log.Infof("hot-unload skill %q from context", name)
					}
				}
			}
		}
	})

	cfg.SetForgeUnloadHandler(func(forgeNames []string) {
		if len(forgeNames) == 0 {
			return
		}
		if loop := react.GetCurrentLoop(); loop != nil {
			if ecm := loop.GetExtraCapabilities(); ecm != nil {
				for _, name := range forgeNames {
					if ecm.RemoveForgeByName(name) {
						log.Infof("hot-unload forge %q from extra capabilities", name)
					}
				}
			}
		}
	})

	cfg.SetSessionSnapshotEmitHandler(func() {
		reactloops.EmitSessionSnapshot(react.config, react.GetCurrentLoop(), react.GetCurrentTask())
	})

	// Register pending context providers
	react.promptManager.cpm = cfg.ContextProviderManager
	react.installRunningSessionRegistry()
	// Start the event loop in background
	mainloopDone := make(chan struct{})
	react.startEventLoop(cfg.Ctx, mainloopDone)
	select {
	case <-cfg.Ctx.Done():
		return nil, utils.Errorf("context canceled before ReAct invoker started")
	case <-mainloopDone:
	}

	// Start queue processor in background
	done := make(chan struct{})
	react.startQueueProcessor(cfg.Ctx, done)
	select {
	case <-cfg.Ctx.Done():
		return nil, utils.Errorf("context canceled before queue processer started")
	case <-done:
	}

	if err := cfg.CreateOrUpdateRuntimeRecord(&schema.AIAgentRuntime{
		Uuid:              cfg.GetRuntimeId(),
		Name:              "[re-act-runtime]",
		Seq:               cfg.Seq,
		TypeName:          schema.AIAgentRuntimeType_ReAct,
		PersistentSession: cfg.PersistentSessionId,
	}); err != nil {
		return nil, err
	}
	cfg.FlushRestoredSessionEvidence()
	// EmitPinDirectory is deferred to ensureWorkDirectory when user input arrives

	// When the session is restricted to its injected MCP servers, the profile/DB
	// MCP machinery must NOT run: its background goroutine (and the
	// tools-list-changed handler) would re-enable profile tools via
	// OverrideToolByName AFTER loadExtraMCPServers calls RestrictToTools,
	// silently defeating the restriction (fail-open) and racing the tool
	// manager's enable map. Skipping it keeps the restriction authoritative.
	if !react.config.DisallowMCPServers && !react.config.RestrictToolsToExtraMCPServers {
		// Synchronously pre-load MCP stub tools from DB into the tool manager
		// so they appear in the system prompt on the very first request, before
		// the background connection goroutine has finished.
		react.preloadMCPStubsFromDB()

		// Fire-and-forget: connect to MCP servers, write fresh tool metadata
		// to DB, then append live tools (with real callbacks) to AiToolManager,
		// replacing the stubs. The first user query is not blocked.
		react.loadMCPServers()
	}

	// 会话级显式挂载（内存态，不读 profile DB、不进全局列表）。
	// 同步加载，保证首轮推理前工具已就绪。
	if len(react.config.ExtraMCPServers) > 0 {
		react.loadExtraMCPServers(react.config.ExtraMCPServers)
	}

	return react, nil
}

// restoreInitializedSessionTitle restores only a real, durable session title.
// New session metadata deliberately contains the non-empty "<未命名>" placeholder
// with TitleInitialized=false. Treating that placeholder as a generated title
// sets sessionTitleGeneratedKey too early and permanently skips title generation
// for the first user input.
func (r *ReAct) restoreInitializedSessionTitle(meta *schema.AISession) bool {
	if r == nil || r.config == nil || meta == nil || !meta.TitleInitialized {
		return false
	}
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		return false
	}
	r.config.SetConfig("session_title", title)
	r.config.SetSessionTitle(title)
	r.config.SetConfig(sessionTitleGeneratedKey, true)
	if r.Emitter != nil {
		r.Emitter.EmitSessionTitle(title)
	}
	return true
}

// UpdateDebugMode dynamically updates the debug mode settings
func (r *ReAct) UpdateDebugMode(debug bool) {
	r.config.DebugPrompt = debug
	r.config.DebugEvent = debug
}

// SendInputEvent sends an input event to the task queue (non-blocking)
// This is the only public API for external clients to send input to ReAct
func (r *ReAct) SendInputEvent(event *ypb.AIInputEvent) (ret error) {
	defer func() {
		if retErr := recover(); retErr != nil {
			ret = utils.Errorf("SendInputEvent panic: %v", retErr)
		}
	}()
	if event == nil {
		return fmt.Errorf("input event is nil")
	}

	r.config.EventInputChan.SafeFeed(event)
	return nil
}

// SendInputEventAndWaitAccepted synchronously dispatches config hot-patches and
// admits plain free-input events into the ReAct task queue. Session coordinators
// use this narrower entry point both to preserve the event loop's
// "hot-patch before following input" order and to make "check session idle ->
// enqueue task" atomic with respect to an external reservation. Other event
// kinds retain the asynchronous event-loop behavior of SendInputEvent.
func (r *ReAct) SendInputEventAndWaitAccepted(event *ypb.AIInputEvent) (ret error) {
	defer func() {
		if retErr := recover(); retErr != nil {
			ret = utils.Errorf("SendInputEventAndWaitAccepted panic: %v", retErr)
		}
	}()
	if event == nil {
		return fmt.Errorf("input event is nil")
	}
	if r == nil || r.config == nil {
		return fmt.Errorf("re-act runtime is nil")
	}
	if err := r.config.GetContext().Err(); err != nil {
		return err
	}

	// Preserve Config.StartEventLoopEx's precedence rule: a hot-patch is
	// converted and queued before the loop receives the following input. Without
	// doing this synchronously, a directly-admitted free input could overtake a
	// hot-patch still waiting in EventInputChan.
	if event.GetIsConfigHotpatch() {
		hotPatchOptions := r.config.ProcessHotPatchMessage(event)
		r.config.PersistSessionStartParamsFromHotpatch(event)
		for _, option := range hotPatchOptions {
			r.config.HotPatchOptionChan.SafeFeed(option)
		}
		return nil
	}

	// Interactive and sync messages retain Config.processInputEvent's existing
	// asynchronous execution behavior.
	if !event.GetIsFreeInput() || event.GetIsInteractiveMessage() || event.GetIsSyncMessage() {
		return r.SendInputEvent(event)
	}
	if r.config.InputEventManager != nil {
		r.config.InputEventManager.CallMirrorOfAIInputEvent(event)
	}
	return r.handleFreeValue(event)
}

// CancelTaskByUserInputUUID cancels the root task admitted from a particular
// input event. A transport-independent session owner uses it to stop only its
// task when it is attached to a ReAct runtime owned by somebody else.
func (r *ReAct) CancelTaskByUserInputUUID(inputUUID string) bool {
	inputUUID = strings.TrimSpace(inputUUID)
	if r == nil || inputUUID == "" {
		return false
	}
	cancelTask := func(task aicommon.AIStatefulTask) bool {
		if task == nil || task.IsFinished() {
			return false
		}
		r.CancelTask(task, &ypb.AIInputEvent{})
		return true
	}
	for _, task := range r.GetRuntimeTasks() {
		if task != nil && task.GetUserInputUUID() == inputUUID && cancelTask(task) {
			return true
		}
	}
	for _, task := range r.GetQueueingTasks() {
		if task == nil || task.GetUserInputUUID() != inputUUID {
			continue
		}
		if r.taskQueue.RemoveTask(task.GetId()) && cancelTask(task) {
			return true
		}
	}
	// The queue processor may have moved the task between the two snapshots.
	for _, task := range r.GetRuntimeTasks() {
		if task != nil && task.GetUserInputUUID() == inputUUID && cancelTask(task) {
			return true
		}
	}
	return false
}

// CancelTaskByUserInputUUIDAndWait retries cancellation across the queue to
// runtime hand-off, then waits until processReActTask's deferred persistence and
// cleanup have finished. A single lookup is insufficient because dequeue hooks
// run after removing the task from the queue and before runtime publication.
func (r *ReAct) CancelTaskByUserInputUUIDAndWait(ctx context.Context, inputUUID string) error {
	inputUUID = strings.TrimSpace(inputUUID)
	if r == nil || inputUUID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, versionBefore := r.taskQueueHandoffState()
		r.CancelTaskByUserInputUUID(inputUUID)
		handoffActive, versionAfter := r.taskQueueHandoffState()
		if !handoffActive && versionBefore == versionAfter {
			break
		}
		if !handoffActive {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return r.WaitTaskByUserInputUUIDStopped(ctx, inputUUID)
}

func (r *ReAct) hasTaskByUserInputUUID(inputUUID string) bool {
	for _, task := range r.GetRuntimeTasks() {
		if task != nil && task.GetUserInputUUID() == inputUUID {
			return true
		}
	}
	for _, task := range r.GetQueueingTasks() {
		if task != nil && task.GetUserInputUUID() == inputUUID {
			return true
		}
	}
	return false
}

func (r *ReAct) beginTaskQueueHandoff() func() {
	if r == nil {
		return func() {}
	}
	r.taskHandoffMu.Lock()
	r.taskHandoffs++
	r.taskHandoffVersion++
	r.taskHandoffMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.taskHandoffMu.Lock()
			if r.taskHandoffs > 0 {
				r.taskHandoffs--
			}
			r.taskHandoffVersion++
			r.taskHandoffMu.Unlock()
		})
	}
}

func (r *ReAct) hasTaskQueueHandoff() bool {
	hasHandoff, _ := r.taskQueueHandoffState()
	return hasHandoff
}

func (r *ReAct) taskQueueHandoffState() (bool, uint64) {
	if r == nil {
		return false, 0
	}
	r.taskHandoffMu.Lock()
	hasHandoff := r.taskHandoffs > 0
	version := r.taskHandoffVersion
	r.taskHandoffMu.Unlock()
	return hasHandoff, version
}

// WaitTaskByUserInputUUIDStopped waits for a task to leave both the queue and
// the runtime list. A terminal status event is emitted before processReActTask's
// deferred persistence and cleanup have returned, so observing that event alone
// is not sufficient for session deletion safety.
func (r *ReAct) WaitTaskByUserInputUUIDStopped(ctx context.Context, inputUUID string) error {
	inputUUID = strings.TrimSpace(inputUUID)
	if r == nil || inputUUID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Require two consecutive absent observations as a final stability check.
	// processReActFromQueue also publishes an explicit hand-off marker while a
	// task is between the queue and runtime lists, because dequeue hooks may block
	// long enough that time-based polling alone cannot safely bridge that window.
	absentOnce := false
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if r.hasTaskByUserInputUUID(inputUUID) || r.hasTaskQueueHandoff() {
			absentOnce = false
		} else if absentOnce {
			return nil
		} else {
			absentOnce = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// AddToTimeline 添加条目到时间线
func (r *ReAct) AddToTimeline(entryType, content string) {
	r.addToTimelineWithPromptProjection(entryType, content, "")
}

// AddToTimelineWithPromptProjection records a display-safe timeline entry and
// an alternate prompt-only representation. It is intentionally not part of the
// public AIInvokeRuntime contract; ReActLoop discovers it through a narrow
// optional interface so existing runtimes and test doubles remain compatible.
func (r *ReAct) AddToTimelineWithPromptProjection(entryType, content, promptContent string) {
	r.addToTimelineWithPromptProjection(entryType, content, promptContent)
}

func (r *ReAct) addToTimelineWithPromptProjection(entryType, content, promptContent string) {
	buildMessage := func(body string) string {
		msg := new(bytes.Buffer)
		if entryType != "" {
			msg.WriteString(fmt.Sprintf("[%s]", entryType))
		} else {
			msg.WriteString("[note]")
		}

		t := r.GetCurrentTask()
		if t != nil {
			msg.WriteString(fmt.Sprintf(" [task:%s]:\n", t.GetId()))
		} else {
			msg.WriteString(":\n")
		}
		// Keep body text unindented; the timeline renderer already emits a
		// per-item header and legacy readers safely tolerate the compact form.
		msg.WriteString(body)
		return msg.String()
	}

	displayText := buildMessage(content)
	promptText := ""
	if promptContent != "" {
		promptText = buildMessage(promptContent)
	}
	r.config.Timeline.PushTextWithPromptProjection(r.config.AcquireId(), displayText, promptText)
	r.SaveTimeline()
}

// getTimeline 获取时间线信息（可选择限制数量）
func (r *ReAct) getTimeline(lastN int) []*aicommon.TimelineItemOutput {
	return r.config.Timeline.ToTimelineItemOutputLastN(lastN)
}

func (r *ReAct) getTimelineTotal() int {
	return r.config.Timeline.GetIdToTimelineItem().Len()
}

// startQueueProcessor 启动任务队列处理器
func (r *ReAct) startQueueProcessor(ctx context.Context, done chan struct{}) {
	closeDoneOnce := new(sync.Once)
	r.queueProcessor.Do(func() {
		if r.lifecycleWG != nil {
			r.lifecycleWG.Add(1)
		}
		go func() {
			if r.lifecycleWG != nil {
				defer r.lifecycleWG.Done()
			}
			defer func() {
				closeDoneOnce.Do(func() {
					close(done)
				})
			}()
			if r.config.DebugEvent {
				log.Infof("Task queue processor started for ReAct instance: %s", r.config.Id)
			}

			// register hook for queue
			r.taskQueue.AddEnqueueHook(func(task aicommon.AIStatefulTask) (bool, error) {
				r.EmitEnqueueReActTask(task)
				return true, nil
			})
			r.taskQueue.AddDequeueHook(func(task aicommon.AIStatefulTask, reason string) {
				r.EmitDequeueReActTask(task, reason)
			})

			ticker := time.NewTicker(100 * time.Millisecond) // 每100ms检查一次队列
			defer ticker.Stop()
			closeDoneOnce.Do(func() {
				close(done)
			})
			for {
				select {
				case <-ticker.C:
					r.processReActFromQueue()
					r.updateRuntimeTasks()
				case <-ctx.Done():
					if r.config.DebugEvent {
						log.Infof("Task queue processor stopped for ReAct instance: %s", r.config.Id)
					}
					return
				}
			}
		}()
	})
}

// GetQueueInfo 获取任务队列信息
func (r *ReAct) GetQueueInfo() map[string]interface{} {
	queueingTasks := r.taskQueue.GetQueueingTasks()
	taskInfos := make([]map[string]interface{}, 0, len(queueingTasks))

	for _, task := range queueingTasks {
		taskInfos = append(taskInfos, buildQueueTaskInfo(task))
	}

	// currentTask is an internal execution cursor and may temporarily point to
	// an intent task or another nested execution unit. Queue consumers need the
	// stable, queue-owned root task so that queue_info agrees with dequeue_task
	// and can be used to restore the frontend's stop target after attaching.
	currentRootTask := r.getProcessingRuntimeTask()
	var currentTaskInfo map[string]interface{}
	if currentRootTask != nil {
		currentTaskInfo = buildQueueTaskInfo(currentRootTask)
	}

	return map[string]interface{}{
		"queue_name":    r.taskQueue.GetQueueName(),
		"total_tasks":   r.taskQueue.GetQueueingCount(),
		"is_processing": currentRootTask != nil,
		"current_task":  currentTaskInfo,
		"tasks":         taskInfos,
		"queue_empty":   r.taskQueue.IsEmpty(),
	}
}

func buildQueueTaskInfo(task aicommon.AIStatefulTask) map[string]interface{} {
	return map[string]interface{}{
		"id":               task.GetId(),
		"user_input":       task.GetUserInput(),
		"user_input_uuid":  task.GetUserInputUUID(),
		"status":           task.GetStatus(),
		"created_at":       task.GetCreatedAt(),
		"focus_mode":       task.GetFocusMode(),
		"input_source":     task.GetInputSource(),
		"schedule_uuid":    task.GetScheduleUUID(),
		"schedule_name":    task.GetScheduleName(),
		"scheduled_at":     task.GetScheduledAt(),
		"schedule_trigger": task.GetScheduleTrigger(),
		"is_recovery":      task.GetTaskKind() == aicommon.AITaskKind_Recovery,
	}
}

// processInputEvent processes a single input event and triggers ReAct loop
func (r *ReAct) processInputEvent(event *ypb.AIInputEvent) error {
	if r.config.DebugEvent {
		log.Infof("Processing input event: IsFreeInput=%v, IsInteractive=%v", event.IsFreeInput, event.IsInteractiveMessage)
	}

	r.CallMirrorOfAIInputEvent(event)

	if event.IsFreeInput {
		return r.handleFreeValue(event)
	} else if event.IsInteractiveMessage {
		return r.handleInteractiveEvent(event)
	} else if event.IsSyncMessage {
		return r.handleSyncMessage(event)
	}

	log.Warnf("No valid input found in event: %v", event)
	return nil
}

// startEventLoop starts the background event processing loop
func (r *ReAct) startEventLoop(ctx context.Context, done chan struct{}) {
	doneOnce := new(sync.Once)
	if r.lifecycleWG != nil {
		r.lifecycleWG.Add(1)
	}
	if !r.pureInvokerMode {
		r.config.InputEventManager.SetFreeInputCallback(r.handleFreeValue)
	}
	r.RegisterReActSyncEvent()
	r.config.StartEventLoopEx(ctx,
		func() {
			doneOnce.Do(func() {
				if done != nil {
					close(done)
				}
			})
		},
		func() {
			if r.lifecycleWG != nil {
				r.lifecycleWG.Done()
			}
			r.UnRegisterReActSyncEvent()
			r.CloseTrackedBrowserSessions()
			doneOnce.Do(func() {
				if done != nil {
					close(done)
				}
			})
		})
}

func (r *ReAct) IsFinished() bool {
	if r.GetCurrentTask() == nil {
		return true
	}
	return r.GetCurrentTask().IsFinished()
}

func (r *ReAct) Wait() {
	if r.wg == nil {
		return
	}
	r.wg.Wait()
}

// WaitLifecycleStopped waits for the long-lived input and task-queue loops to
// exit after their context is cancelled. It is separate from Wait so existing
// AIEngine/ReAct callers keep the original Wait behavior.
func (r *ReAct) WaitLifecycleStopped() {
	if r == nil || r.lifecycleWG == nil {
		return
	}
	r.lifecycleWG.Wait()
	if r.config != nil {
		r.config.WaitHotPatchLoopStopped()
	}
}

// loadMCPServers loads AI tools from enabled MCP servers asynchronously
func (r *ReAct) loadMCPServers() {
	go func() {
		emitter := r.config.GetEmitter()
		startLoadingPR, startLoadingPW := utils.NewPipe()
		defer startLoadingPW.Close()
		doneLoadingPR, doneLoadingPW := utils.NewPipe()
		defer doneLoadingPW.Close()

		m := new(sync.Mutex)
		promptStartLoadingOnce := utils.NewOnce()
		promptDoneLoadingOnce := utils.NewOnce()

		onToolsListChanged := func(serverName string, tools []*aitool.Tool, removed []string) {
			mng := r.config.GetAiToolManager()
			if mng == nil {
				return
			}
			for _, t := range tools {
				if t != nil {
					mng.OverrideToolByName(t)
				}
			}
			for _, name := range removed {
				mng.RemoveToolByName(name)
			}
			if len(tools) > 0 || len(removed) > 0 {
				log.Infof(
					"MCP server %q tools list_changed: refreshed %d tool(s), removed %d",
					serverName, len(tools), len(removed),
				)
			}
		}

		tools, err := aitool.LoadAllEnabledAIToolsFromMCPServersWithCallback(
			consts.GetGormProfileDatabase(),
			r.config.Ctx,
			func(mcpServer *schema.MCPServer) {
				m.Lock()
				defer m.Unlock()

				promptStartLoadingOnce.Do(func() {
					emitter.EmitDefaultStreamEvent(
						"mcp-loader",
						startLoadingPR,
						r.config.GetRuntimeId(),
					)
					startLoadingPW.WriteString("Loading AI tools from MCP server: ")
				})
				startLoadingPW.WriteString(mcpServer.Name + " ")
			}, func(mcpServer *schema.MCPServer, tools []*aitool.Tool, err error) {
				m.Lock()
				defer m.Unlock()

				if len(tools) > 0 {
					promptDoneLoadingOnce.Do(func() {
						emitter.EmitDefaultStreamEvent(
							"mcp-loader",
							doneLoadingPR,
							r.config.GetRuntimeId(),
						)
						doneLoadingPW.WriteString("Loaded AI tools from MCP servers: ")
					})
					doneLoadingPW.WriteString(fmt.Sprintf("@mcp[%v](%v tools) ", mcpServer.Name, len(tools)))
				}
			}, func(tools []*aitool.Tool, err error) {
				startLoadingPW.Close()
				doneLoadingPW.Close()
			},
			onToolsListChanged,
		)
		if err != nil {
			log.Errorf("load tools failed: %v", err)
		}
		if len(tools) > 0 {
			mng := r.config.GetAiToolManager()
			// Use OverrideToolByName so live MCP tools replace any stub tools
			// that were pre-loaded from the DB cache (AppendTools skips
			// already-registered names, so stubs would block live replacements).
			for _, t := range tools {
				mng.OverrideToolByName(t)
			}
		}
	}()
}

// preloadMCPStubsFromDB loads MCP tool stubs from the DB cache into the tool
// manager synchronously, before the background MCP server connection goroutine
// finishes. This ensures MCP tools appear in the system prompt on the first
// request even when the remote MCP server hasn't responded yet.
// Once loadMCPServers completes, AppendTools will replace each stub with a
// live tool carrying a real network callback (OverrideToolByName semantics via
// AppendTools dedup logic).
func (r *ReAct) preloadMCPStubsFromDB() {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}
	cfgs, err := yakit.GetAllEnabledMCPServerToolConfigs(db)
	if err != nil {
		log.Warnf("preload MCP stubs: failed to query DB: %v", err)
		return
	}
	if len(cfgs) == 0 {
		return
	}

	mng := r.config.GetAiToolManager()
	if mng == nil {
		return
	}

	var stubs []*aitool.Tool
	for _, cfg := range cfgs {
		fullName := fmt.Sprintf("mcp_%s_%s", cfg.ServerName, cfg.ToolName)
		stub := buildinaitools.BuildStubToolFromMCPCachePublic(fullName, cfg)
		if stub != nil {
			stubs = append(stubs, stub)
		}
	}
	if len(stubs) > 0 {
		mng.AppendTools(stubs...)
		log.Infof("preloaded %d MCP tool stubs from DB cache into tool manager", len(stubs))
	}
}

// loadExtraMCPServers mounts session-scoped MCP servers at construction time.
// 每个 server 经 aitool.LoadAIToolsFromMCPServer 取工具（不查 profile DB），
// 并按 AllowedTools 在 client 侧做白名单过滤后 AppendTools。
func (r *ReAct) loadExtraMCPServers(servers []*aicommon.ExtraMCPServer) {
	mng := r.config.GetAiToolManager()
	if mng == nil {
		log.Errorf("cannot mount session-scoped mcp servers: tool manager is nil")
		return
	}
	var mountedNames []string
	for _, s := range servers {
		if s == nil || s.Server == nil {
			continue
		}
		tools, err := aitool.LoadAIToolsFromMCPServer(r.config.Ctx, s.Server, s.AllowedTools)
		if err != nil {
			log.Errorf("load session-scoped mcp server %s failed: %v", s.Server.Name, err)
			continue
		}
		if len(tools) > 0 {
			mng.AppendTools(tools...)
			for _, tool := range tools {
				mountedNames = append(mountedNames, tool.Name)
			}
			log.Infof("session-scoped mcp server %s mounted %d tool(s)", s.Server.Name, len(tools))
		}
	}
	// Restrict unconditionally (deny-all when nothing mounted): if a restricted
	// session's MCP servers are unreachable/empty, falling back to the full
	// builtin/search toolset would be a fail-open, defeating the restriction.
	if r.config.RestrictToolsToExtraMCPServers {
		mng.RestrictToTools(mountedNames...)
		log.Infof("session tools restricted to %d session-scoped mcp tool(s): %v", len(mountedNames), mountedNames)
	}
}

// cycle import issue

func WithBuiltinTools() aicommon.ConfigOption {
	return func(cfg *aicommon.Config) error {

		// Get all builtin tools
		allTools := buildinaitools.GetAllTools()

		// Create a simple AI chat function for the searcher
		aiChatFunc := func(prompt string) (io.Reader, error) {
			response, err := ai.Chat(prompt)
			if err != nil {
				return nil, err
			}
			return strings.NewReader(response), nil
		}

		// Create keyword searcher
		aiToolSearcher := rag_search_tool.NewComprehensiveSearcher[*aitool.Tool](rag_search_tool.AIToolVectorIndexName, aiChatFunc)
		forgeSearcher := rag_search_tool.NewComprehensiveSearcher[*schema.AIForge](rag_search_tool.ForgeVectorIndexName, aiChatFunc)

		log.Infof("Added %d builtin AI tools (search_capabilities is a built-in @action)", len(allTools))
		return aicommon.WithAiToolManagerOptions(
			buildinaitools.WithExtendTools(allTools, true),
			buildinaitools.WithAIToolsSearcher(aiToolSearcher),
			buildinaitools.WithAiForgeSearcher(forgeSearcher))(cfg)
	}
}

// emitArtifactsSummaryToTimeline keeps the legacy call site name but only pins the
// directory for UI visibility. Mechanical directory listings must not enter Timeline
// because Timeline is prompt-visible.
func (r *ReAct) emitArtifactsSummaryToTimeline() {
	artifactsDir := r.config.GetOrCreateWorkDir()
	if artifactsDir == "" {
		return
	}

	// Ensure the artifacts directory is pinned for UI visibility
	if !r.config.IsArtifactsPinned() {
		if r.Emitter != nil {
			r.Emitter.EmitPinDirectory(artifactsDir)
		}
		r.config.SetArtifactsPinned()
	}
}
