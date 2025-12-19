package aicommon

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIEngineOperator 定义了 AI 引擎操作器的核心接口
// 这个接口抽象了 ReAct 和 Coordinator 等不同 AI 执行模式的共同操作
// 允许 aiengine 模块在不直接导入具体实现的情况下使用这些功能
type AIEngineOperator interface {
	// ========== 核心接口 ==========

	// SendInputEvent 发送输入事件到 AI 引擎
	// 这是与 AI 引擎交互的主要入口点
	SendInputEvent(event *ypb.AIInputEvent) error

	// Wait 等待所有任务完成
	// 阻塞直到所有正在执行的任务都完成或被取消
	Wait()

	// IsFinished 检查所有任务是否已完成
	// 返回 true 表示当前没有正在执行的任务
	IsFinished() bool

	// ========== 便捷包装接口 ==========

	// SendFreeInput 发送自由文本输入
	// 这是 SendInputEvent 的便捷包装，用于发送用户的自由文本查询
	SendFreeInput(input string) error

	// SendInteractiveResponse 发送交互式响应
	// 用于回复 AI 提出的问题或需要用户确认的操作
	SendInteractiveResponse(response string) error

	// SendStartEvent 发送启动事件
	// 用于初始化 AI 引擎并传递启动参数
	SendStartEvent(params *ypb.AIStartParams) error

	// SendSyncEvent 发送同步事件
	// 用于请求特定类型的同步信息
	SendSyncEvent(syncType string, jsonInput string) error

	// ========== 配置热更新接口 ==========

	// SendConfigHotpatch 发送配置热更新事件
	// 用于在运行时动态更新 AI 引擎的配置
	SendConfigHotpatch(config map[string]interface{}) error
}

// AIEngineOperatorBase 提供了 AIEngineOperator 接口的基础实现
// 基于 SendInputEvent 方法实现所有包装接口
type AIEngineOperatorBase struct {
	// SendInputEventFunc 是 SendInputEvent 的实际实现函数
	SendInputEventFunc func(event *ypb.AIInputEvent) error

	// WaitFunc 是 Wait 的实际实现函数
	WaitFunc func()

	// IsFinishedFunc 是 IsFinished 的实际实现函数
	IsFinishedFunc func() bool
}

// SendInputEvent 实现 AIEngineOperator 接口
func (b *AIEngineOperatorBase) SendInputEvent(event *ypb.AIInputEvent) error {
	if b.SendInputEventFunc == nil {
		return nil
	}
	return b.SendInputEventFunc(event)
}

// Wait 实现 AIEngineOperator 接口
func (b *AIEngineOperatorBase) Wait() {
	if b.WaitFunc != nil {
		b.WaitFunc()
	}
}

// IsFinished 实现 AIEngineOperator 接口
func (b *AIEngineOperatorBase) IsFinished() bool {
	if b.IsFinishedFunc == nil {
		return true
	}
	return b.IsFinishedFunc()
}

// SendFreeInput 发送自由文本输入
func (b *AIEngineOperatorBase) SendFreeInput(input string) error {
	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   input,
	}
	return b.SendInputEvent(event)
}

// SendInteractiveResponse 发送交互式响应
func (b *AIEngineOperatorBase) SendInteractiveResponse(response string) error {
	event := &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveJSONInput: response,
	}
	return b.SendInputEvent(event)
}

// SendStartEvent 发送启动事件
func (b *AIEngineOperatorBase) SendStartEvent(params *ypb.AIStartParams) error {
	event := &ypb.AIInputEvent{
		IsStart: true,
		Params:  params,
	}
	return b.SendInputEvent(event)
}

// SendSyncEvent 发送同步事件
func (b *AIEngineOperatorBase) SendSyncEvent(syncType string, jsonInput string) error {
	event := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      syncType,
		SyncJsonInput: jsonInput,
	}
	return b.SendInputEvent(event)
}

// SendConfigHotpatch 发送配置热更新事件
func (b *AIEngineOperatorBase) SendConfigHotpatch(config map[string]interface{}) error {
	// 配置热更新需要特殊的事件类型
	event := &ypb.AIInputEvent{
		IsConfigHotpatch: true,
	}
	return b.SendInputEvent(event)
}

// NewAIEngineOperatorBase 创建 AIEngineOperatorBase 实例
func NewAIEngineOperatorBase(
	sendInputEvent func(event *ypb.AIInputEvent) error,
	wait func(),
	isFinished func() bool,
) *AIEngineOperatorBase {
	return &AIEngineOperatorBase{
		SendInputEventFunc: sendInputEvent,
		WaitFunc:           wait,
		IsFinishedFunc:     isFinished,
	}
}

// WrapToAIEngineOperator 将实现了基础接口的对象包装为 AIEngineOperator
// 这允许将现有的 ReAct 或其他实现快速转换为 AIEngineOperator
func WrapToAIEngineOperator(
	sendInputEvent func(event *ypb.AIInputEvent) error,
	wait func(),
	isFinished func() bool,
) AIEngineOperator {
	return NewAIEngineOperatorBase(sendInputEvent, wait, isFinished)
}

