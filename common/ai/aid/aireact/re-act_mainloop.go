package aireact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) persistTaskUserInput(task aicommon.AIStatefulTask) {
	if task == nil {
		return
	}
	userInput := strings.TrimSpace(task.GetUserInput())
	if userInput == "" {
		return
	}

	if r.config.Timeline != nil {
		r.config.Timeline.PushUserInteraction(
			aicommon.UserInteractionStage_FreeInput,
			r.config.AcquireId(),
			"",
			userInput,
		)
	}

	quotedHistory, err := r.config.AppendUserInputHistory(userInput, task.GetCreatedAt())
	if err != nil {
		log.Warnf("ReAct: failed to build user input history payload: %v", err)
		return
	}

	if r.config.PersistentSessionId != "" && r.config.GetDB() != nil {
		if err := yakit.UpdateAIAgentRuntimeUserInput(
			r.config.GetDB(), r.config.GetRuntimeId(), quotedHistory); err != nil {
			log.Warnf("ReAct: failed to persist user input to DB: %v", err)
		}
	}
}

const (
	sessionTitleGeneratedKey = "session_title_generated"
	sessionTitleDisableKey   = "disable_session_title_generation"
)

// updateRuntimeTasks 更新 runtime tasks
func (r *ReAct) updateRuntimeTasks() {
	r.UpdateRuntimeTaskMutex.Lock()
	defer r.UpdateRuntimeTaskMutex.Unlock()
	newRuntimeTasks := make([]aicommon.AIStatefulTask, 0)

	for _, task := range r.RuntimeTasks {
		if task.GetStatus() == aicommon.AITaskState_Completed {
			continue
		}
		if task.GetStatus() == aicommon.AITaskState_Aborted {
			continue
		}
		newRuntimeTasks = append(newRuntimeTasks, task)
	}
	r.RuntimeTasks = newRuntimeTasks
}

// processReActFromQueue 处理队列中的下一个任务
func (r *ReAct) processReActFromQueue() {
	if r.taskQueue.IsEmpty() {
		return
	}

	// 如果正在处理任务，直接返回
	if r.IsProcessingReAct() {
		return
	}

	// 从队列获取下一个任务
	log.Infof("start to get first task from queue for ReAct instance: %s", r.config.Id)
	nextTask := r.taskQueue.GetFirst()
	if nextTask == nil {
		return
	}

	r.addRuntimeTask(nextTask)
	r.setCurrentTask(nextTask)
	r.persistTaskUserInput(nextTask)
	nextTask.SetStatus(aicommon.AITaskState_Processing)
	if r.config.DebugEvent {
		log.Infof("Processing task from queue: %s", nextTask.GetId())
	}
	// 异步处理任务
	r.processReActTask(nextTask)
}

