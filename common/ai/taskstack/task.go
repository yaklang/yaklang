package taskstack

import (
	"github.com/yaklang/yaklang/common/utils"
)

type Task struct {
	Name     string
	Goal     string
	Subtasks []Task
}

type Runtime struct {
	Freeze bool
	Task   Task
	Stack  *utils.Stack[Task]
}
