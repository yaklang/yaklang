package yak

import "github.com/tevino/abool"

type Task struct {
	TaskID string
	Code   string

	isRunning  *abool.AtomicBool
	isFinished *abool.AtomicBool

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
