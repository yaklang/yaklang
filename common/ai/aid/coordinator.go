package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"io"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// CoordinatorOption 定义配置 Coordinator 的选项接口
type CoordinatorOption func(c *Coordinator)

type Coordinator struct {
	userInput string
	config    *Config
}

func (c *Coordinator) CallAI(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	for _, cb := range []aicommon.AICallbackType{
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
	consts.WaitAIDatabasePostInit()

	config := NewConfig(ctx)
	for _, opt := range options {
		err := opt(config)
		if err != nil {
			return nil, utils.Errorf("coordinator: apply option failed: %v", err)
		}
	}
	if config.memory == nil {
		config.memory = GetDefaultMemory()
	}
	if utils.IsNil(config.memory.timeline.GetAICaller()) {
		config.memory.timeline.SetAICaller(config)
	}

	if err := config.loadToolsViaOptions(); err != nil {
		return nil, utils.Errorf("coordinator: load tools (post-init) failed: %v", err)
	}
	config.startEventLoop(ctx)
	config.startHotpatchLoop(ctx)
	config.guardian.SetOutputEmitter(config.id, config.eventHandler)
	config.guardian.SetAICaller(config)
	c := &Coordinator{
		config:    config,
		userInput: userInput,
	}
	config.memory.BindCoordinator(c)
	config.InitToolManager()
	return c, nil
}

func (c *Coordinator) CallAITransaction(prompt string, postHandler func(response *aicommon.AIResponse) error, requestOpts ...aicommon.AIRequestOption) error {
	return c.config.callAiTransaction(prompt, c.CallAI, func(rsp *aicommon.AIResponse) error {
		if postHandler == nil {
			return nil
		}
		return postHandler(rsp)
	}, requestOpts...)
}

func (c *Coordinator) GetConfig() *Config {
	return c.config
}

func (c *Coordinator) Run() error {
	c.config.EmitCurrentConfigInfo()
	c.CreateDatabaseSchema(c.userInput)
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
	ep := c.config.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	c.config.EmitRequireReviewForPlan(rsp, ep.GetId())
	c.config.DoWaitAgree(nil, ep)
	params := ep.GetParams()
	c.config.ReleaseInteractiveEvent(ep.GetId(), params)
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
	alltools, err := c.config.aiToolManager.GetEnableTools()
	if err != nil {
		log.Warnf("coordinator: get all tools failed: %v", err)
	}
	if len(alltools) <= 0 {
		log.Warnf("coordinator: no tools enable")
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
	} else if c.config.generateReport {
		c.config.EmitInfo("start to generate report or result")
		prompt, err := c.generateReport()
		if err != nil {
			c.config.EmitError("generate report failed: %v", err)
			return utils.Error("coordinator: generate report failed")
		}
		aiRsp, err := c.CallAI(aicommon.NewAIRequest(prompt))
		if err != nil {
			c.config.EmitError("AICallback failed: %v", err)
			return utils.Errorf("coordinator: AICallback failed: %v", err)
		}
		output, err := io.ReadAll(aiRsp.GetOutputStreamReader("result", false, c.config.GetEmitter()))
		if err != nil {
			c.config.EmitError("read AICallback response failed: %v", err)
			return utils.Errorf("coordinator: read AICallback response failed: %v", err)
		}
		c.config.EmitStructured("result", map[string]any{
			"data": string(output),
		})
	}
	// maybe need special type to tell user run finished,just wait db insert?
	c.config.EmitInfo("coordinator run finished")
	c.config.Wait()
	return nil
}
