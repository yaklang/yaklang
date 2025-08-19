package aireact

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 同步类型常量
const (
	SYNC_TYPE_QUEUE_INFO = "queue_info"
	SYNC_TYPE_TIMELINE   = "timeline"
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

type ReAct struct {
	config        *ReActConfig
	promptManager *PromptManager
	*aicommon.Emitter

	// 任务队列相关
	currentTask    *Task        // 当前正在处理的任务
	taskQueue      *TaskQueue   // 任务队列
	queueProcessor sync.Once    // 确保队列处理器只启动一次
	queueMutex     sync.RWMutex // 保护队列相关状态
	isProcessing   bool         // 是否正在处理任务

	// 时间线相关
	timeline *aicommon.Timeline

	mirrorMutex          sync.RWMutex
	mirrorOfAIInputEvent map[string]func(*ypb.AIInputEvent)
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

func NewReAct(opts ...Option) (*ReAct, error) {
	cfg := NewReActConfig(context.Background(), opts...)

	react := &ReAct{
		config:               cfg,
		Emitter:              cfg.Emitter, // Use the emitter from config
		taskQueue:            NewTaskQueue("react-main-queue"),
		isProcessing:         false,
		timeline:             aicommon.NewTimeline(cfg.timelineLimit, cfg, nil),
		mirrorOfAIInputEvent: make(map[string]func(*ypb.AIInputEvent)),
	}

	// Initialize prompt manager
	react.promptManager = NewPromptManager(react)
	cfg.promptManager = react.promptManager

	// Initialize memory with AI capability
	if cfg.memory != nil && cfg.aiCallback != nil {
		// Set the AI instance for memory timeline
		cfg.memory.SetTimelineAI(cfg)

		// Store tools function
		cfg.memory.StoreTools(func() []*aitool.Tool {
			if cfg.aiToolManager == nil {
				return []*aitool.Tool{}
			}
			tools, err := cfg.aiToolManager.GetEnableTools()
			if err != nil {
				return []*aitool.Tool{}
			}
			return tools
		})
	}

	// Start the event loop in background
	react.startEventLoop(cfg.ctx)

	// Start queue processor in background
	react.startQueueProcessor(cfg.ctx)

	return react, nil
}

// UpdateDebugMode dynamically updates the debug mode settings
func (r *ReAct) UpdateDebugMode(debug bool) {
	r.config.mu.Lock()
	defer r.config.mu.Unlock()
	r.config.debugEvent = debug
	r.config.debugPrompt = debug
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

	r.config.eventInputChan.SafeFeed(event)
	return nil
}

// addToTimeline 添加条目到时间线
func (r *ReAct) addToTimeline(entryType, content, taskID string) {
	r.timeline.PushText(
		r.config.AcquireId(),
		"[type:%v] [id:%v] content: %v",
		entryType, taskID, content,
	)
}

// getTimeline 获取时间线信息（可选择限制数量）
func (r *ReAct) getTimeline(lastN int) []*aicommon.TimelineItemOutput {
	return r.timeline.ToTimelineItemOutputLastN(lastN)
}

func (r *ReAct) getTimelineTotal() int {
	return r.timeline.GetIdToTimelineItem().Len()
}

// startQueueProcessor 启动任务队列处理器
func (r *ReAct) startQueueProcessor(ctx context.Context) {
	r.queueProcessor.Do(func() {
		go func() {
			if r.config.debugEvent {
				log.Infof("Task queue processor started for ReAct instance: %s", r.config.id)
			}

			ticker := time.NewTicker(100 * time.Millisecond) // 每100ms检查一次队列
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					r.processNextTaskFromQueue()
				case <-ctx.Done():
					if r.config.debugEvent {
						log.Infof("Task queue processor stopped for ReAct instance: %s", r.config.id)
					}
					return
				}
			}
		}()
	})
}

