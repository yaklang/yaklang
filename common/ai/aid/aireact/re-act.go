package aireact

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aimem/memory_type"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/rag/rag_search_tool"
	"github.com/yaklang/yaklang/common/aiforge"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/ai/aid/aimem"

	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactinit"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 同步类型常量
const (
	SYNC_TYPE_QUEUE_INFO                = "queue_info"
	SYNC_TYPE_TIMELINE                  = "timeline"
	SYNC_TYPE_KNOWLEDGE                 = "enhance_knowledge"
	SYNC_TYPE_UPDATE_CONFIG             = "update_config"
	SYNC_TYPE_MEMORY_CONTEXT            = "memory_sync"
	SYNC_TYPE_REACT_CANCEL_CURRENT_TASK = "react_cancel_current_task"
	SYNC_TYPE_REACT_JUMP_QUEUE          = "react_jump_queue"
	SYNC_TYPE_REACT_REMOVE_TASK         = "react_remove_task"
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

var _ aicommon.AIInvokeRuntime = (*ReAct)(nil)

type ReAct struct {
	*aicommon.Emitter

	// runtime fields
	cumulativeSummary            string // Cumulative summary for conversation memory
	cumulativeSummaryHandleQueue *chanx.UnlimitedChan[func() string]

	currentIteration            int
	currentUserInteractiveCount int64 // 当前用户交互次数

	config        *aicommon.Config
	promptManager *PromptManager

	inputChanx *chanx.UnlimitedChan[*ypb.AIInputEvent]

	// 任务队列相关
	currentTask          aicommon.AIStatefulTask // 当前正在处理的任务
	currentPlanExecution aicommon.AIStatefulTask
	taskQueue            *TaskQueue // 任务队列
	queueProcessor       sync.Once  // 确保队列处理器只启动一次
	mirrorMutex          sync.RWMutex
	mirrorOfAIInputEvent map[string]func(*ypb.AIInputEvent)

	saveTimelineThrottle func(func())
	artifacts            *filesys.RelLocalFs

	wg             *sync.WaitGroup
	timelineDiffer *aicommon.TimelineDiffer
	memoryTriage   aicommon.MemoryTriage
	memoryPool     *omap.OrderedMap[string, *memory_type.MemoryEntity]
}

func (r *ReAct) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	return r.promptManager.GetBasicPromptInfo(tools)
}

var _ aicommon.AIInvokeRuntime = (*ReAct)(nil)

const SKIP_AI_REVIEW = "skip_ai_review"

func (r *ReAct) GetConfig() aicommon.AICallerConfigIf {
	return r.config
}

