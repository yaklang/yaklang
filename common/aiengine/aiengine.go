package aiengine

import (
	"context"
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
	config      *AIEngineConfig
	react       *aireact.ReAct
	outputChan  chan *schema.AiOutputEvent
	ctx         context.Context
	cancel      context.CancelFunc
	finished    chan struct{}
	lastResult  map[string]any
	lastSuccess bool
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

	engine := &AIEngine{
		config:     config,
		react:      react,
		outputChan: outputChan,
		ctx:        ctx,
		cancel:     cancel,
		finished:   make(chan struct{}),
		lastResult: make(map[string]any),
	}

	// 启动输出处理器
	go engine.handleOutputEvents()

	// 发送初始化配置
	err = engine.sendInitConfig()
	if err != nil {
		return nil, utils.Errorf("send init config failed: %v", err)
	}
	return engine, nil
}

// sendInitConfig 发送初始化配置
func (e *AIEngine) sendInitConfig() error {
	event := &ypb.AIInputEvent{
		IsStart: true,
		Params:  e.config.ConvertToYPBAIStartParams(),
	}
	return e.react.SendInputEvent(event)
}

// SendMsg 执行 AI 任务（阻塞直到完成）
func (e *AIEngine) SendMsg(input string) error {
	if input == "" {
		return utils.Error("input cannot be empty")
	}

	// 发送输入事件
	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   input,
	}

	if err := e.react.SendInputEvent(event); err != nil {
		return utils.Errorf("failed to send input event: %v", err)
	}

	// 等待任务完成
	e.react.Wait()

	// 调用完成回调
	if e.config.OnFinished != nil {
		e.config.OnFinished(e.react, e.lastSuccess, e.lastResult)
	}

	return nil
}

// SendMsgAsync 异步执行 AI 任务（立即返回）
func (e *AIEngine) SendMsgAsync(input string) error {
	if input == "" {
		return utils.Error("input cannot be empty")
	}

	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   input,
	}

	return e.react.SendInputEvent(event)
}

// InvokeReActAsync 异步执行 ReAct 任务，并返回引擎实例
func InvokeReActAsync(input string, options ...AIEngineConfigOption) (*AIEngine, error) {
	engine, err := NewAIEngine(options...)
	if err != nil {
		return nil, err
	}
	defer engine.Close()

	if err := engine.SendMsgAsync(input); err != nil {
		return nil, err
	}

	return engine, nil
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

// GetLastResult 获取最后一次执行的结果
func (e *AIEngine) GetLastResult() (success bool, result map[string]any) {
	return e.lastSuccess, e.lastResult
}

// Close 关闭 AI 引擎，释放资源
func (e *AIEngine) Close() {
	e.cancel()
	close(e.outputChan)
	<-e.finished
}

// handleOutputEvents 处理输出事件
func (e *AIEngine) handleOutputEvents() {
	defer close(e.finished)

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
				e.config.OnEvent(event)
			}

		case <-e.ctx.Done():
			return
		}
	}
}

// processOutputEvent 处理单个输出事件
func (e *AIEngine) processOutputEvent(event *schema.AiOutputEvent) {
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
		// 流式文本输出
		if e.config.OnStream != nil && len(event.Content) > 0 {
			e.config.OnStream(e.react, event.Content)
		}

	case schema.EVENT_TYPE_OBSERVATION:
		// 任务完成（通过观察结果判断）
		if !event.IsStream && len(event.Content) > 0 {
			e.lastSuccess = true
			e.lastResult["content"] = string(event.Content)
		}

	default:
		// 记录其他事件类型
		if event.Type == "error" {
			e.lastSuccess = false
			e.lastResult["error"] = string(event.Content)
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
			case <-ctx.Done():
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
	if config.AIService != "" {
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

// InvokeReActWithResult 快速执行 ReAct 任务并返回结果
func InvokeReActWithResult(input string, options ...AIEngineConfigOption) (success bool, result map[string]any, err error) {
	engine, err := NewAIEngine(options...)
	if err != nil {
		return false, nil, err
	}
	defer engine.Close()

	if err := engine.SendMsg(input); err != nil {
		return false, nil, err
	}

	success, result = engine.GetLastResult()
	return success, result, nil
}

// InvokeReActWithTimeout 执行 ReAct 任务，带超时控制
func InvokeReActWithTimeout(input string, timeout time.Duration, options ...AIEngineConfigOption) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 添加超时上下文
	options = append([]AIEngineConfigOption{WithContext(ctx)}, options...)

	return InvokeReAct(input, options...)
}
