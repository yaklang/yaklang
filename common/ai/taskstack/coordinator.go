package taskstack

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"sync"
)

// CoordinatorOption 定义配置 Coordinator 的选项接口
type CoordinatorOption func(c *Coordinator)

type Coordinator struct {
	id string

	eventEmitMutex sync.Mutex
	eventHandler   func(e *Event)
	userInput      string
	aiCallback     AICallbackType
	PlanOption     []PlanOption
	TaskOption     []TaskOption
	Tools          []*Tool
}

// NewCoordinator 创建一个新的 Coordinator
func NewCoordinator(userInput string, options ...any) *Coordinator {
	coordinatorId := uuid.New().String()
	c := &Coordinator{
		id:         coordinatorId,
		userInput:  userInput,
		PlanOption: []PlanOption{},
		TaskOption: []TaskOption{},
		Tools:      []*Tool{},
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
		c.userInput = rawUserInput
	}
}

func WithCoordinator_Tool(tools ...*Tool) CoordinatorOption {
	return func(c *Coordinator) {
		c.Tools = append(c.Tools, tools...)
	}
}

func WithCoordinator_AICallback(callback AICallbackType) CoordinatorOption {
	return func(c *Coordinator) {
		c.aiCallback = callback
	}
}

func (c *Coordinator) Run() error {
	if c.aiCallback == nil {
		return utils.Error("taskstack coordinator run failed: no AICallback found")
	}

	planReq, err := CreatePlanRequest(c.userInput, c.PlanOption...)
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

	prompt, err := c.generateReport(runtime)
	if err != nil {
		return utils.Error("coordinator: generate report failed")
	}
	aiRsp, err := c.aiCallback(NewAIRequest(prompt))
	if err != nil {
		return utils.Errorf("coordinator: AICallback failed: %v", err)
	}
	output, err := io.ReadAll(aiRsp.Reader())
	if err != nil {
		return utils.Errorf("coordinator: read AICallback response failed: %v", err)
	}
	// todo: callback output
	_ = output
	return nil
}
