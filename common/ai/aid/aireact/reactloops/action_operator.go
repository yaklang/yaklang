package reactloops

import (
	"bytes"
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

type LoopActionHandlerOperator struct {
	feedbacks            *bytes.Buffer
	disallowLoopExitOnce *utils.Once
	disallowLoopExit     bool

	terminateOperateOnce *utils.Once

	isContinued  bool
	isTerminated bool
	failedError  error
	isSilence    bool

	task aicommon.AIStatefulTask

	// 自我反思相关
	reflectionLevel      ReflectionLevel
	customReflectionData map[string]interface{}
}

func (r *LoopActionHandlerOperator) GetContext() context.Context {
	return r.task.GetContext()
}

func newLoopActionHandlerOperator(task aicommon.AIStatefulTask) *LoopActionHandlerOperator {
	return &LoopActionHandlerOperator{
		feedbacks:            new(bytes.Buffer),
		terminateOperateOnce: utils.NewOnce(),
		disallowLoopExitOnce: utils.NewOnce(),
		task:                 task,
		reflectionLevel:      ReflectionLevel_None, // 默认不反思，由策略决定
		customReflectionData: make(map[string]interface{}),
	}
}

func (l *LoopActionHandlerOperator) GetTask() aicommon.AIStatefulTask {
	return l.task
}

func (l *LoopActionHandlerOperator) DisallowNextLoopExit() {
	l.disallowLoopExitOnce.Do(func() {
		l.disallowLoopExit = true
	})
}

func (l *LoopActionHandlerOperator) Continue() {
	l.terminateOperateOnce.Do(func() {
		l.isContinued = true
	})
}

func (l *LoopActionHandlerOperator) Exit() {
	l.terminateOperateOnce.Do(func() {
		l.isTerminated = true
	})
}

func (l *LoopActionHandlerOperator) IsTerminated() (bool, error) {
	return l.isTerminated, l.failedError
}

func (l *LoopActionHandlerOperator) IsContinued() bool {
	return l.isContinued
}

func (l *LoopActionHandlerOperator) Fail(i any) {
	l.terminateOperateOnce.Do(func() {
		l.failedError = utils.Error(i)
		l.isTerminated = true
	})
}

func (l *LoopActionHandlerOperator) Feedback(i any) {
	_, _ = l.feedbacks.WriteString(utils.InterfaceToString(i))
	l.feedbacks.WriteRune('\n')
}

func (l *LoopActionHandlerOperator) GetFeedback() *bytes.Buffer {
	return l.feedbacks
}

func (l *LoopActionHandlerOperator) GetDisallowLoopExit() bool {
	return l.disallowLoopExit
}

func (l *LoopActionHandlerOperator) MarkSilence(i ...bool) {
	if len(i) <= 0 {
		l.isSilence = true
		return
	}
	l.isSilence = i[0]
}

// SetReflectionLevel 设置该 action 执行后的反思级别
// action 可以在执行过程中根据实际情况动态设置反思级别
func (l *LoopActionHandlerOperator) SetReflectionLevel(level ReflectionLevel) {
	l.reflectionLevel = level
}

// GetReflectionLevel 获取当前设置的反思级别
func (l *LoopActionHandlerOperator) GetReflectionLevel() ReflectionLevel {
	return l.reflectionLevel
}

// SetReflectionData 设置自定义反思数据
// action 可以添加额外的上下文信息用于反思分析
func (l *LoopActionHandlerOperator) SetReflectionData(key string, value interface{}) {
	l.customReflectionData[key] = value
}

// GetReflectionData 获取自定义反思数据
func (l *LoopActionHandlerOperator) GetReflectionData() map[string]interface{} {
	return l.customReflectionData
}