// processReActTask 处理单个 Task
func (r *ReAct) processReActTask(task aicommon.AIStatefulTask) {
	skipStatusFallback := utils.NewAtomicBool()
	defer func() {
		r.SaveTimeline()
		r.setCurrentTask(nil) // 处理完成后清除当前任务
		if err := recover(); err != nil {
			log.Errorf("ReAct task processing panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
			task.SetStatus(aicommon.AITaskState_Aborted)
			r.AddToTimeline("error", fmt.Sprintf("Task processing panic: %v", err))
		} else {
			if r.config.DebugEvent {
				log.Infof("Finished processing task: %s", task.GetId())
			}
			if !skipStatusFallback.IsSet() {
				task.SetStatus(aicommon.AITaskState_Completed)
			}
		}
	}()

	// 任务状态应该已经在调用前被设置为处理中，这里不需要重复设置

	// 从任务中提取用户输入
	userInput := task.GetUserInput()

	log.Info("start to handle ensure work directory and session title for ReAct task")
	r.ensureWorkDirectory(userInput) // must be first: creates artifact dir + session title
	r.ensureSessionTitle(userInput)  // will skip if already done by ensureWorkDirectory

	r.currentIteration = 0
	skipStatus, err := r.executeMainLoop(task)
	if err != nil {
		log.Errorf("Task execution failed: %v", err)
		task.SetStatus(aicommon.AITaskState_Aborted)
		r.AddToTimeline("error", fmt.Sprintf("Task execution failed: %v", err))
		return
	}
	if !skipStatus {
		task.SetStatus(aicommon.AITaskState_Completed)
	}
	skipStatusFallback.SetTo(skipStatus)
}

func (r *ReAct) executeMainLoop(task aicommon.AIStatefulTask) (bool, error) {
	parsedQuery, focus, loopOptions := r.selectLoopForTask(task)
	task.SetUserInput(parsedQuery)
	return r.ExecuteLoopTask(focus, task, loopOptions...)
}

func (r *ReAct) selectLoopForTask(task aicommon.AIStatefulTask) (string, string, []reactloops.ReActLoopOption) {
	defaultFocus := r.config.Focus
	userQuery := task.GetUserInput()
	parsedQuery, focus, loopOptions := r.parseLoopDirectives(userQuery, defaultFocus) // 遗留的输入指令解析
	if task.GetFocusMode() != "" {
		focus = task.GetFocusMode() // 任务级别的 focus 模式覆盖
	}
	if focus == "" {
		focus = schema.AI_REACT_LOOP_NAME_DEFAULT
	}
	return parsedQuery, focus, loopOptions
}

func (r *ReAct) parseLoopDirectives(userQuery string, defaultFocus string) (string, string, []reactloops.ReActLoopOption) {
	query := userQuery
	focus := defaultFocus
	var loopOptions []reactloops.ReActLoopOption

	const loopConfigPrefix = "@__LOOP_CONFIG__"
	if token, updatedQuery, ok := extractDirectiveToken(query, loopConfigPrefix); ok {
		query = updatedQuery
		if hasLoopConfigFlag(token, "enable_debug") {
			loopOptions = append(loopOptions, reactloops.WithVar("debug_mode", true))
		}
	}

	const focusPrefix = "@__FOCUS__"
	if token, updatedQuery, ok := extractDirectiveToken(query, focusPrefix); ok {
		query = updatedQuery
		if token != "" && focus == "" {
			focus = token
		}
	}

	return strings.TrimSpace(query), focus, loopOptions
}

func extractDirectiveToken(query string, prefix string) (string, string, bool) {
	idx := strings.Index(query, prefix)
	if idx == -1 {
		return "", query, false
	}
	remaining := query[idx+len(prefix):]
	if remaining == "" {
		return "", strings.TrimSpace(query[:idx]), true
	}
	spaceIdx := strings.Index(remaining, " ")
	if spaceIdx == -1 {
		return remaining, strings.TrimSpace(query[:idx]), true
	}
	return remaining[:spaceIdx], strings.TrimSpace(query[:idx] + remaining[spaceIdx+1:]), true
}

func hasLoopConfigFlag(token string, flag string) bool {
	parts := strings.FieldsFunc(token, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})
	for _, part := range parts {
		if part == flag {
			return true
		}
	}
	return false
}

func (r *ReAct) ExecuteLoopTaskIF(taskTypeName string, task aicommon.AIStatefulTask, options ...any) (bool, error) {
	loopOptions := []reactloops.ReActLoopOption{
		reactloops.WithNoEndLoadingStatus(true),
	}
	for _, option := range options {
		opt, ok := option.(reactloops.ReActLoopOption)
		if ok {
			loopOptions = append(loopOptions, opt)
		}
	}
	return r.ExecuteLoopTask(taskTypeName, task, loopOptions...)
}

