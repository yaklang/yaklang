package aid

import (
	"context"
	"io"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/ai/rag/rag_search_tool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// CoordinatorOption 定义配置 Coordinator 的选项接口
type CoordinatorOption func(c *Coordinator)

func WithCoordinatorPlanMocker(i func(coordinator *Coordinator) *PlanResponse) CoordinatorOption {
	return func(cod *Coordinator) {
		cod.PlanMocker = i
	}
}

func WithCoordinatorResultHandler(h func(c *Coordinator)) CoordinatorOption {
	return func(cod *Coordinator) {
		cod.ResultHandler = h
	}
}

func WithPromptContextProvider(provider *PromptContextProvider) aicommon.ConfigOption {
	return func(config *aicommon.Config) error {
		if err := aicommon.WithTimeline(provider.timeline)(config); err != nil {
			return err
		}

		if err := aicommon.WithAppendPersistentMemory(provider.PersistentMemory())(config); err != nil {
			return err
		}
		return nil
	}
}

// cycle import issue
func WithResultHandler(h func(c *Coordinator)) aicommon.ConfigOption {
	return func(config *aicommon.Config) error {
		return aicommon.WithAppendOtherOption(WithCoordinatorResultHandler(h))(config)
	}
}

func WithPlanMocker(i func(coordinator *Coordinator) *PlanResponse) aicommon.ConfigOption {
	return func(config *aicommon.Config) error {
		return aicommon.WithAppendOtherOption(WithCoordinatorPlanMocker(i))(config)
	}
}

// !!!!
func WithAiToolsSearchTool() aicommon.ConfigOption {
	return func(c *aicommon.Config) error {
		aiChatFunc := func(prompt string) (io.Reader, error) {
			response, err := ai.Chat(prompt)
			if err != nil {
				return nil, err
			}
			return strings.NewReader(response), nil
		}

		aiToolSearcher := rag_search_tool.NewComprehensiveSearcher[*aitool.Tool](rag_search_tool.AIToolVectorIndexName, aiChatFunc)
		return aicommon.WithAiToolManagerOptions(buildinaitools.WithSearchToolEnabled(true),
			buildinaitools.WithAIToolsSearcher(aiToolSearcher))(c)
	}
}

func WithAiForgeSearchTool() aicommon.ConfigOption {
	return func(c *aicommon.Config) error {
		aiChatFunc := func(prompt string) (io.Reader, error) {
			response, err := ai.Chat(prompt)
			if err != nil {
				return nil, err
			}
			return strings.NewReader(response), nil
		}

		forgeSearcher := rag_search_tool.NewComprehensiveSearcher[*schema.AIForge](rag_search_tool.ForgeVectorIndexName, aiChatFunc)
		return aicommon.WithAiToolManagerOptions(
			buildinaitools.WithForgeSearchToolEnabled(true),
			buildinaitools.WithAiForgeSearcher(forgeSearcher))(c)
	}
}

func (c *Coordinator) enableTaskAnalyze() {
	err := aicommon.WithGuardianEventTrigger(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, func(event *schema.AiOutputEvent, emitter aicommon.GuardianEmitter, caller aicommon.AICaller) {
		var plansUUID string
		var planTree string
		type analyzeItem struct {
			Index string `json:"index"`
			Goal  string `json:"goal"`
		}
		var analyzeItemList = make([]*analyzeItem, 0)
		err := jsonextractor.ExtractStructuredJSON(string(event.Content), jsonextractor.WithObjectCallback(func(data map[string]any) {
			if aitool.InvokeParams(data).Has("index") {
				analyzeItemList = append(analyzeItemList, &analyzeItem{
					Index: utils.InterfaceToString(data["index"]),
					Goal:  utils.InterfaceToString(data["goal"]),
				})
			}
		}), jsonextractor.WithRootMapCallback(func(data map[string]any) {
			id, ok := data["plans_id"]
			if ok {
				plansUUID = utils.InterfaceToString(id)
			}
			plans, ok := data["plans"]
			if ok {
				planTree = utils.InterfaceToString(plans)
			}
		}))
		if err != nil {
			return
		}

		analyze := func(currentPlanTree, currentUUID string, task *analyzeItem) {
			param := []*ypb.ExecParamItem{
				{
					Key:   "current_task_goal",
					Value: task.Goal,
				},
				{
					Key:   "task_tree",
					Value: currentPlanTree,
				},
			}

			action, err := ExecuteAIForge(c.Ctx, "task-analyst", param, aicommon.WithAICallback(c.OriginalAICallback))
			if err != nil {
				return
			}
			obj := action.GetInvokeParams("params")
			desc := obj.GetString("description")
			keywords := obj.GetStringSlice("keywords")
			emitter.EmitJson(schema.EVENT_PLAN_TASK_ANALYSIS, "task-analyst", map[string]any{
				"plans_id":    currentUUID,
				"description": desc,
				"keywords":    keywords,
				"index":       task.Index,
			})
		}
		for _, item := range analyzeItemList {
			go analyze(planTree, plansUUID, item)
		}

	})(c.Config)
	if err != nil {
		log.Errorf("coordinator: append utils option failed: %v", err)
	}
}

type Coordinator struct {
	*aicommon.Config
	userInput       string
	runtime         *runtime
	PlanMocker      func(config *Coordinator) *PlanResponse
	ContextProvider *PromptContextProvider

	ResultHandler func(cod *Coordinator)

	rootTask *AiTask
}

func (c *Coordinator) GetContextProvider() *PromptContextProvider {
	return c.ContextProvider
}

func (c *Coordinator) getCurrentTaskPlan() *AiTask {
	return c.runtime.RootTask
}

func (c *Coordinator) HandleSearch(query string, items *omap.OrderedMap[string, []string]) ([]*searchtools.KeywordSearchResult, error) {
	type ToolWithKeywords struct {
		Name     string `json:"Name"`
		Keywords string `json:"Keywords"`
	}

	toolsLists := []ToolWithKeywords{}
	items.ForEach(func(key string, value []string) bool {
		toolsLists = append(toolsLists, ToolWithKeywords{
			Name:     key,
			Keywords: strings.Join(value, ", "),
		})
		return true
	})
	var nonce = strings.ToLower(utils.RandStringBytes(6))
	prompt, err := c.quickBuildPrompt(__prompt_KeywordSearchPrompt, map[string]any{
		"NONCE":           nonce,
		"ContextProvider": c.ContextProvider,
		"UserRequirement": query,
		"ToolsLists":      toolsLists,
	})
	if err != nil {
		return nil, err
	}
	var callResults []*searchtools.KeywordSearchResult

	err = c.CallAITransaction(
		prompt,
		func(response *aicommon.AIResponse) error {
			action, err := aicommon.ExtractActionFromStream(c.Ctx, response.GetUnboundStreamReader(false), "keyword_search", aicommon.WithActionNonce(nonce))
			if err != nil {
				log.Errorf("extract aitool-keyword-search action failed: %v", err)
				return utils.Errorf("extract aitool-keyword-search failed: %v", err)
			}
			tools := action.GetInvokeParamsArray("matches")
			if len(tools) > 0 {
				for _, toolInfo := range tools {
					callResults = append(callResults, &searchtools.KeywordSearchResult{
						Key:             toolInfo.GetString("tool"),
						MatchedKeywords: toolInfo.GetStringSlice("matched_keywords"),
					})
				}
				return nil
			}
			return utils.Errorf("no tool found")
		})
	if err != nil {
		return nil, err
	}
	return callResults, nil

}

func NewCoordinator(userInput string, options ...aicommon.ConfigOption) (*Coordinator, error) {
	return NewCoordinatorContext(context.Background(), userInput, options...)
}

// NewCoordinatorContext  创建一个新的 Coordinator
func NewCoordinatorContext(ctx context.Context, userInput string, options ...aicommon.ConfigOption) (*Coordinator, error) {
	config := aicommon.NewConfig(ctx, options...)

	if utils.IsNil(config.Timeline.GetAICaller()) {
		config.Timeline.SetAICaller(config)
	}

	config.StartEventLoop(ctx)
	config.StartHotPatchLoop(ctx)
	config.Guardian.SetOutputEmitter(config.Id, config.EventHandler)
	config.Guardian.SetAICaller(config)
	c := &Coordinator{
		Config:    config,
		userInput: userInput,
	}

	c.ContextProvider = GetDefaultContextProvider()
	c.ContextProvider.SetTimelineInstance(config.Timeline)
	c.ContextProvider.BindCoordinator(c)
	if err := c.loadToolsViaOptions(); err != nil {
		return nil, utils.Errorf("coordinator: load tools (post-init) failed: %v", err)
	}

	return c, nil
}

func (c *Coordinator) loadToolsViaOptions() error {
	if c.AllowRequireForUserInteract {
		userPromptTool, err := c.CreateRequireUserInteract()
		if err != nil {
			log.Errorf("create user prompt tool: %v", err)
			return err
		}

		err = c.Config.AiToolManager.AppendTools(userPromptTool)
		if err != nil {
			log.Errorf("load require for user prompt tools: %v", err)
			return err
		}
	}

	if c.EnableAISearch {
		err := c.EnableToolManagerAISearch()
		if err != nil {
			log.Errorf("enable tool manager AI search: %v", err)
			return err
		}
	}

	if c.Config.EnableTaskAnalyze {
		c.enableTaskAnalyze()
	}

	for _, o := range c.OtherOption {
		switch co := o.(type) {
		case CoordinatorOption:
			co(c)
		}
	}

	return nil
}

func (c *Coordinator) CallAITransaction(prompt string, postHandler func(response *aicommon.AIResponse) error, requestOpts ...aicommon.AIRequestOption) error {
	return c.CallAiTransaction(prompt, c.CallAI, func(rsp *aicommon.AIResponse) error {
		if postHandler == nil {
			return nil
		}
		return postHandler(rsp)
	}, requestOpts...)
}

func (c *Coordinator) Run() error {
	c.registerPEModeInputEventCallback()
	c.EmitCurrentConfigInfo()
	c.CreateDatabaseSchema(c.userInput)
	c.EmitInfo("start to create plan request")
	planReq, err := c.createPlanRequest(c.userInput)
	if err != nil {
		c.EmitError("create planRequest failed: %v", err)
		return utils.Errorf("coordinator: create planRequest failed: %v", err)
	}

	c.EmitInfo("start to invoke plan request")
	rsp, err := planReq.Invoke()
	if err != nil {
		c.EmitError("invoke planRequest failed(first): %v", err)
		return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
	}

	// 审查
	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	c.EmitRequireReviewForPlan(rsp, ep.GetId())
	c.DoWaitAgree(c.GetContext(), ep)
	params := ep.GetParams()
	c.ReleaseInteractiveEvent(ep.GetId(), params)
	if params == nil {
		c.EmitError("user review params is nil, plan failed")
		return utils.Errorf("coordinator: user review params is nil")
	}
	c.EmitInfo("start to handle review plan response")
	rsp, err = planReq.handleReviewPlanResponse(rsp, params)
	if err != nil {
		c.EmitError("handle review plan response failed: %v", err)
		return utils.Errorf("coordinator: handle review plan response failed: %v", err)
	}

	if rsp.RootTask == nil {
		c.EmitError("root aiTask is nil, plan failed")
		return utils.Errorf("coordinator: root aiTask is nil")
	}
	// init aiTask
	// check tools
	root := rsp.RootTask
	c.ContextProvider.StoreRootTask(root)
	if len(root.Subtasks) <= 0 {
		c.EmitError("no subtasks found, this task is not a valid task")
		return utils.Errorf("coordinator: no subtasks found")
	}
	log.Infof("create aiTask pipeline: %v", root.Name)
	for stepIdx, taskIns := range root.Subtasks {
		log.Infof("step %d: %v", stepIdx, taskIns.Name)
	}
	alltools, err := c.AiToolManager.GetEnableTools()
	if err != nil {
		log.Warnf("coordinator: get all tools failed: %v", err)
	}
	if len(alltools) <= 0 {
		log.Warnf("coordinator: no tools enable")
	}

	c.EmitInfo("start to create runtime")
	rt := c.createRuntime()
	c.runtime = rt
	rt.Invoke(root)

	/*
		Result Handler
		Result Handler 是用户自定义的回调函数，用于处理 AI 的输出结果。
		用户可以在这个回调函数中处理 AI 的输出结果，或者将结果存储到数据库中。
	*/
	if c.ResultHandler != nil {
		c.ResultHandler(c)
	} else if c.GenerateReport {
		c.EmitInfo("start to generate report or result")
		prompt, err := c.generateReport()
		if err != nil {
			c.EmitError("generate report failed: %v", err)
			return utils.Error("coordinator: generate report failed")
		}
		aiRsp, err := c.CallAI(aicommon.NewAIRequest(prompt))
		if err != nil {
			c.EmitError("AICallback failed: %v", err)
			return utils.Errorf("coordinator: AICallback failed: %v", err)
		}
		output, err := io.ReadAll(aiRsp.GetOutputStreamReader("result", false, c.GetEmitter()))
		if err != nil {
			c.EmitError("read AICallback response failed: %v", err)
			return utils.Errorf("coordinator: read AICallback response failed: %v", err)
		}
		c.EmitStructured("result", map[string]any{
			"data": string(output),
		})
	}
	// maybe need special type to tell user run finished,just wait db insert?
	c.EmitInfo("coordinator run finished")
	c.Wait()
	return nil
}

func (c *Coordinator) GetPromptContextProvider() *PromptContextProvider {
	return c.ContextProvider
}

func (c *Coordinator) registerPEModeInputEventCallback() {
	c.InputEventManager.RegisterSyncCallback(aicommon.SYNC_TYPE_PLAN, func(event *ypb.AIInputEvent) error {
		if c.rootTask != nil {
			c.EmitSyncJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
				"root_task": c.rootTask,
			},
				event.SyncID,
			)
		}
		return nil
	})
}

