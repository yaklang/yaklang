package taskstack

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// CoordinatorOption 定义配置 Coordinator 的选项接口
type CoordinatorOption func(c *Coordinator)

// CoordinatorResult 定义 Coordinator 执行的结果
type CoordinatorResult struct {
	PlanName     string
	TaskResults  []string
	Error        error
	AllCompleted bool
}

type Coordinator struct {
	RawUserInput string
	PlanOption   []PlanOption
	TaskOption   []TaskOption
	Tools        []*Tool
}

// NewCoordinator 创建一个新的 Coordinator
func NewCoordinator(userInput string, options ...any) *Coordinator {
	c := &Coordinator{
		RawUserInput: userInput,
		PlanOption:   []PlanOption{},
		TaskOption:   []TaskOption{},
		Tools:        []*Tool{},
	}

	// 应用所有选项
	for _, option := range options {
		switch option := option.(type) {
		case CoordinatorOption:
			option(c)
		case PlanOption:
			c.PlanOption = append(c.PlanOption, option)
		case TaskOption:
			c.TaskOption = append(c.TaskOption, option)
		case *Tool:
			c.Tools = append(c.Tools, option)
		}
	}

	return c
}

func WithCoordinator_RawUserInput(rawUserInput string) CoordinatorOption {
	return func(c *Coordinator) {
		c.RawUserInput = rawUserInput
	}
}

func WithCoordinator_Tool(tools ...*Tool) CoordinatorOption {
	return func(c *Coordinator) {
		c.Tools = append(c.Tools, tools...)
	}
}

func (c *Coordinator) Run() error {
	planReq, err := CreatePlanRequest(c.RawUserInput, c.PlanOption...)
	if err != nil {
		return utils.Errorf("coordinator: create PlanRequest failed: %v", err)
	}
	rsp, err := planReq.Invoke()
	if err != nil {
		return utils.Errorf("coordinator: invoke PlanRequest failed: %v", err)
	}
	if rsp.RootTask == nil {
		return utils.Errorf("coordinator: root task is nil")
	}
	// init task
	// check tools
	root := rsp.RootTask
	if len(root.Subtasks) <= 0 {
		return utils.Errorf("coordinator: no subtasks found")
	}
	for _, taskOption := range c.TaskOption {
		taskOption.Apply(root)
	}
	log.Infof("create task pipeline: %v", root.Name)
	for stepIdx, task := range root.Subtasks {
		log.Infof("step %d: %v", stepIdx, task.Name)
	}
	if len(root.tools) <= 0 {
		if len(c.Tools) <= 0 {
			log.Warnf("coordinator: no tools found")
		} else {
			root.tools = append(root.tools, c.Tools...)
			root.applyToolsForAllSubtasks()
		}
	}

	runtime := CreateRuntime()
	runtime.Invoke(root)

	return utils.Error("not implemented")
}
