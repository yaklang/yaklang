package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// 注意：AICallbackType 已经在 aicaller.go 中定义

// BaseAICallerConfig 基础的 AICallerConfigIf 实现
type BaseAICallerConfig struct {
	// 基础配置
	ctx       context.Context
	cancel    context.CancelFunc
	mutex     *sync.RWMutex
	runtimeId string
	db        *gorm.DB

	// WaitGroup for sub tasks
	wg sync.WaitGroup

	// ID 生成器
	idSequence  int64
	idGenerator func() int64

	// 消费量统计
	inputConsumption  *int64
	outputConsumption *int64

	// AI 重试配置
	aiAutoRetry            int64
	aiTransactionAutoRetry int64
	aiCallTokenLimit       int64

	// 事件发射器
	emitter *Emitter

	// 端点管理器
	endpointManager *EndpointManager

	// 用户交互通道
	userInteractionChan *chanx.UnlimitedChan[UserInteractionEvent]

	// 检查点存储回调
	checkpointStorage CheckpointStorage

	// 工具管理
	aiToolManager       *buildinaitools.AiToolManager
	aiToolManagerOption []buildinaitools.ToolManagerOption

	// 同步功能
	syncMutex *sync.RWMutex
	syncMap   map[string]func() any

	// 扩展动作回调
	extendedActionCallback map[string]func(config *BaseAICallerConfig, action *Action)

	// AI回调
	originalAICallback    AICallbackType
	coordinatorAICallback AICallbackType
	planAICallback        AICallbackType
	taskAICallback        AICallbackType

	// 调试和事件配置
	debugPrompt bool
	debugEvent  bool
	saveEvent   bool

	// 事件处理相关
	eventHandler        func(e *schema.AiOutputEvent)
	eventProcessHandler *utils.Stack[func(e *schema.AiOutputEvent) *schema.AiOutputEvent]

	// 提示钩子
	promptHook func(string) string

	// 输出事件类型禁用列表
	disableOutputEventType []string
}

// UserInteractionEvent 用户交互事件
type UserInteractionEvent struct {
	EventID      string              `json:"event_id"`
	InvokeParams aitool.InvokeParams `json:"invoke_params"`
}

// CheckpointStorage 检查点存储接口
type CheckpointStorage interface {
	CreateReviewCheckpoint(runtimeId string, id int64) *schema.AiCheckpoint
	CreateToolCallCheckpoint(runtimeId string, id int64) *schema.AiCheckpoint
	SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, req any) error
	SubmitCheckpointResponse(checkpoint *schema.AiCheckpoint, rsp any) error
}

// DefaultCheckpointStorage 默认检查点存储实现
type DefaultCheckpointStorage struct {
	db *gorm.DB
}

func (d *DefaultCheckpointStorage) createCheckpoint(runtimeId string, typeName schema.AiCheckpointType, id int64) *schema.AiCheckpoint {
	checkpoint := &schema.AiCheckpoint{
		CoordinatorUuid: runtimeId,
		Seq:             id,
		Type:            typeName,
	}
	yakit.CreateOrUpdateCheckpoint(d.db, checkpoint)
	return checkpoint
}

func (d *DefaultCheckpointStorage) CreateReviewCheckpoint(runtimeId string, id int64) *schema.AiCheckpoint {
	return d.createCheckpoint(runtimeId, schema.AiCheckpointType_Review, id)
}

func (d *DefaultCheckpointStorage) CreateToolCallCheckpoint(runtimeId string, id int64) *schema.AiCheckpoint {
	return d.createCheckpoint(runtimeId, schema.AiCheckpointType_ToolCall, id)
}

func (d *DefaultCheckpointStorage) SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, req any) error {
	checkpoint.RequestQuotedJson = codec.StrConvQuote(string(utils.Jsonify(req)))
	return yakit.CreateOrUpdateCheckpoint(d.db, checkpoint)
}

func (d *DefaultCheckpointStorage) SubmitCheckpointResponse(checkpoint *schema.AiCheckpoint, rsp any) error {
	checkpoint.ResponseQuotedJson = codec.StrConvQuote(string(utils.Jsonify(rsp)))
	checkpoint.Finished = true
	return yakit.CreateOrUpdateCheckpoint(d.db, checkpoint)
}

