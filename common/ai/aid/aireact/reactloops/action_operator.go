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
