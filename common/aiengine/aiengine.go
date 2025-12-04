package aiengine

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIEngine AI 引擎封装
// 提供简化的 API 来使用 ReAct 和其他 AI 功能
type AIEngine struct {
	config     *AIEngineConfig
	react      *aireact.ReAct
	outputChan chan *schema.AiOutputEvent
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup // 等待 goroutine 完成

	// 任务跟踪
	tasksMutex         sync.RWMutex
	activeTasks        map[string]aicommon.AITaskState // 任务ID -> 任务状态
	allTasksEndpoint   *aicommon.Endpoint              // 所有任务完成信号
	taskCreatedPending *aicommon.Endpoint              // 等待任务创建的 endpoint
	sendMsgMutex       sync.Mutex                      // sendMsgAndGetTaskName 同步锁
	taskEndpoints      map[string]*aicommon.Endpoint   // 任务ID -> 任务完成 endpoint
}

// NewAIEngine 创建新的 AI 引擎实例
func NewAIEngine(options ...AIEngineConfigOption) (*AIEngine, error) {
	config := NewAIEngineConfig(options...)

	// 创建上下文
	ctx, cancel := context.WithCancel(config.Context)

	// 创建通道
	outputChan := make(chan *schema.AiOutputEvent, 100)

	// 构建 ReAct 配置选项
	reactOptions := buildReActOptions(ctx, config, outputChan)

	// 创建 ReAct 实例
	react, err := aireact.NewReAct(reactOptions...)
	if err != nil {
		cancel()
		return nil, utils.Errorf("failed to create ReAct instance: %v", err)
	}

	// 创建 endpoint manager 用于任务同步
	epm := aicommon.NewEndpointManagerContext(ctx)

	engine := &AIEngine{
		config:           config,
		react:            react,
		outputChan:       outputChan,
		ctx:              ctx,
		cancel:           cancel,
		activeTasks:      make(map[string]aicommon.AITaskState),
		allTasksEndpoint: epm.CreateEndpoint(), // 所有任务完成信号
		taskEndpoints:    make(map[string]*aicommon.Endpoint),
	}

	// 启动输出处理器
	engine.wg.Add(1)
	go engine.handleOutputEvents()

	// 发送初始化配置
	err = engine.sendInitConfig()
	if err != nil {
		return nil, utils.Errorf("send init config failed: %v", err)
	}
	return engine, nil
}

func (e *AIEngine) GetReAct() *aireact.ReAct {
	return e.react
}

// sendInitConfig 发送初始化配置
func (e *AIEngine) sendInitConfig() error {
	event := &ypb.AIInputEvent{
		IsStart: true,
		Params:  e.config.ConvertToYPBAIStartParams(),
	}
	return e.react.SendInputEvent(event)
}

// sendMsgAndGetTaskName 发送消息并等待获取任务名称
// 该函数使用互斥锁确保同一时间只有一个调用在执行
func (e *AIEngine) sendMsgAndGetTaskName(input string) (string, error) {
	// 加锁，确保同一时间只有一个 sendMsgAndGetTaskName 在运行
	e.sendMsgMutex.Lock()

	if input == "" {
		e.sendMsgMutex.Unlock()
		return "", utils.Error("input cannot be empty")
	}

	// 创建 endpoint manager 并创建等待任务创建的 endpoint
	epm := aicommon.NewEndpointManagerContext(e.ctx)
	taskCreatedEndpoint := epm.CreateEndpoint()
	e.taskCreatedPending = taskCreatedEndpoint

	e.sendMsgMutex.Unlock()

	// 发送输入事件
	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   input,
	}

	if err := e.react.SendInputEvent(event); err != nil {
		return "", utils.Errorf("failed to send input event: %v", err)
	}

	// 等待任务创建，获取任务ID（带超时）
	if !taskCreatedEndpoint.WaitTimeout(30 * time.Second) {
		return "", utils.Error("timeout waiting for task creation")
	}

	// 从 endpoint 参数中获取任务ID
	params := taskCreatedEndpoint.GetParams()
	if taskID, ok := params["task_id"].(string); ok {
		return taskID, nil
	}

	return "", utils.Error("failed to get task ID from endpoint")
}