// NewBaseAICallerConfig 创建新的基础配置
func NewBaseAICallerConfig(ctx context.Context, runtimeId string, db *gorm.DB) *BaseAICallerConfig {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)

	idGenerator := new(int64)
	config := &BaseAICallerConfig{
		ctx:        ctx,
		cancel:     cancel,
		mutex:      &sync.RWMutex{},
		runtimeId:  runtimeId,
		db:         db,
		idSequence: atomic.AddInt64(idGenerator, 1000), // 从1000开始
		idGenerator: func() int64 {
			return atomic.AddInt64(idGenerator, 1)
		},
		inputConsumption:       new(int64),
		outputConsumption:      new(int64),
		aiAutoRetry:            5,
		aiTransactionAutoRetry: 5, // 默认重试5次
		aiCallTokenLimit:       int64(1000 * 30),
		userInteractionChan:    chanx.NewUnlimitedChan[UserInteractionEvent](ctx, 100),
		aiToolManagerOption:    make([]buildinaitools.ToolManagerOption, 0),
		syncMutex:              &sync.RWMutex{},
		syncMap:                make(map[string]func() any),
		extendedActionCallback: make(map[string]func(config *BaseAICallerConfig, action *Action)),
		debugPrompt:            false,
		debugEvent:             false,
		saveEvent:              false,
		disableOutputEventType: make([]string, 0),
		eventProcessHandler:    utils.NewStack[func(e *schema.AiOutputEvent) *schema.AiOutputEvent](),
		checkpointStorage: &DefaultCheckpointStorage{
			db: db,
		},
	}

	// 创建事件发射器
	config.emitter = NewEmitter(runtimeId, func(e *schema.AiOutputEvent) error {
		// 默认的事件处理，可以在这里添加日志记录等
		log.Debugf("emit event: type=%s, nodeId=%s", e.Type, e.NodeId)
		return nil
	})

	// 创建端点管理器
	config.endpointManager = NewEndpointManagerContext(ctx)
	config.endpointManager.SetConfig(config)

	return config
}

// 实现 AICallerConfigIf 接口

func (g *BaseAICallerConfig) AcquireId() int64 {
	return g.idGenerator()
}

func (g *BaseAICallerConfig) GetDB() *gorm.DB {
	return g.db
}

func (g *BaseAICallerConfig) GetRuntimeId() string {
	return g.runtimeId
}

func (g *BaseAICallerConfig) IsCtxDone() bool {
	select {
	case <-g.ctx.Done():
		return true
	default:
		return false
	}
}

func (g *BaseAICallerConfig) GetContext() context.Context {
	return g.ctx
}

func (g *BaseAICallerConfig) CallAIResponseConsumptionCallback(current int) {
	atomic.AddInt64(g.outputConsumption, int64(current))
}

func (g *BaseAICallerConfig) GetAITransactionAutoRetryCount() int64 {
	return g.aiTransactionAutoRetry
}

// RetryPromptBuilder 默认的重试提示构建器，使用类似 AITransactionWithError 的逻辑
func (g *BaseAICallerConfig) RetryPromptBuilder(rawPrompt string, retryErr error) string {
	if retryErr == nil {
		return rawPrompt
	}

	retryTemplate := `
{{ .RawPrompt }}

# Error Handling:
Note that your previous response encountered an error. Here's the failure reason:
{{ .RetryReason }}
Please avoid making the same mistake when generating your response.
# How to fix?
If you need to generate action/@action JSON, refer to the following format and ensure compliance: {"@action": "...", ... }
`

	templateData := map[string]interface{}{
		"RetryReason": retryErr.Error(),
		"RawPrompt":   rawPrompt,
	}

	// 简单的模板替换
	result := retryTemplate
	for key, value := range templateData {
		placeholder := fmt.Sprintf("{{ .%s }}", key)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}

	return result
}

func (g *BaseAICallerConfig) GetEmitter() *Emitter {
	return g.emitter
}

func (g *BaseAICallerConfig) NewAIResponse() *AIResponse {
	return NewAIResponse(g)
}

