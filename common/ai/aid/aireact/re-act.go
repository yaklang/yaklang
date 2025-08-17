package aireact

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ReAct 队列同步类型
type ReactSyncType string

const (
	REACT_SYNC_TYPE_QUEUE_INFO ReactSyncType = "queue_info"
	REACT_SYNC_TYPE_TIMELINE   ReactSyncType = "timeline"
)

// ReactInputEvent ReAct 专用的输入事件结构
type ReactInputEvent struct {
	Id string

	// 是否是 ReAct 同步信息
	IsReActSyncInfo bool
	// ReAct 同步类型
	ReactSyncType ReactSyncType

	// 兼容原有的交互和参数
	IsInteractive bool
	Params        aitool.InvokeParams
}

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

// TimelineEntry 时间线条目
type TimelineEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"` // "input", "thought", "action", "observation", "result"
	Content   string                 `json:"content"`
	TaskID    string                 `json:"task_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type ReAct struct {
	config        *ReActConfig
	promptManager *PromptManager
	*aicommon.Emitter

	// 任务队列相关
	taskQueue      *TaskQueue   // 任务队列
	currentTask    *Task        // 当前正在处理的任务
	queueProcessor sync.Once    // 确保队列处理器只启动一次
	queueMutex     sync.RWMutex // 保护队列相关状态
	isProcessing   bool         // 是否正在处理任务

	// 时间线相关
	timeline      []*TimelineEntry // 事件时间线
	timelineMutex sync.RWMutex     // 保护时间线
}

func NewReAct(opts ...Option) (*ReAct, error) {
	cfg := NewReActConfig(context.Background(), opts...)

	react := &ReAct{
		config:       cfg,
		Emitter:      cfg.Emitter, // Use the emitter from config
		taskQueue:    NewTaskQueue("react-main-queue"),
		isProcessing: false,
		timeline:     make([]*TimelineEntry, 0),
	}

	// Initialize prompt manager
	react.promptManager = NewPromptManager(react)

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
func (r *ReAct) SendInputEvent(event *ypb.AIInputEvent) error {
	if event == nil {
		return fmt.Errorf("input event is nil")
	}

	// 对于交互式消息，直接发送到事件通道处理
	if event.IsInteractiveMessage {
		if r.config.eventInputChan == nil {
			return fmt.Errorf("event input channel is not initialized")
		}

		select {
		case r.config.eventInputChan <- event:
			if r.config.debugEvent {
				log.Infof("Interactive event sent to channel: ID=%s", event.InteractiveId)
			}
			return nil
		default:
			return fmt.Errorf("event input channel is full, interactive event dropped")
		}
	}

	// 对于普通输入，创建任务并添加到队列
	if event.IsFreeInput {
		return r.enqueueTask(event)
	}

	return fmt.Errorf("unsupported event type")
}

// SendReActSyncRequest 发送 ReAct 专用的同步请求
func (r *ReAct) SendReActSyncRequest(syncType ReactSyncType, params aitool.InvokeParams) error {
	switch syncType {
	case REACT_SYNC_TYPE_QUEUE_INFO:
		// 获取队列信息并通过事件发送
		queueInfo := r.GetQueueInfo()

		// 通过 Emitter 发送队列信息事件
		r.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "queue_info", queueInfo)
		return nil

	case REACT_SYNC_TYPE_TIMELINE:
		// 获取时间线信息
		limit := 20 // 默认限制
		if params != nil {
			if l, ok := params["limit"].(int); ok && l > 0 {
				limit = l
			}
		}

		timeline := r.getTimeline(limit)
		timelineInfo := map[string]interface{}{
			"total_entries": len(r.timeline),
			"limit":         limit,
			"entries":       timeline,
		}

		// 通过 Emitter 发送时间线信息事件
		r.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "timeline", timelineInfo)
		return nil

	default:
		return fmt.Errorf("unsupported ReAct sync type: %s", string(syncType))
	}
}

// enqueueTask 将输入事件转换为任务并添加到队列
func (r *ReAct) enqueueTask(event *ypb.AIInputEvent) error {
	// 创建基于aireact.Task的任务
	task := NewTask(
		fmt.Sprintf("react-task-%d", time.Now().UnixNano()),
		event.FreeInput,
	)

	// 添加到队列
	err := r.taskQueue.Append(task)
	if err != nil {
		log.Errorf("Failed to add task to queue: %v", err)
		return fmt.Errorf("failed to enqueue task: %v", err)
	}

	// 添加到时间线
	r.addToTimeline("input", fmt.Sprintf("Task enqueued: %s", event.FreeInput), task.GetId())

	if r.config.debugEvent {
		log.Infof("Task enqueued: %s with input: %s", task.GetId(), event.FreeInput)
	}

	return nil
}

// addToTimeline 添加条目到时间线
func (r *ReAct) addToTimeline(entryType, content, taskID string) {
	r.timelineMutex.Lock()
	defer r.timelineMutex.Unlock()

	entry := &TimelineEntry{
		Timestamp: time.Now(),
		Type:      entryType,
		Content:   content,
		TaskID:    taskID,
	}

	r.timeline = append(r.timeline, entry)
}

// getTimeline 获取时间线信息（可选择限制数量）
func (r *ReAct) getTimeline(limit int) []*TimelineEntry {
	r.timelineMutex.RLock()
	defer r.timelineMutex.RUnlock()

	if limit <= 0 || len(r.timeline) <= limit {
		// 返回所有条目的副本
		result := make([]*TimelineEntry, len(r.timeline))
		copy(result, r.timeline)
		return result
	}

	// 返回最后 limit 个条目
	start := len(r.timeline) - limit
	result := make([]*TimelineEntry, limit)
	copy(result, r.timeline[start:])
	return result
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

	// 标记正在处理
	r.isProcessing = true
	r.queueMutex.Unlock()

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
		r.queueMutex.Unlock()

		if r.config.debugEvent {
			log.Infof("Task processing completed: %s", task.GetId())
		}
	}()

	// 设置任务状态为处理中
	task.SetStatus(string(TaskStatus_Processing))
	r.addToTimeline("processing", fmt.Sprintf("Started processing task: %s", task.GetUserInput()), task.GetId())

	// 从任务中提取用户输入
	userInput := task.GetUserInput()

	// 重置会话状态
	r.config.mu.Lock()
	r.config.finished = false
	r.config.currentIteration = 0
	// 为新任务重置内存
	r.config.memory = aid.GetDefaultMemory()
	// 重新初始化内存
	if r.config.memory != nil && r.config.aiCallback != nil {
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

// AddTaskHook 为任务队列添加Hook
func (r *ReAct) AddTaskHook(hook TaskHook) {
	r.taskQueue.AddHook(hook)
}

// ClearTaskHooks 清除任务队列中的所有Hook
func (r *ReAct) ClearTaskHooks() {
	r.taskQueue.ClearHooks()
}

// processInputEvent processes a single input event and triggers ReAct loop
func (r *ReAct) processInputEvent(event *ypb.AIInputEvent) error {
	if r.config.debugEvent {
		log.Infof("Processing input event: IsFreeInput=%v, IsInteractive=%v", event.IsFreeInput, event.IsInteractiveMessage)
	}

	// Handle different types of input events
	var userInput string
	var shouldResetSession bool

	if event.IsFreeInput {
		userInput = event.FreeInput
		shouldResetSession = true // Reset session for new free input
		if r.config.debugEvent {
			log.Infof("Using free input: %s", userInput)
		}
	} else if event.IsInteractiveMessage {
		// Handle interactive messages (tool review responses)
		if r.config.debugEvent {
			log.Infof("Processing interactive message: ID=%s", event.InteractiveId)
		}

		// Parse the interactive JSON input to get the suggestion
		suggestion := gjson.Get(event.InteractiveJSONInput, "suggestion").String()
		if suggestion == "" {
			suggestion = "continue" // Default fallback
		}

		// Feed the response to the endpoint manager
		if r.config.epm != nil {
			params := aitool.InvokeParams{
				"suggestion": suggestion,
			}
			r.config.epm.Feed(event.InteractiveId, params)
			if r.config.debugEvent {
				log.Infof("Fed interactive response to endpoint: ID=%s, suggestion=%s", event.InteractiveId, suggestion)
			}
		}

		return nil
	} else {
		log.Warnf("No valid input found in event")
		return nil
	}

	// Reset session state if needed
	if shouldResetSession {
		r.config.mu.Lock()
		r.config.finished = false
		r.config.currentIteration = 0
		// Reset memory for new session
		r.config.memory = aid.GetDefaultMemory()
		// Re-initialize memory with tools and AI capability
		if r.config.memory != nil && r.config.aiCallback != nil {
			// Reset memory state for new session
			// Set the AI instance for memory timeline
			r.config.memory.SetTimelineAI(r.config)

			// Store tools function
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
		if r.config.debugEvent {
			log.Infof("Reset ReAct session for new input")
		}
	}

	// Execute the main ReAct loop using the new schema-based approach
	if r.config.debugEvent {
		log.Infof("Executing main loop with user input: %s", userInput)
	}
	return r.executeMainLoop(userInput)
}

// startEventLoop starts the background event processing loop
func (r *ReAct) startEventLoop(ctx context.Context) {
	r.config.startInputEventOnce.Do(func() {
		go func() {
			if r.config.debugEvent {
				log.Infof("ReAct event loop started for instance: %s", r.config.id)
			}

			for {
				if r.config.eventInputChan == nil {
					if r.config.debugEvent {
						log.Warnf("ReAct event input channel is nil, will retry...")
					}
					<-ctx.Done()
					return
				}

				select {
				case event, ok := <-r.config.eventInputChan:
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