func (c *Coordinator) unregisterPEModeInputEventCallback() {
	c.InputEventManager.UnRegisterSyncCallback(aicommon.SYNC_TYPE_PLAN)
}

func (c *Coordinator) newPlanResponse(rootTask *AiTask) *PlanResponse {
	c.rootTask = rootTask
	return &PlanResponse{
		RootTask: rootTask,
	}
}

func (c *Coordinator) ProcessExtendedActionCallback(resp string) {
	actions := aicommon.ExtractAllAction(resp)
	for _, action := range actions {
		if cb, ok := c.ExtendedActionCallback[action.Name()]; ok {
			cb(c.Config, action)
		}
	}
}

func (c *Coordinator) EnableToolManagerAISearch() error {
	keyWordSearcher := func(query string, searchList []searchtools.AISearchable) ([]searchtools.AISearchable, error) {
		keywords := omap.NewOrderedMap[string, []string](nil)
		toolMap := map[string]searchtools.AISearchable{}
		for _, tool := range searchList {
			keywords.Set(tool.GetName(), tool.GetKeywords())
			toolMap[tool.GetName()] = tool
		}
		searchResults, err := c.HandleSearch(query, keywords)
		if err != nil {
			return nil, err
		}
		tools := []searchtools.AISearchable{}
		for _, result := range searchResults {
			tools = append(tools, toolMap[result.Key])
		}
		return tools, nil
	}

	aiToolKeywordSearcher := func(query string, searchList []*aitool.Tool) ([]*aitool.Tool, error) {
		tools, err := keyWordSearcher(query, lo.Map(searchList, func(item *aitool.Tool, _ int) searchtools.AISearchable {
			return item
		}))
		if err != nil {
			return nil, err
		}
		res := lo.Map(tools, func(item searchtools.AISearchable, _ int) *aitool.Tool {
			return item.(*aitool.Tool)
		})
		return res, nil
	}

	forgeKeywordSearcher := func(query string, searchList []*schema.AIForge) ([]*schema.AIForge, error) {
		tools, err := keyWordSearcher(query, lo.Map(searchList, func(item *schema.AIForge, _ int) searchtools.AISearchable {
			return item
		}))
		if err != nil {
			return nil, err
		}
		res := lo.Map(tools, func(item searchtools.AISearchable, _ int) *schema.AIForge {
			return item.(*schema.AIForge)
		})
		return res, nil
	}

	aiToolRagSearcher, err := rag_search_tool.NewRAGSearcher[*aitool.Tool](rag_search_tool.AIToolVectorIndexName)
	if err != nil {
		log.Errorf("create ai tool rag searcher failed: %v", err)
	}
	forgeRagSearcher, err := rag_search_tool.NewRAGSearcher[*schema.AIForge](rag_search_tool.ForgeVectorIndexName)
	if err != nil {
		log.Errorf("create forge rag searcher failed: %v", err)
	}
	aiToolRagSearcher = rag_search_tool.NewMergeSearchr(aiToolRagSearcher, aiToolKeywordSearcher)
	forgeRagSearcher = rag_search_tool.NewMergeSearchr(forgeRagSearcher, forgeKeywordSearcher)
	err = c.AiToolManager.EnableAIToolSearch(aiToolRagSearcher)
	if err != nil {
		return utils.Errorf("aiToolManager.EnableAIToolSearch failed: %v", err)
	}

	err = c.AiToolManager.EnableAIForgeSearch(forgeRagSearcher)
	if err != nil {
		return utils.Errorf("aiToolManager.EnableAIForgeSearch failed: %v", err)
	}
	return nil
}