func (r *ReAct) ExecuteLoopTask(taskTypeName string, task aicommon.AIStatefulTask, options ...reactloops.ReActLoopOption) (bool, error) {
	memoryFlushBuffer := aicommon.NewMemoryFlushBuffer("react", r.config.TimelineDiffer, nil)
	defer memoryFlushBuffer.Close()
	defaultOptions := reactloops.BasicAICommonConfigOption(r.config)
	defaultOptions = append(defaultOptions,
		reactloops.WithOnAsyncTaskTrigger(func(i *reactloops.LoopAction, task aicommon.AIStatefulTask) {
			r.SetCurrentPlanExecutionTask(task)
		}),
		reactloops.WithOnAsyncTaskFinished(func(task aicommon.AIStatefulTask) {
			r.SetCurrentPlanExecutionTask(nil)
		}),
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
			// Defer the emit decision to after ALL callbacks have completed.
			// This ensures that IgnoreError() calls from loop-specific callbacks
			// (e.g. loop_intent, loop_knowledge_enhance) are respected regardless
			// of callback registration order. Without deferral, this callback might
			// check ShouldIgnoreError() before a later callback has called IgnoreError().
			operator.DeferAfterCallbacks(func() {
				if isDone && reason != nil && !operator.ShouldIgnoreError() {
					r.Emitter.EmitReActFail(fmt.Sprintf("ReAct task execution failed: %v", utils.InterfaceToString(reason)))
				} else if !operator.ShouldIgnoreError() {
					// Only emit success when IgnoreError is not set.
					// Hidden/internal sub-loops (like loop_intent) that set IgnoreError
					// should not emit success/fail to avoid confusing UI signals.
					r.Emitter.EmitReActSuccess("ReAct task execution success")
				}
			})
			operator.DeferAfterCallbacks(func() {
				if r.memoryTriage == nil {
					return
				}
				memoryFlushBuffer.ProcessAsync(aicommon.MemoryFlushSignal{
					Iteration:          iteration,
					Task:               task,
					IsDone:             isDone,
					Reason:             reason,
					ShouldEndIteration: operator.ShouldEndIteration(),
				}, func(payload *aicommon.MemoryFlushPayload, err error) {
					if err != nil {
						log.Warnf("timeline differ call failed: %v", err)
						return
					}
					if payload == nil && !isDone {
						return
					}

					go func() {
						defer func() {
							if err := recover(); err != nil {
								log.Errorf("intelligent memory processing panic: %v", err)
								utils.PrintCurrentGoroutineRuntimeStack()
							}
						}()

						if payload != nil {
							if r.config.DebugEvent {
								log.Infof("processing memory flush[%s] for iteration %d with %d pending diffs (%d bytes)", payload.FlushReason, iteration, payload.PendingIterations, payload.PendingBytes)
							}
							if err := r.memoryTriage.HandleMemory(payload.ContextualInput); err != nil {
								log.Warnf("intelligent memory processing failed: %v", err)
								return
							}
						}

						if isDone && !task.IsAsyncMode() {
							searchResult, err := r.memoryTriage.SearchMemory(task.GetUserInput(), 4096)
							if err != nil {
								log.Warnf("memory search for completed task failed: %v", err)
								return
							}

							if len(searchResult.Memories) > 0 {
								log.Infof("found %d relevant memories for completed task %s (total: %d tokens)", len(searchResult.Memories), task.GetId(), searchResult.ContentTokens)
								if r.config.DebugEvent {
									log.Infof("memory search summary: %s", searchResult.SearchSummary)
								}
							} else if r.config.DebugEvent {
								log.Infof("no relevant memories found for completed task %s", task.GetId())
							}
						}
					}()
				})
			})
		}),
		reactloops.WithAllowAIForge(r.config.EnablePlanAndExec),
	)

	defaultOptions = append(defaultOptions, options...)

	mainloop, err := reactloops.CreateLoopByName(
		taskTypeName, r,
		defaultOptions...,
	)
	if err != nil {
		return false, utils.Errorf("failed to create main loop runtime instance: %v", err)
	}

	if r.GetCurrentPlanExecutionTask() != nil {
		// have async plan execution task running, disable plan and exec in main loop
		mainloop.RemoveAction(schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION)
		mainloop.RemoveAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT)
	}
	err = mainloop.ExecuteWithExistedTask(task)
	if err != nil {
		return false, err
	}
	return task.IsAsyncMode(), nil
}

func init() {
	aicommon.RegisterDefaultAIRuntimeInvoker(BuildReActInvoker)
}

