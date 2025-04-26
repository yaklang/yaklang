package aid

import (
	"context"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
		return cb(c.config, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

func NewCoordinator(userInput string, options ...Option) (*Coordinator, error) {
	return NewCoordinatorContext(context.Background(), userInput, options...)
}

// NewCoordinator 创建一个新的 Coordinator
func NewCoordinatorContext(ctx context.Context, userInput string, options ...Option) (*Coordinator, error) {
	config := newConfig(ctx)
	for _, opt := range options {
		err := opt(config)
		if err != nil {
			return nil, utils.Errorf("coordinator: apply option failed: %v", err)
		}
	}
	config.startEventLoop(ctx)

	if config.aiToolManager == nil {
		config.aiToolManager = buildinaitools.NewDefaultToolManager(config.tools)
	}
	c := &Coordinator{
		config:    config,
		userInput: userInput,
	}
	config.memory.StoreQuery(c.userInput)
	config.memory.StoreTools(func() []*aitool.Tool {
		alltools, err := config.aiToolManager.GetAllTools()
		if err != nil {
			log.Errorf("coordinator: get all tools failed: %v", err)
			return nil
		}
		return alltools
	})
	return c, nil
}

func (c *Coordinator) Run() error {

	c.CreateDatabaseSchema()
	c.config.EmitInfo("start to create plan request")
	planReq, err := c.createPlanRequest(c.userInput)
	if err != nil {
		c.config.EmitError("create planRequest failed: %v", err)
		return utils.Errorf("coordinator: create planRequest failed: %v", err)
	}

	c.config.EmitInfo("start to invoke plan request")
	rsp, err := planReq.Invoke()
	if err != nil {
		c.config.EmitError("invoke planRequest failed(first): %v", err)
		return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
	}

	// 审查
	ep := c.config.epm.createEndpoint()
	ep.SetDefaultSuggestionContinue()

	c.config.EmitRequireReviewForPlan(rsp, ep.id)
	c.config.doWaitAgree(nil, ep)
	params := ep.GetParams()
	c.config.memory.StoreInteractiveUserInput(ep.id, params)
	if params == nil {
		c.config.EmitError("user review params is nil, plan failed")
		return utils.Errorf("coordinator: user review params is nil")
	}
	c.config.EmitInfo("start to handle review plan response")
	rsp, err = planReq.handleReviewPlanResponse(rsp, params)
	if err != nil {
		c.config.EmitError("handle review plan response failed: %v", err)
		return utils.Errorf("coordinator: handle review plan response failed: %v", err)
	}

	if rsp.RootTask == nil {
		c.config.EmitError("root aiTask is nil, plan failed")
		return utils.Errorf("coordinator: root aiTask is nil")
	}
	// init aiTask
	// check tools
	root := rsp.RootTask
	c.config.memory.StoreRootTask(root)
	if len(root.Subtasks) <= 0 {
		c.config.EmitError("no subtasks found, this task is not a valid task")
		return utils.Errorf("coordinator: no subtasks found")
	}
	log.Infof("create aiTask pipeline: %v", root.Name)
	for stepIdx, taskIns := range root.Subtasks {
		log.Infof("step %d: %v", stepIdx, taskIns.Name)
	}
	alltools, err := c.config.aiToolManager.GetAllTools()
	if err != nil {
		log.Warnf("coordinator: get all tools failed: %v", err)
	}
	if len(alltools) <= 0 {
		log.Warnf("coordinator: no tools found")
	}

	c.config.EmitInfo("start to create runtime")
	rt := c.createRuntime()
	rt.Invoke(root)

	/*
		Result Handler
		Result Handler 是用户自定义的回调函数，用于处理 AI 的输出结果。
		用户可以在这个回调函数中处理 AI 的输出结果，或者将结果存储到数据库中。
	*/
	if c.config.resultHandler != nil {
		c.config.resultHandler(c.config)
		return nil
	}

	c.config.EmitInfo("start to generate report or result")
	prompt, err := c.generateReport()
	if err != nil {
		c.config.EmitError("generate report failed: %v", err)
		return utils.Error("coordinator: generate report failed")
	}
	aiRsp, err := c.callAI(NewAIRequest(prompt))
	if err != nil {
		c.config.EmitError("AICallback failed: %v", err)
		return utils.Errorf("coordinator: AICallback failed: %v", err)
	}
	output, err := io.ReadAll(aiRsp.GetOutputStreamReader("result", false, c.config))
	if err != nil {
		c.config.EmitError("read AICallback response failed: %v", err)
		return utils.Errorf("coordinator: read AICallback response failed: %v", err)
	}
	c.config.EmitStructured("result", map[string]any{
		"data": string(output),
	})
	return nil
}