func (g *BaseAICallerConfig) CallAIResponseOutputFinishedCallback(output string) {
	// 可以在这里处理输出完成后的回调，例如解析扩展动作
	log.Debugf("AI response output finished, length: %d", len(output))
}

func (g *BaseAICallerConfig) CreateReviewCheckpoint(id int64) *schema.AiCheckpoint {
	return g.checkpointStorage.CreateReviewCheckpoint(g.runtimeId, id)
}

func (g *BaseAICallerConfig) CreateToolCallCheckpoint(id int64) *schema.AiCheckpoint {
	return g.checkpointStorage.CreateToolCallCheckpoint(g.runtimeId, id)
}

func (g *BaseAICallerConfig) GetEndpointManager() *EndpointManager {
	return g.endpointManager
}

func (g *BaseAICallerConfig) SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, req any) error {
	return g.checkpointStorage.SubmitCheckpointRequest(checkpoint, req)
}

func (g *BaseAICallerConfig) SubmitCheckpointResponse(checkpoint *schema.AiCheckpoint, rsp any) error {
	return g.checkpointStorage.SubmitCheckpointResponse(checkpoint, rsp)
}

// DoWaitAgree 通过 UnlimitedChannel 处理外部输入
func (g *BaseAICallerConfig) DoWaitAgree(ctx context.Context, endpoint *Endpoint) {
	if ctx == nil {
		ctx = g.ctx
	}

	// 如果检查点已完成，直接返回
	if checkpoint := endpoint.GetCheckpoint(); checkpoint != nil && checkpoint.Finished {
		return
	}

	defer func() {
		// 完成时提交检查点响应
		if checkpoint := endpoint.GetCheckpoint(); checkpoint != nil {
			if err := g.SubmitCheckpointResponse(checkpoint, endpoint.GetParams()); err != nil {
				log.Errorf("submit checkpoint response error: %v", err)
			}
		}
	}()

	// 等待用户交互输入或上下文取消
	select {
	case <-ctx.Done():
		log.Infof("DoWaitAgree context cancelled")
		return
	case userEvent, ok := <-g.userInteractionChan.OutputChannel():
		if ok && userEvent.EventID == endpoint.GetId() {
			// 设置用户输入的参数
			endpoint.SetParams(userEvent.InvokeParams)
			endpoint.Release()
			log.Infof("received user interaction for endpoint %s", endpoint.GetId())
		}
	case <-time.After(30 * time.Second): // 默认30秒超时
		log.Infof("DoWaitAgree timeout, using default params")
		endpoint.Release()
	}
}

func (g *BaseAICallerConfig) ReleaseInteractiveEvent(eventID string, invokeParams aitool.InvokeParams) {
	// 发出交互释放事件
	g.emitter.EmitInteractiveRelease(eventID, invokeParams)

	// 通过通道发送用户交互事件
	userEvent := UserInteractionEvent{
		EventID:      eventID,
		InvokeParams: invokeParams,
	}
	g.userInteractionChan.SafeFeed(userEvent)
}

// 辅助方法

// SetCheckpointStorage 设置自定义的检查点存储实现
func (g *BaseAICallerConfig) SetCheckpointStorage(storage CheckpointStorage) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.checkpointStorage = storage
}

// SetAITransactionAutoRetryCount 设置AI事务自动重试次数
func (g *BaseAICallerConfig) SetAITransactionAutoRetryCount(count int64) {
	atomic.StoreInt64(&g.aiTransactionAutoRetry, count)
}

// GetInputConsumption 获取输入消费量
func (g *BaseAICallerConfig) GetInputConsumption() int64 {
	return atomic.LoadInt64(g.inputConsumption)
}

// GetOutputConsumption 获取输出消费量
func (g *BaseAICallerConfig) GetOutputConsumption() int64 {
	return atomic.LoadInt64(g.outputConsumption)
}

// Close 关闭配置，清理资源
func (g *BaseAICallerConfig) Close() {
	if g.cancel != nil {
		g.cancel()
	}
	if g.userInteractionChan != nil {
		g.userInteractionChan.Close()
	}
}

// GetUserInteractionChannel 获取用户交互通道，用于外部发送用户交互事件
func (g *BaseAICallerConfig) GetUserInteractionChannel() *chanx.UnlimitedChan[UserInteractionEvent] {
	return g.userInteractionChan
}