// SendMsg 执行 AI 任务（阻塞直到该任务完成）
func (e *AIEngine) SendMsg(input string) error {
	// 发送消息并获取任务名称
	taskID, err := e.sendMsgAndGetTaskName(input)
	if err != nil {
		return err
	}

	// 等待该任务完成
	if err := e.WaitTaskFinishByTaskName(taskID); err != nil {
		return err
	}

	// 调用完成回调
	if e.config.OnFinished != nil {
		e.config.OnFinished(e.react)
	}

	return nil
}

// WaitTaskFinishByTaskName 等待指定任务完成
// 传入任务ID，等待该任务状态变为 Completed 或 Aborted
func (e *AIEngine) WaitTaskFinishByTaskName(taskID string) error {
	if taskID == "" {
		return utils.Error("taskID cannot be empty")
	}

	// 检查任务是否已经完成
	e.tasksMutex.RLock()
	status, exists := e.activeTasks[taskID]
	if exists && (status == aicommon.AITaskState_Completed || status == aicommon.AITaskState_Aborted) {
		e.tasksMutex.RUnlock()
		return nil
	}
	e.tasksMutex.RUnlock()

	// 创建该任务的 endpoint
	e.tasksMutex.Lock()
	taskEndpoint, exists := e.taskEndpoints[taskID]
	if !exists {
		epm := aicommon.NewEndpointManagerContext(e.ctx)
		taskEndpoint = epm.CreateEndpoint()
		e.taskEndpoints[taskID] = taskEndpoint
	}
	e.tasksMutex.Unlock()

	// 等待任务完成
	taskEndpoint.WaitContext(e.ctx)
	return e.ctx.Err()
}

// WaitTaskFinish 等待所有任务完成
// 通过监听任务状态变化来判断所有任务是否完成
func (e *AIEngine) WaitTaskFinish() error {
	// 检查是否还有活跃任务
	if !e.hasActiveTasks() {
		return nil
	}

	// 等待所有任务完成
	e.allTasksEndpoint.WaitContext(e.ctx)
	return e.ctx.Err()
}

// hasActiveTasks 检查是否还有活跃的任务（未完成或未中止的任务）
func (e *AIEngine) hasActiveTasks() bool {
	e.tasksMutex.RLock()
	defer e.tasksMutex.RUnlock()

	for _, status := range e.activeTasks {
		if status != aicommon.AITaskState_Completed && status != aicommon.AITaskState_Aborted {
			return true
		}
	}
	return false
}

// GetActiveTaskCount 获取当前活跃任务数量
func (e *AIEngine) GetActiveTaskCount() int {
	e.tasksMutex.RLock()
	defer e.tasksMutex.RUnlock()

	count := 0
	for _, status := range e.activeTasks {
		if status != aicommon.AITaskState_Completed && status != aicommon.AITaskState_Aborted {
			count++
		}
	}
	return count
}

// SendMsgAsync 异步执行 AI 任务（立即返回）
func (e *AIEngine) SendMsgAsync(input string) error {
	_, err := e.sendMsgAndGetTaskName(input)
	if err != nil {
		return err
	}
	return nil
}

// SendInteractiveResponse 发送交互式响应
// 用于回复 AI 提出的问题或需要用户确认的操作
func (e *AIEngine) SendInteractiveResponse(response string) error {
	event := &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveJSONInput: response,
	}

	return e.react.SendInputEvent(event)
}

// Wait 等待所有任务完成
func (e *AIEngine) Wait() {
	e.react.Wait()
}

// IsFinished 检查任务是否完成
func (e *AIEngine) IsFinished() bool {
	return e.react.IsFinished()
}

// Close 关闭 AI 引擎，释放资源
func (e *AIEngine) Close() {
	// 先取消上下文，通知所有 goroutine 停止
	e.cancel()

	// 等待所有 goroutine 完成
	e.wg.Wait()

	// close(e.outputChan) // 需要等待 React 退出后再close
}

