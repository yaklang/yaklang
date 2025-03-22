package aid

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
)

// CoordinatorOption 定义配置 Coordinator 的选项接口
type CoordinatorOption func(c *Coordinator)

type Coordinator struct {
	userInput string
	config    *Config
}

func (c *Coordinator) callAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		c.config.coordinatorAICallback,
		c.config.planAICallback,
		c.config.taskAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

// NewCoordinator 创建一个新的 Coordinator
func NewCoordinator(userInput string, options ...Option) (*Coordinator, error) {
	config := newConfig()
	for _, opt := range options {
		err := opt(config)
		if err != nil {
			return nil, utils.Errorf("coordinator: apply option failed: %v", err)
		}
	}

	c := &Coordinator{
		config:    config,
		userInput: userInput,
	}
	return c, nil
}

func (c *Coordinator) Run() error {
	c.config.EmitInfo("start to create plan request")
	planReq, err := c.createPlanRequest(c.userInput)
	if err != nil {
		c.config.EmitError("create planRequest failed: %v", err)
		return utils.Errorf("coordinator: create planRequest failed: %v", err)
	}

	c.config.EmitInfo("start to invoke plan request")
	rsp, err := planReq.Invoke()
	if err != nil {
		c.config.EmitError("invoke planRequest failed: %v", err)
		return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
	}

	if rsp.RootTask == nil {
		c.config.EmitError("root aiTask is nil, plan failed")
		return utils.Errorf("coordinator: root aiTask is nil")
	}
	// init aiTask
	// check tools
	root := rsp.RootTask
	if len(root.Subtasks) <= 0 {
		c.config.EmitError("no subtasks found, this task is not a valid task")
		return utils.Errorf("coordinator: no subtasks found")
	}
	log.Infof("create aiTask pipeline: %v", root.Name)
	for stepIdx, taskIns := range root.Subtasks {
		log.Infof("step %d: %v", stepIdx, taskIns.Name)
	}
	if len(root.config.tools) <= 0 {
		if len(c.config.tools) <= 0 {
			log.Warnf("coordinator: no tools found")
		}
	}

	c.config.EmitInfo("start to create runtime")
	rt := c.createRuntime()
	rt.Invoke(root)
	prompt, err := c.generateReport(rt)
	if err != nil {
		return utils.Error("coordinator: generate report failed")
	}
	aiRsp, err := c.callAI(NewAIRequest(prompt))
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