// SetEmitterHandler 设置自定义的事件发射处理器
func (g *BaseAICallerConfig) SetEmitterHandler(handler BaseEmitter) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.emitter = NewEmitter(g.runtimeId, handler)
}

// === 扩展方法，覆盖 aid.Config 的功能 ===

// Add 增加等待任务数量
func (g *BaseAICallerConfig) Add(delta int) {
	g.wg.Add(delta)
}

// Done 标记任务完成
func (g *BaseAICallerConfig) Done() {
	g.wg.Done()
}

// Wait 等待所有任务完成
func (g *BaseAICallerConfig) Wait() {
	g.wg.Wait()
	log.Info("BaseAICallerConfig 's wg is waiting done, all tasks finished, start to check stream...")
	g.emitter.WaitForStream()
	log.Info("BaseAICallerConfig 's all stream waitgroup is done, all tasks finished")
}

// CallAI 调用AI，支持多种回调类型
func (g *BaseAICallerConfig) CallAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		g.taskAICallback,
		g.coordinatorAICallback,
		g.planAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(g, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

// GetSequenceStart 获取序列起始值
func (g *BaseAICallerConfig) GetSequenceStart() int64 {
	return g.idSequence
}

// MakeInvokeParams 创建调用参数
func (g *BaseAICallerConfig) MakeInvokeParams() aitool.InvokeParams {
	p := make(aitool.InvokeParams)
	p["runtime_id"] = g.runtimeId
	return p
}

// ProcessExtendedActionCallback 处理扩展动作回调
func (g *BaseAICallerConfig) ProcessExtendedActionCallback(resp string) {
	actions := ExtractAllAction(resp)
	for _, action := range actions {
		if cb, ok := g.extendedActionCallback[action.Name()]; ok {
			cb(g, action)
		}
	}
}

// SetSyncCallback 设置同步回调
func (g *BaseAICallerConfig) SetSyncCallback(key string, callback func() any) {
	g.syncMutex.Lock()
	defer g.syncMutex.Unlock()
	g.syncMap[key] = callback
}

// GetSyncCallback 获取同步回调结果
func (g *BaseAICallerConfig) GetSyncCallback(key string) any {
	g.syncMutex.RLock()
	defer g.syncMutex.RUnlock()
	if cb, ok := g.syncMap[key]; ok {
		return cb()
	}
	return nil
}

// SetExtendedActionCallback 设置扩展动作回调
func (g *BaseAICallerConfig) SetExtendedActionCallback(name string, cb func(config *BaseAICallerConfig, action *Action)) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.extendedActionCallback == nil {
		g.extendedActionCallback = make(map[string]func(config *BaseAICallerConfig, action *Action))
	}
	g.extendedActionCallback[name] = cb
}

// SetOriginalAICallback 设置原始AI回调
func (g *BaseAICallerConfig) SetOriginalAICallback(cb AICallbackType) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.originalAICallback = cb
}

// SetCoordinatorAICallback 设置协调器AI回调
func (g *BaseAICallerConfig) SetCoordinatorAICallback(cb AICallbackType) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.coordinatorAICallback = cb
}

// SetPlanAICallback 设置计划AI回调
func (g *BaseAICallerConfig) SetPlanAICallback(cb AICallbackType) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.planAICallback = cb
}

// SetTaskAICallback 设置任务AI回调
func (g *BaseAICallerConfig) SetTaskAICallback(cb AICallbackType) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.taskAICallback = cb
}

// GetOriginalAICallback 获取原始AI回调
func (g *BaseAICallerConfig) GetOriginalAICallback() AICallbackType {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.originalAICallback
}

// SetPromptHook 设置提示钩子
func (g *BaseAICallerConfig) SetPromptHook(hook func(string) string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.promptHook = hook
}

// GetPromptHook 获取提示钩子
func (g *BaseAICallerConfig) GetPromptHook() func(string) string {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.promptHook
}

// SetDebugPrompt 设置调试提示
func (g *BaseAICallerConfig) SetDebugPrompt(debug bool) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.debugPrompt = debug
}

// GetDebugPrompt 获取调试提示状态
func (g *BaseAICallerConfig) GetDebugPrompt() bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.debugPrompt
}