const (
	workDirCreatedKey = "work_dir_created"
)

// sanitizeFolderName cleans a string for use as a filesystem folder name.
// Keeps ASCII letters, digits, underscores, hyphens. Replaces everything else with underscore.
// Converts to lowercase. Truncates to maxLen characters.
func sanitizeFolderName(name string, maxLen int) string {
	var result []rune
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result = append(result, r)
		} else if r == ' ' || r == '/' || r == '\\' {
			result = append(result, '_')
		} else if unicode.Is(unicode.Han, r) {
			// keep Chinese characters for readability
			result = append(result, r)
		}
		// skip other characters
	}
	s := string(result)
	// collapse multiple underscores
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	if s == "" {
		return "session"
	}
	return s
}

// ensureWorkDirectory lazily creates the artifact working directory with a semantic name.
// This is called at the start of processReActTask, after user input is available.
// It uses LiteForge to generate a meaningful folder name, falling back to a generic name.
// It also generates the session title in the same LiteForge call to save overhead.
func (r *ReAct) ensureWorkDirectory(userInput string) {
	cfg := r.config
	if cfg == nil {
		return
	}

	if cfg.IsWorkDirReady() {
		// WorkDir was restored from persistent session or set by parent config.
		// Still need to: update the new runtime's DB record with the restored WorkDir,
		// and initialize the artifacts filesystem for this ReAct instance.
		dirPath := cfg.GetOrCreateWorkDir()
		r.artifacts = filesys.NewRelLocalFs(dirPath)
		yakit.UpdateAIAgentRuntimeWorkDir(cfg.GetDB(), cfg.GetRuntimeId(), dirPath, "")
		return
	}

	if cfg.GetConfigBool(workDirCreatedKey) {
		return
	}
	cfg.SetConfig(workDirCreatedKey, true)

	trimmedInput := strings.TrimSpace(userInput)

	shortUuid := cfg.GetRuntimeId()
	if len(shortUuid) > 5 {
		shortUuid = shortUuid[:5]
	}
	dateStr := time.Now().Format("20060102")

	var folderName string
	var sessionTitle string

	shouldTrySessionTitle := true
	if r.config.PersistentSessionId != "" && cfg.GetDB() != nil {
		initialized, err := yakit.IsAISessionTitleInitialized(cfg.GetDB(), r.config.PersistentSessionId)
		if err == nil && initialized {
			shouldTrySessionTitle = false
		}
	}

	// try LiteForge to generate both folder_name and session_title
	// use a tight timeout to avoid blocking the main flow
	if trimmedInput != "" && !cfg.GetConfigBool(sessionTitleDisableKey) && cfg.OriginalAICallback != nil {
		func() {
			defer func() {
				if err := recover(); err != nil {
					log.Warnf("generate semantic folder name panic: %v", err)
				}
			}()

			// use a timeout context to avoid blocking the main flow
			liteForgeCtx, cancel := context.WithTimeout(cfg.GetContext(), 10*time.Second)
			defer cancel()

			prompt, err := r.promptManager.GenerateRequireConversationTitlePrompt(r.DumpTimeline(), trimmedInput)
			if err != nil {
				log.Warnf("generate semantic folder name prompt failed: %v", err)
				return
			}

			toolOptions := []aitool.ToolOption{
				aitool.WithStringParam("folder_name", aitool.WithParam_Description("Short filesystem-safe folder name in snake_case English, describing the task purpose, e.g. sql_injection_scan, http_flow_analysis"), aitool.WithParam_MaxLength(30), aitool.WithParam_Required(true)),
			}
			if shouldTrySessionTitle {
				toolOptions = append(toolOptions, aitool.WithStringParam("session_title", aitool.WithParam_Description("Concise session title for display"), aitool.WithParam_MaxLength(50), aitool.WithParam_Required(true)))
			}
			action, err := r.InvokeSpeedPriorityLiteForge(liteForgeCtx, "session-init-generator", prompt, toolOptions)
			if err != nil {
				log.Warnf("generate semantic folder name failed: %v", err)
				return
			}

			if fn := strings.TrimSpace(action.GetString("folder_name")); fn != "" {
				folderName = sanitizeFolderName(fn, 30)
			}
			if shouldTrySessionTitle {
				if st := strings.TrimSpace(action.GetString("session_title")); st != "" {
					sessionTitle = st
				}
			}
		}()
	}

	// load existing title for restored session so UI gets it even when generation is skipped.
	if cfg.GetConfigString("session_title", "") == "" && r.config.PersistentSessionId != "" && cfg.GetDB() != nil {
		if meta, err := yakit.GetAISessionMetaBySessionID(cfg.GetDB(), r.config.PersistentSessionId); err == nil {
			if existing := strings.TrimSpace(meta.Title); existing != "" {
				cfg.SetConfig("session_title", existing)
				cfg.SetSessionTitle(existing)
				cfg.SetConfig(sessionTitleGeneratedKey, true)
				r.Emitter.EmitSessionTitle(existing)
			}
		}
	}

	// build the final directory name: {dbId}_{semanticOrSession}_{date}_{shortUuid}
	if folderName == "" {
		folderName = "session"
	}
	dirName := fmt.Sprintf("%d_%s_%s_%s",
		cfg.DatabaseRecordID,
		folderName,
		dateStr,
		shortUuid,
	)

	// create the directory
	dirPath := consts.TempAIDir(dirName)
	cfg.SetWorkDir(dirPath)
	// Also set Workdir (capital W) so ConvertConfigToOptions can propagate it
	// to child configs (Coordinator, P&E sub-invokers, forge executions).
	// Without this, child configs created via ConvertConfigToOptions would not
	// inherit the semantic work directory and would create their own fallback dirs.
	cfg.Workdir = dirPath

	// initialize artifacts filesystem
	r.artifacts = filesys.NewRelLocalFs(dirPath)

	// emit pin directory - at this point the name is final and meaningful
	r.Emitter.EmitPinDirectory(dirPath)

	// update DB record with work dir and semantic label
	yakit.UpdateAIAgentRuntimeWorkDir(cfg.GetDB(), cfg.GetRuntimeId(), dirPath, folderName)

	// if we got a session title from LiteForge, persist title only when not initialized.
	if sessionTitle != "" && r.config.PersistentSessionId != "" && cfg.GetDB() != nil {
		updated, err := yakit.InitAISessionTitleIfNeeded(cfg.GetDB(), r.config.PersistentSessionId, sessionTitle)
		if err != nil {
			log.Warnf("init ai session title failed: %v", err)
		}
		if updated {
			cfg.SetConfig("session_title", sessionTitle)
			cfg.SetSessionTitle(sessionTitle)
			cfg.SetConfig(sessionTitleGeneratedKey, true)
			r.Emitter.EmitSessionTitle(sessionTitle)
		}
	} else if sessionTitle != "" {
		cfg.SetConfig("session_title", sessionTitle)
		cfg.SetSessionTitle(sessionTitle)
		cfg.SetConfig(sessionTitleGeneratedKey, true)
		r.Emitter.EmitSessionTitle(sessionTitle)
	}

	log.Infof("work directory created: %s (semantic: %s)", dirPath, folderName)
}