func (r *ReAct) SaveTimeline() {
	if r.config.PersistentSessionId == "" {
		return
	}
	r.saveTimelineThrottle(func() {
		ins := r.config.Timeline
		if ins == nil {
			return
		}
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

func (r *ReAct) PushCumulativeSummaryHandle(f func() string) {
	if r == nil {
		return
	}
	if r.cumulativeSummaryHandleQueue != nil {
		r.cumulativeSummaryHandleQueue.SafeFeed(f)
	}
	return
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
	r.currentPlanExecution = t
}

func (r *ReAct) GetCurrentPlanExecutionTask() aicommon.AIStatefulTask {
	if r == nil {
		return nil
	}
	if r.currentPlanExecution == nil {
		return nil
	}
	return r.currentPlanExecution
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
	opts = append(opts, aicommon.WithAIBlueprintManager(aiforge.NewForgeFactory()))
	cfg := aicommon.NewConfig(context.Background(), opts...)
	dirname := consts.TempAIDir(cfg.GetRuntimeId())
	if existed, _ := utils.PathExists(dirname); !existed {
		return nil, utils.Errorf("temp ai dir %s not existed", dirname)
	}
	react := &ReAct{
		config:               cfg,
		Emitter:              cfg.Emitter, // Use the emitter from config
		taskQueue:            NewTaskQueue("react-main-queue"),
		mirrorOfAIInputEvent: make(map[string]func(*ypb.AIInputEvent)),
		saveTimelineThrottle: utils.NewThrottleEx(3, true, true),
		artifacts:            filesys.NewRelLocalFs(dirname),
		wg:                   new(sync.WaitGroup),
		memoryPool:           omap.NewOrderedMap(make(map[string]*memory_type.MemoryEntity)),
	}

	if cfg.MemoryTriage != nil {
		react.memoryTriage = cfg.MemoryTriage
	} else {
		var err error
		react.memoryTriage, err = aimem.NewAIMemory("default", aimem.WithInvoker(react))
		if err != nil {
			return nil, utils.Errorf("create memory triage failed: %v", err)
		}
	}

	react.timelineDiffer = aicommon.NewTimelineDiffer(cfg.Timeline)
	cfg.EnhanceKnowledgeManager.SetEmitter(cfg.Emitter)
	// Initialize prompt manager
	workdir := cfg.Workdir
	if workdir == "" {
		workdir, _ = react.artifacts.Getwd()
		if workdir == "" {
			workdir = filepath.Join(consts.GetDefaultBaseHomeDir(), "code")
			if utils.GetFirstExistedFile(workdir) == "" {
				os.MkdirAll(workdir, os.ModePerm)
			}
		}
	}
	react.promptManager = NewPromptManager(react, workdir)

	// Register pending context providers
	for _, entry := range cfg.PendingContextProviders {
		if entry.Traced {
			react.promptManager.cpm.RegisterTracedContent(entry.Name, entry.Provider)
		} else {
			react.promptManager.cpm.Register(entry.Name, entry.Provider)
		}
	}
	// Clear pending list after registration
	cfg.PendingContextProviders = nil

	// Start the event loop in background
	mainloopDone := make(chan struct{})
	react.startEventLoop(cfg.Ctx, mainloopDone)
	<-mainloopDone // Ensure the event loop has started

	// Start queue processor in background
	done := make(chan struct{})
	react.startQueueProcessor(cfg.Ctx, done)
	<-done // Ensure the queue processor has started

	err := yakit.CreateOrUpdateAIAgentRuntime(
		react.config.GetDB(), &schema.AIAgentRuntime{
			Uuid:              cfg.GetRuntimeId(),
			Name:              "[re-act-runtime]",
			Seq:               cfg.IdSequence,
			TypeName:          schema.AIAgentRuntimeType_ReAct,
			PersistentSession: cfg.PersistentSessionId,
		},
	)
	if err != nil {
		return nil, err
	}

	wd, err := react.artifacts.Getwd()
	if err != nil {
		return nil, err
	}
	react.Emitter.EmitPinDirectory(wd)

	return react, nil
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

// AddToTimeline 添加条目到时间线
func (r *ReAct) AddToTimeline(entryType, content string) {
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
	msg.WriteString(utils.PrefixLines(content, "  "))
	r.config.Timeline.PushText(r.config.AcquireId(), msg.String())
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
		go func() {
			r.cumulativeSummaryHandleQueue = chanx.NewUnlimitedChan[func() string](ctx, 100)
			for {
				select {
				case f, ok := <-r.cumulativeSummaryHandleQueue.OutputChannel():
					if !ok {
						return
					}
					if f != nil {
						s := f()
						if s != "" {
							r.cumulativeSummary = s
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		go func() {
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
		taskInfo := map[string]interface{}{
			"id":         task.GetId(),
			"user_input": task.GetUserInput(),
			"status":     task.GetStatus(),
			"created_at": task.GetCreatedAt(),
		}

		taskInfos = append(taskInfos, taskInfo)
	}

	return map[string]interface{}{
		"queue_name":    r.taskQueue.GetQueueName(),
		"total_tasks":   r.taskQueue.GetQueueingCount(),
		"is_processing": r.IsProcessingReAct(),
		"tasks":         taskInfos,
		"queue_empty":   r.taskQueue.IsEmpty(),
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

	r.config.StartInputEventOnce.Do(func() {
		go func() {
			defer func() {
				doneOnce.Do(func() {
					if done != nil {
						close(done)
					}
				})
			}()
			if r.config.DebugEvent {
				log.Infof("ReAct event loop started for instance: %s", r.config.Id)
			}

			doneOnce.Do(func() {
				if done != nil {
					close(done)
				}
			})
			for {
				select {
				case event, ok := <-r.config.EventInputChan.OutputChannel():
					if !ok {
						log.Errorf("ReAct event input channel closed for instance: %s", r.config.Id)
						return
					}
					if event == nil {
						continue
					}

					if r.config.DebugEvent {
						log.Infof("ReAct event loop processing event: IsFreeInput=%v, IsInteractive=%v",
							event.IsFreeInput, event.IsInteractiveMessage)
					}

					// Process the event in the background (non-blocking)
					go func(event *ypb.AIInputEvent) {
						if err := r.processInputEvent(event); err != nil {
							log.Errorf("ReAct event processing failed: %v", err)
						}
					}(event)
				case <-ctx.Done():
					if r.config.DebugEvent {
						log.Infof("ReAct event loop stopped for instance: %s", r.config.Id)
					}
					return
				}
			}
		}()
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
		// Enable tool search functionality
		log.Infof("Added %d builtin AI tools with search capability", len(allTools))
		return aicommon.WithAiToolManagerOptions(
			buildinaitools.WithExtendTools(allTools, true),
			buildinaitools.WithSearchToolEnabled(true),
			buildinaitools.WithForgeSearchToolEnabled(true),
			buildinaitools.WithAIToolsSearcher(aiToolSearcher),
			buildinaitools.WithAiForgeSearcher(forgeSearcher))(cfg)
	}
}