// SetDebugEvent 设置调试事件
func (g *BaseAICallerConfig) SetDebugEvent(debug bool) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.debugEvent = debug
}

// GetDebugEvent 获取调试事件状态
func (g *BaseAICallerConfig) GetDebugEvent() bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.debugEvent
}

// SetEventHandler 设置事件处理器
func (g *BaseAICallerConfig) SetEventHandler(handler func(e *schema.AiOutputEvent)) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.eventHandler = handler
}

// GetEventHandler 获取事件处理器
func (g *BaseAICallerConfig) GetEventHandler() func(e *schema.AiOutputEvent) {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.eventHandler
}

// SetSaveEvent 设置保存事件
func (g *BaseAICallerConfig) SetSaveEvent(save bool) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.saveEvent = save
}

// GetSaveEvent 获取保存事件状态
func (g *BaseAICallerConfig) GetSaveEvent() bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return g.saveEvent
}

// InitToolManager 初始化工具管理器
func (g *BaseAICallerConfig) InitToolManager() error {
	if g.aiToolManager == nil {
		g.aiToolManager = buildinaitools.NewToolManager(g.aiToolManagerOption...)
	}
	return nil
}

// GetToolManager 获取工具管理器
func (g *BaseAICallerConfig) GetToolManager() *buildinaitools.AiToolManager {
	return g.aiToolManager
}

// SetToolManager 设置工具管理器
func (g *BaseAICallerConfig) SetToolManager(manager *buildinaitools.AiToolManager) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.aiToolManager = manager
}

// AddToolManagerOption 添加工具管理器选项
func (g *BaseAICallerConfig) AddToolManagerOption(option buildinaitools.ToolManagerOption) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.aiToolManagerOption = append(g.aiToolManagerOption, option)
}

// SetDisableOutputEventType 设置禁用输出事件类型
func (g *BaseAICallerConfig) SetDisableOutputEventType(types []string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.disableOutputEventType = types
}

// AddDisableOutputEventType 添加禁用输出事件类型
func (g *BaseAICallerConfig) AddDisableOutputEventType(eventType string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.disableOutputEventType = append(g.disableOutputEventType, eventType)
}

// IsOutputEventTypeDisabled 检查输出事件类型是否被禁用
func (g *BaseAICallerConfig) IsOutputEventTypeDisabled(eventType string) bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	for _, disabled := range g.disableOutputEventType {
		if disabled == eventType {
			return true
		}
	}
	return false
}

// GetAIAutoRetry 获取AI自动重试次数
func (g *BaseAICallerConfig) GetAIAutoRetry() int64 {
	return atomic.LoadInt64(&g.aiAutoRetry)
}

// SetAIAutoRetry 设置AI自动重试次数
func (g *BaseAICallerConfig) SetAIAutoRetry(count int64) {
	atomic.StoreInt64(&g.aiAutoRetry, count)
}

// GetAICallTokenLimit 获取AI调用token限制
func (g *BaseAICallerConfig) GetAICallTokenLimit() int64 {
	return atomic.LoadInt64(&g.aiCallTokenLimit)
}

// SetAICallTokenLimit 设置AI调用token限制
func (g *BaseAICallerConfig) SetAICallTokenLimit(limit int64) {
	atomic.StoreInt64(&g.aiCallTokenLimit, limit)
}

// === 额外的工具和配置方法 ===

// InputConsumptionCallback 输入消费回调
func (g *BaseAICallerConfig) InputConsumptionCallback(current int) {
	atomic.AddInt64(g.inputConsumption, int64(current))
}

// GetInputConsumptionCallback 获取输入消费回调函数
func (g *BaseAICallerConfig) GetInputConsumptionCallback() func(int) {
	return g.InputConsumptionCallback
}