func (r *ReAct) ensureSessionTitle(userInput string) {
	cfg := r.GetConfig()
	if cfg == nil {
		return
	}

	if cfg.GetConfigBool(sessionTitleDisableKey) {
		return
	}

	if r.config.PersistentSessionId != "" && cfg.GetDB() != nil {
		initialized, err := yakit.IsAISessionTitleInitialized(cfg.GetDB(), r.config.PersistentSessionId)
		if err == nil && initialized {
			return
		}
	}

	if cfg.GetConfigString("session_title", "") != "" || cfg.GetConfigBool(sessionTitleGeneratedKey) {
		return
	}

	cfg.SetConfig(sessionTitleGeneratedKey, true)

	trimmedInput := strings.TrimSpace(userInput)
	if trimmedInput == "" {
		return
	}

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Warnf("generate session title panic: %v", err)
			}
		}()

		prompt, err := r.promptManager.GenerateRequireConversationTitlePrompt(r.DumpTimeline(), trimmedInput)
		if err != nil {
			log.Errorf("generate session title prompt failed: %v", err)
			return
		}

		log.Info("start to handle session-title-generator,  using speed-priority LiteForge for session title generation")
		action, err := r.InvokeSpeedPriorityLiteForge(cfg.GetContext(), "session-title-generator", prompt, []aitool.ToolOption{
			aitool.WithStringParam("session_title", aitool.WithParam_Description("Concise session title"), aitool.WithParam_MaxLength(50), aitool.WithParam_Required(true)),
		})
		if err != nil {
			log.Warnf("generate session title failed: %v", err)
			return
		}

		sessionTitle := strings.TrimSpace(action.GetString("session_title"))
		if sessionTitle == "" {
			return
		}

		if r.config.PersistentSessionId != "" && cfg.GetDB() != nil {
			updated, err := yakit.InitAISessionTitleIfNeeded(cfg.GetDB(), r.config.PersistentSessionId, sessionTitle)
			if err != nil {
				log.Warnf("init ai session title failed: %v", err)
				return
			}
			if !updated {
				return
			}
		}
		cfg.SetConfig("session_title", sessionTitle)
		r.config.SetSessionTitle(sessionTitle)
		r.Emitter.EmitSessionTitle(sessionTitle)
	}()
}