// processNextTaskFromQueue 处理队列中的下一个任务
func (r *ReAct) processNextTaskFromQueue() {
	r.queueMutex.Lock()
	// 如果正在处理任务，直接返回
	if r.isProcessing {
		r.queueMutex.Unlock()
		return
	}

	// 从队列获取下一个任务
	nextTask := r.taskQueue.GetFirst()
	if nextTask == nil {
		r.queueMutex.Unlock()
		return
	}

	// 标记正在处理并设置当前任务
	r.isProcessing = true
	r.currentTask = nextTask
	r.queueMutex.Unlock()

	// 更新任务状态为处理中
	nextTask.SetStatus(string(TaskStatus_Processing))
	r.addToTimeline("processing", fmt.Sprintf("Started processing task from queue: %s", nextTask.GetUserInput()), nextTask.GetId())

	if r.config.debugEvent {
		log.Infof("Processing task from queue: %s", nextTask.GetId())
	}

	// 异步处理任务
	go r.processTask(nextTask)
}

// processTask 处理单个 Task
func (r *ReAct) processTask(task *Task) {
	defer func() {
		r.queueMutex.Lock()
		r.isProcessing = false
		r.currentTask = nil // 清空当前任务
		r.queueMutex.Unlock()

		if r.config.debugEvent {
			log.Infof("Task processing completed: %s", task.GetId())
		}
	}()

	// 任务状态应该已经在调用前被设置为处理中，这里不需要重复设置

	// 从任务中提取用户输入
	userInput := task.GetUserInput()

	// 重置会话状态
	r.config.mu.Lock()
	r.config.finished = false
	r.config.currentIteration = 0
	// 为新任务重置内存

	// 重新初始化内存
	if r.config.memory == nil {
		r.config.memory = aid.GetDefaultMemory()
		r.config.memory.SetTimelineAI(r.config)
		r.config.memory.StoreTools(func() []*aitool.Tool {
			if r.config.aiToolManager == nil {
				return []*aitool.Tool{}
			}
			tools, err := r.config.aiToolManager.GetEnableTools()
			if err != nil {
				return []*aitool.Tool{}
			}
			return tools
		})
	}
	r.config.mu.Unlock()

	// 执行主循环
	err := r.executeMainLoop(userInput)
	if err != nil {
		log.Errorf("Task execution failed: %v", err)
		task.SetStatus(string(TaskStatus_Aborted))
		r.addToTimeline("error", fmt.Sprintf("Task execution failed: %v", err), task.GetId())
	} else {
		task.SetStatus(string(TaskStatus_Completed))
		r.addToTimeline("completed", fmt.Sprintf("Task completed: %s", task.GetUserInput()), task.GetId())
	}
}

// GetQueueInfo 获取任务队列信息
func (r *ReAct) GetQueueInfo() map[string]interface{} {
	r.queueMutex.RLock()
	defer r.queueMutex.RUnlock()

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
		"is_processing": r.isProcessing,
		"tasks":         taskInfos,
		"queue_empty":   r.taskQueue.IsEmpty(),
	}
}

// processInputEvent processes a single input event and triggers ReAct loop
func (r *ReAct) processInputEvent(event *ypb.AIInputEvent) error {
	if r.config.debugEvent {
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
func (r *ReAct) startEventLoop(ctx context.Context) {
	r.config.startInputEventOnce.Do(func() {
		go func() {
			if r.config.debugEvent {
				log.Infof("ReAct event loop started for instance: %s", r.config.id)
			}

			for {
				select {
				case event, ok := <-r.config.eventInputChan.OutputChannel():
					if !ok {
						log.Errorf("ReAct event input channel closed for instance: %s", r.config.id)
						return
					}
					if event == nil {
						continue
					}

					if r.config.debugEvent {
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
					if r.config.debugEvent {
						log.Infof("ReAct event loop stopped for instance: %s", r.config.id)
					}
					return
				}
			}
		}()
	})
}