// EmitBaseHandler 基础事件处理器
func (g *BaseAICallerConfig) EmitBaseHandler(e *schema.AiOutputEvent) {
	// 应用事件处理栈
	if g.eventProcessHandler != nil && !g.eventProcessHandler.IsEmpty() {
		// 从栈顶到栈底遍历处理器
		g.eventProcessHandler.ForeachStack(func(processor func(e *schema.AiOutputEvent) *schema.AiOutputEvent) bool {
			if processor != nil {
				e = processor(e)
				if e == nil {
					return false // 如果事件被处理器移除，停止处理
				}
			}
			return true // 继续处理下一个
		})
		if e == nil {
			return
		}
	}

	// 检查是否禁用此类型的事件
	if g.IsOutputEventTypeDisabled(string(e.Type)) {
		return
	}

	// 调用自定义事件处理器
	if g.eventHandler != nil {
		g.eventHandler(e)
	}

	// 发送事件
	if g.emitter != nil {
		g.emitter.Emit(e)
	}
}

// PushEventProcessor 添加事件处理器到栈顶
func (g *BaseAICallerConfig) PushEventProcessor(processor func(e *schema.AiOutputEvent) *schema.AiOutputEvent) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.eventProcessHandler == nil {
		g.eventProcessHandler = utils.NewStack[func(e *schema.AiOutputEvent) *schema.AiOutputEvent]()
	}
	g.eventProcessHandler.Push(processor)
}

// PopEventProcessor 从栈顶移除事件处理器
func (g *BaseAICallerConfig) PopEventProcessor() func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.eventProcessHandler == nil || g.eventProcessHandler.IsEmpty() {
		return nil
	}
	return g.eventProcessHandler.Pop()
}

// ClearEventProcessors 清空所有事件处理器
func (g *BaseAICallerConfig) ClearEventProcessors() {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.eventProcessHandler != nil {
		g.eventProcessHandler.Free()
	}
}

// WithCallback 包装AI回调以添加额外功能
func (g *BaseAICallerConfig) WithCallback(cb AICallbackType) AICallbackType {
	return func(config AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
		// 在这里可以添加请求预处理、日志记录等
		if g.debugPrompt && req != nil {
			log.Debugf("AI Request Prompt: %s", req.GetPrompt())
		}

		// 调用原始回调
		resp, err := cb(config, req)

		// 在这里可以添加响应后处理
		if err != nil && g.debugEvent {
			log.Debugf("AI Response Error: %v", err)
		}

		return resp, err
	}
}

// ApplyPromptHook 应用提示钩子
func (g *BaseAICallerConfig) ApplyPromptHook(prompt string) string {
	hook := g.GetPromptHook()
	if hook != nil {
		return hook(prompt)
	}
	return prompt
}

// IsDebugMode 检查是否处于调试模式
func (g *BaseAICallerConfig) IsDebugMode() bool {
	return g.GetDebugPrompt() || g.GetDebugEvent()
}

// ResetConsumption 重置消费量计数
func (g *BaseAICallerConfig) ResetConsumption() {
	atomic.StoreInt64(g.inputConsumption, 0)
	atomic.StoreInt64(g.outputConsumption, 0)
}

// GetTotalConsumption 获取总消费量
func (g *BaseAICallerConfig) GetTotalConsumption() int64 {
	return g.GetInputConsumption() + g.GetOutputConsumption()
}

func (g *BaseAICallerConfig) CallAfterInteractiveEventReleased(eventID string, invoke aitool.InvokeParams) {
	return
}

// Clone 创建配置的副本（浅拷贝）
func (g *BaseAICallerConfig) Clone() *BaseAICallerConfig {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	clone := &BaseAICallerConfig{
		ctx:                    g.ctx,
		runtimeId:              g.runtimeId,
		db:                     g.db,
		aiAutoRetry:            g.aiAutoRetry,
		aiTransactionAutoRetry: g.aiTransactionAutoRetry,
		aiCallTokenLimit:       g.aiCallTokenLimit,
		debugPrompt:            g.debugPrompt,
		debugEvent:             g.debugEvent,
		saveEvent:              g.saveEvent,
		promptHook:             g.promptHook,
		disableOutputEventType: append([]string{}, g.disableOutputEventType...),
	}

	// 初始化新的计数器
	clone.inputConsumption = new(int64)
	clone.outputConsumption = new(int64)
	clone.mutex = &sync.RWMutex{}
	clone.syncMutex = &sync.RWMutex{}
	clone.syncMap = make(map[string]func() any)
	clone.extendedActionCallback = make(map[string]func(config *BaseAICallerConfig, action *Action))

	return clone
}