// handleOutputEvents 处理输出事件
func (e *AIEngine) handleOutputEvents() {
	defer e.wg.Done() // 确保在退出时调用 Done

	for {
		select {
		case event, ok := <-e.outputChan:
			if !ok {
				return
			}
			if event == nil {
				continue
			}

			// 处理不同类型的事件
			e.processOutputEvent(event)

			// 调用用户的事件回调
			if e.config.OnEvent != nil {
				e.config.OnEvent(e.react, event)
			}

		case <-e.ctx.Done():
			return
		}
	}
}

// processOutputEvent 处理单个输出事件
func (e *AIEngine) processOutputEvent(event *schema.AiOutputEvent) {
	if event.Type == schema.EVENT_TYPE_STRUCTURED {
		if event.NodeId == "react_task_created" {
			taskInfo := map[string]string{}
			err := json.Unmarshal(event.Content, &taskInfo)
			if err != nil {
				log.Errorf("failed to unmarshal task info: %v", err)
				return
			}
			// {"react_task_id":"re-act-task-355VUaVpMxVpglfOyMuuacTWGcV","react_task_status":"created","react_user_input":"你好"}
			taskID := taskInfo["react_task_id"]
			taskStatus := aicommon.AITaskState(taskInfo["react_task_status"])

			// 记录新任务
			e.tasksMutex.Lock()
			e.activeTasks[taskID] = taskStatus

			// 通知任务已创建（如果有等待的 endpoint）
			if e.taskCreatedPending != nil {
				params := make(map[string]interface{})
				params["task_id"] = taskID
				e.taskCreatedPending.ActiveWithParams(e.ctx, params)
				e.taskCreatedPending = nil
			}
			e.tasksMutex.Unlock()
		}
		// {"react_task_id":"re-act-task-355VUaVpMxVpglfOyMuuacTWGcV","react_task_now_status":"completed","react_task_old_status":"processing"}
		if event.NodeId == "react_task_status_changed" {
			taskInfo := map[string]string{}
			err := json.Unmarshal(event.Content, &taskInfo)
			if err != nil {
				log.Errorf("failed to unmarshal task info: %v", err)
				return
			}

			// 更新任务状态
			taskID := taskInfo["react_task_id"]
			nowStatus := aicommon.AITaskState(taskInfo["react_task_now_status"])

			e.tasksMutex.Lock()
			e.activeTasks[taskID] = nowStatus
			e.tasksMutex.Unlock()

			// 检查任务是否完成或中止
			if nowStatus == aicommon.AITaskState_Completed || nowStatus == aicommon.AITaskState_Aborted {
				// 通知该任务的等待者
				e.tasksMutex.Lock()
				if taskEndpoint, exists := e.taskEndpoints[taskID]; exists {
					taskEndpoint.Release()
					// 清理 endpoint
					delete(e.taskEndpoints, taskID)
				}
				e.tasksMutex.Unlock()

				// 检查是否所有任务都已完成
				if !e.hasActiveTasks() {
					// 通知所有任务完成
					e.allTasksEndpoint.Release()
				}
			}
		}
	}

	if event.IsInteractive() {
		if e.config.OnInputRequired != nil {
			response := e.config.OnInputRequired(e.react, string(event.Content))
			if response != "" {
				_ = e.SendInteractiveResponse(response)
			}
		}
		if e.config.OnInputRequiredRaw != nil {
			response := e.config.OnInputRequiredRaw(e.react, event, string(event.Content))
			if response != "" {
				_ = e.SendInteractiveResponse(response)
			}
		}
		return
	}
	switch event.Type {
	case schema.EVENT_TYPE_STREAM:
		e.config.OnStream(e.react, event, event.NodeId, event.StreamDelta)
	default:
		// 记录其他事件类型
		if event.Type == "error" {
			log.Errorf("AI Engine error: %s", string(event.Content))
		}
	}
}