func BuildReActInvoker(ctx context.Context, options ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
	cfg := aicommon.NewConfig(ctx, options...)
	// artifacts directory is lazily created when user input arrives (ensureWorkDirectory)
	invoker := &ReAct{
		config:               cfg,
		Emitter:              cfg.Emitter, // Use the emitter from config
		taskQueue:            NewTaskQueue("react-main-queue"),
		mirrorOfAIInputEvent: make(map[string]func(*ypb.AIInputEvent)),
		saveTimelineThrottle: utils.NewThrottleEx(3, true, true),
		artifacts:            nil, // lazy: created in ensureWorkDirectory
		wg:                   new(sync.WaitGroup),
		pureInvokerMode:      true,
	}

	if cfg.MemoryTriage != nil {
		invoker.memoryTriage = cfg.MemoryTriage
	} else {
		memoryTriageId := cfg.MemoryTriageId
		if memoryTriageId == "" {
			memoryTriageId = "default"
		}
		var err error
		invoker.memoryTriage, err = aimem.NewAIMemory(memoryTriageId, aimem.WithInvoker(invoker))
		if err != nil {
			return nil, utils.Errorf("create memory triage failed: %v", err)
		}
		invoker.config.MemoryTriage = invoker.memoryTriage
	}

	log.Infof("memory triage id: %s", invoker.memoryTriage.GetSessionID())
	if cfg.Timeline == nil {
		cfg.Timeline = aicommon.NewTimeline(cfg, nil)
	}
	if cfg.TimelineDiffer == nil {
		cfg.TimelineDiffer = aicommon.NewTimelineDiffer(cfg.Timeline)
	}
	cfg.EnhanceKnowledgeManager.SetEmitter(cfg.Emitter)
	// Initialize prompt manager (workdir does not depend on artifacts, which is lazy)
	workdir := cfg.Workdir
	if workdir == "" {
		workdir = filepath.Join(consts.GetDefaultYakitBaseDir(), "code")
		if utils.GetFirstExistedFile(workdir) == "" {
			os.MkdirAll(workdir, os.ModePerm)
		}
	}
	invoker.promptManager = NewPromptManager(invoker, workdir)

	// Register pending context providers
	invoker.promptManager.cpm = cfg.ContextProviderManager

	// EmitPinDirectory is deferred to ensureWorkDirectory when user input arrives

	// Start the event loop in background
	mainloopDone := make(chan struct{})
	invoker.startEventLoop(cfg.Ctx, mainloopDone)
	select {
	case <-cfg.Ctx.Done():
		return nil, utils.Errorf("context canceled before ReAct invoker started")
	case <-mainloopDone:
	}
	return invoker, nil
}
