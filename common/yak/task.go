package yak

import "github.com/yaklang/yaklang/common/utils"

type Task struct {
	TaskID string
	Code   string

	isRunning  *utils.AtomicBool
	isFinished *utils.AtomicBool

	Output   []string
	Log      []string
	Alert    []string
	Finished []string
	Failed   []string
}

func (t *Task) IsRunning() bool {
	return t.isRunning.IsSet()
}

func (t *Task) IsFinished() bool {
	return t.isFinished.IsSet()
}