// buildReActOptions 构建 ReAct 配置选项
func buildReActOptions(ctx context.Context, config *AIEngineConfig, outputChan chan *schema.AiOutputEvent) []aicommon.ConfigOption {
	options := []aicommon.ConfigOption{
		// 基础配置
		aicommon.WithContext(ctx),
		aireact.WithBuiltinTools(),

		// AI 服务配置
		aicommon.WithAICallback(aicommon.AIChatToAICallbackType(ai.Chat)),

		// 知识库配置
		aicommon.WithEnhanceKnowledgeManager(rag.NewRagEnhanceKnowledgeManager()),

		// 会话配置
		aicommon.WithPersistentSessionId(config.SessionID),
		aicommon.WithEnableSelfReflection(true),
		aicommon.WithEnablePETaskAnalyze(true),

		// 事件处理
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			select {
			case outputChan <- e:
				// 成功发送
			case <-ctx.Done():
				// 上下文已取消，停止发送
			default:
				// 如果通道满了，记录警告
				log.Warnf("Output channel full, dropping event: %s", e.Type)
			}
		}),
	}

	// 迭代次数限制
	if config.MaxIteration > 0 {
		options = append(options, aicommon.WithMaxIterationCount(int64(config.MaxIteration)))
	}

	// 工具配置
	if config.DisableToolUse {
		options = append(options, aicommon.WithDisableToolUse(true))
	}

	if config.DisableAIForge {
		options = append(options, aicommon.WithEnablePlanAndExec(false))
	} else {
		options = append(options, aicommon.WithEnablePlanAndExec(true))
	}

	if config.DisableMCPServers {
		options = append(options, aicommon.WithDisallowMCPServers(true))
	}

	if config.EnableAISearchTool {
		options = append(options, aid.WithAiToolsSearchTool())
	}

	if config.EnableForgeSearchTool {
		options = append(options, aid.WithAiForgeSearchTool())
	}

	if len(config.ExcludeToolNames) > 0 {
		options = append(options, aicommon.WithDisableToolsName(config.ExcludeToolNames...))
	}

	if len(config.IncludeToolNames) > 0 {
		options = append(options, aicommon.WithEnableToolsName(config.IncludeToolNames...))
	}

	if len(config.Keywords) > 0 {
		options = append(options, aicommon.WithKeywords(config.Keywords...))
	}

	// 交互配置
	if !config.AllowUserInteract {
		options = append(options, aicommon.WithAllowRequireForUserInteract(false))
	}

	if config.ReviewPolicy != "" {
		options = append(options, aicommon.WithAgreePolicy(aicommon.AgreePolicyType(config.ReviewPolicy)))
	}

	if config.UserInteractLimit > 0 {
		options = append(options, aicommon.WithPlanUserInteractMaxCount(config.UserInteractLimit))
	}

	// 内容限制
	if config.TimelineContentLimit > 0 {
		options = append(options, aicommon.WithTimelineContentLimit(config.TimelineContentLimit))
	}

	// AI 服务
	if config.AICallback != nil {
		options = append(options, aicommon.WithAICallback(config.AICallback))
	} else if config.AIService != "" {
		chat, err := ai.LoadChater(config.AIService)
		if err != nil {
			log.Errorf("load ai service failed: %v", err)
		} else {
			options = append(options, aicommon.WithAICallback(aicommon.AIChatToAICallbackType(chat)))
		}
	}

	// 高级配置

	if config.Focus != "" {
		options = append(options, aicommon.WithFocus(config.Focus))
	}

	if config.Workdir != "" {
		options = append(options, aicommon.WithWorkdir(config.Workdir))
	}

	if config.Language != "" {
		options = append(options, aicommon.WithLanguage(config.Language))
	}

	// 调试模式
	if config.DebugMode {
		options = append(options,
			aicommon.WithDebugPrompt(true),
			aicommon.WithDebugEvent(true),
		)
	}

	options = append(options, config.ExtOptions...)

	return options
}

func InvokeReAct(input string, options ...AIEngineConfigOption) error {
	engine, err := NewAIEngine(options...)
	if err != nil {
		return err
	}
	defer engine.Close()

	return engine.SendMsg(input)
}

// InvokeReActAsync 异步执行 ReAct 任务，并返回引擎实例
func InvokeReActAsync(input string, options ...AIEngineConfigOption) (*AIEngine, error) {
	engine, err := NewAIEngine(options...)
	if err != nil {
		return nil, err
	}

	if err := engine.SendMsgAsync(input); err != nil {
		return nil, err
	}

	return engine, nil
}
