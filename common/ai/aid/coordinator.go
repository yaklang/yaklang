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

// PlanExecutingLoadingStatusKey is the key used to emit loading status events for plan execution
// Similar to ReActLoadingStatusKey in reactloops, this allows UI to show current execution phase
const PlanExecutingLoadingStatusKey = "plan-executing-loading-status-key"

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

		if err := aicommon.WithAppendPersistentContext(provider.PersistentMemory())(config); err != nil {
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

func (c *Coordinator) GetAIConfig() *aicommon.Config {
	return c.Config
}

// planLoadingStatus emits the current loading status for plan execution
// This allows UI to display the current phase of plan execution
func (c *Coordinator) planLoadingStatus(status string) {
	if c.Emitter == nil {
		return
	}
	log.Infof("plan-executing loading status updated: %v", status)
	c.EmitStatus(PlanExecutingLoadingStatusKey, status)
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
	c.CreateDatabaseSchema(c.userInput)
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
	c.planLoadingStatus("初始化 / Initializing...")
	defer c.planLoadingStatus("end")

	c.registerPEModeInputEventCallback()
	c.EmitCurrentConfigInfo()

	// Phase 1: Creating plan
	c.planLoadingStatus("创建任务计划 / Creating Plan...")
	c.EmitInfo("start to create plan request")
	planReq, err := c.createPlanRequest(c.userInput)
	if err != nil {
		c.planLoadingStatus("计划创建失败 / Plan Creation Failed")
		c.EmitError("create planRequest failed: %v", err)
		return utils.Errorf("coordinator: create planRequest failed: %v", err)
	}

	// Phase 2: Invoking plan (AI generating plan)
	c.planLoadingStatus("等待 AI 生成计划 / Waiting AI to Generate Plan...")
	c.EmitInfo("start to invoke plan request")
	rsp, err := planReq.Invoke()
	if err != nil {
		c.planLoadingStatus("计划生成失败 / Plan Generation Failed")
		c.EmitError("invoke planRequest failed(first): %v", err)
		return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
	}

	// Phase 3: Waiting for user review
	c.planLoadingStatus("等待用户审查计划 / Waiting User to Review Plan...")
	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	c.EmitRequireReviewForPlan(rsp, ep.GetId())
	c.DoWaitAgree(c.GetContext(), ep)
	params := ep.GetParams()
	c.ReleaseInteractiveEvent(ep.GetId(), params)
	if params == nil {
		c.planLoadingStatus("用户审查失败 / User Review Failed")
		c.EmitError("user review params is nil, plan failed")
		return utils.Errorf("coordinator: user review params is nil")
	}

	// Phase 4: Processing user review
	c.planLoadingStatus("处理用户审查结果 / Processing User Review...")
	c.EmitInfo("start to handle review plan response")
	rsp, err = planReq.handleReviewPlanResponse(rsp, params)
	if err != nil {
		c.planLoadingStatus("处理审查结果失败 / Review Processing Failed")
		c.EmitError("handle review plan response failed: %v", err)
		return utils.Errorf("coordinator: handle review plan response failed: %v", err)
	}

	if rsp.RootTask == nil {
		c.planLoadingStatus("任务计划无效 / Invalid Task Plan")
		c.EmitError("root aiTask is nil, plan failed")
		return utils.Errorf("coordinator: root aiTask is nil")
	}

	// Phase 5: Initializing tasks
	c.planLoadingStatus("初始化任务队列 / Initializing Task Queue...")
	root := rsp.RootTask
	c.rootTask = root
	c.ContextProvider.StoreRootTask(root)
	if len(root.Subtasks) <= 0 {
		c.planLoadingStatus("无有效子任务 / No Valid Subtasks")
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

	// Phase 6: Executing tasks
	c.planLoadingStatus("执行任务中 / Executing Tasks...")
	c.EmitInfo("start to create runtime")
	rt := c.createRuntime()
	c.runtime = rt
	err = rt.Invoke(root)
	if err != nil {
		c.planLoadingStatus("任务执行失败 / Task Execution Failed")
		return err
	}

	// Phase 7: Generating result/report
	c.planLoadingStatus("生成执行结果 / Generating Results...")
	/*
		Result Handler
		Result Handler 是用户自定义的回调函数，用于处理 AI 的输出结果。
		用户可以在这个回调函数中处理 AI 的输出结果，或者将结果存储到数据库中。
	*/
	if c.ResultHandler != nil {
		c.ResultHandler(c)
	} else if c.GenerateReport {
		c.planLoadingStatus("进入报告生成专注模式 / Entering Report Generation Focus Mode...")
		c.EmitInfo("start report generation via focus mode loop")
		if err := c.generateReportViaFocusMode(); err != nil {
			c.planLoadingStatus("报告生成失败 / Report Generation Failed")
			c.EmitError("report generation via focus mode failed: %v", err)
			return utils.Errorf("coordinator: report generation failed: %v", err)
		}
	}

	// Phase 8: Completed
	c.planLoadingStatus("执行完成 / Execution Completed")
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

	c.InputEventManager.RegisterSyncCallback(aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN, c.HandleSkipSubtaskInPlan)
	c.InputEventManager.RegisterSyncCallback(aicommon.SYNC_TYPE_REDO_SUBTASK_IN_PLAN, c.HandleRedoSubtaskInPlan)
}

func (c *Coordinator) unregisterPEModeInputEventCallback() {
	c.InputEventManager.UnRegisterSyncCallback(aicommon.SYNC_TYPE_PLAN)
	c.InputEventManager.UnRegisterSyncCallback(aicommon.SYNC_TYPE_SKIP_SUBTASK_IN_PLAN)
	c.InputEventManager.UnRegisterSyncCallback(aicommon.SYNC_TYPE_REDO_SUBTASK_IN_PLAN)
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

// FindSubtaskByIndex 根据任务索引查找子任务
// 索引格式为 "1-1", "1-2", "1-1-1" 等
func (c *Coordinator) FindSubtaskByIndex(index string) *AiTask {
	if c.rootTask == nil {
		return nil
	}

	// 使用 DFS 遍历查找匹配 index 的任务
	taskLink := DFSOrderAiTask(c.rootTask)
	for i := 0; i < taskLink.Len(); i++ {
		task, ok := taskLink.Get(i)
		if !ok {
			continue
		}
		if task.Index == index {
			return task
		}
	}
	return nil
}

func (c *Coordinator) AppendTask(t *AiTask) {
	defer func() {
		t.GenerateIndex()
	}()
	r := c.runtime
	task, ok := r.TaskLink.Get(r.currentIndex())
	if !ok {
		log.Warnf("coordinator: append task failed, current task not found")
		return
	}
	if parent := task.ParentTask; parent != nil {
		parent.Subtasks = append(parent.Subtasks, t)
	}
}

// HandleSkipSubtaskInPlan 处理跳过子任务的同步事件
// 输入参数:
//   - subtask_index: 子任务的索引，如 "1-1", "1-2" （当 current task 为false的时候必须）
//   - skip_current_task: 跳过当前任务（可选）
//   - reason: 用户跳过该任务的理由（可选）
//
// 注意：此函数不会返回错误导致整体中断，而是通过同步响应返回失败信息
func (c *Coordinator) HandleSkipSubtaskInPlan(event *ypb.AIInputEvent) error {
	// 容错处理：捕获可能的 panic
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("HandleSkipSubtaskInPlan panic recovered: %v", r)
			c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skip_subtask_in_plan", map[string]any{
				"success": false,
				"error":   utils.InterfaceToString(r),
			}, event.SyncID)
		}
	}()

	// 辅助函数：发送失败响应（不返回错误）
	sendFailResponse := func(errMsg string) {
		c.EmitError(errMsg)
		c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skip_subtask_in_plan", map[string]any{
			"success": false,
			"error":   errMsg,
		}, event.SyncID)
	}

	// 解析参数
	var params map[string]interface{}
	if event.SyncJsonInput != "" {
		if err := jsonextractor.ExtractStructuredJSON(event.SyncJsonInput, jsonextractor.WithObjectCallback(func(data map[string]any) {
			params = data
		})); err != nil {
			sendFailResponse("parse skip_subtask_in_plan params failed: " + err.Error())
			return nil
		}
	}

	if params == nil {
		sendFailResponse("skip_subtask_in_plan params is nil")
		return nil
	}

	subtaskIndex := utils.InterfaceToString(params["subtask_index"])
	if subtaskIndex == "" {
		if utils.InterfaceToBoolean(params["skip_current_task"]) {
			// 跳过当前任务
			currentTask, ok := c.runtime.TaskLink.Get(c.runtime.currentIndex())
			if !ok || currentTask == nil {
				sendFailResponse("no current task found to skip")
				return nil
			}
			subtaskIndex = currentTask.Index
		} else {
			sendFailResponse("subtask_index is required for skip_subtask_in_plan")
			return nil
		}
	}

	// 获取用户理由（可选）
	userReason := utils.InterfaceToString(params["reason"])

	// 查找子任务
	task := c.FindSubtaskByIndex(subtaskIndex)
	if task == nil {
		sendFailResponse("subtask not found by index: " + subtaskIndex)
		return nil
	}

	// 取消任务并设置为 Skipped 状态（区别于 Aborted，Skipped 专门表示用户主动跳过）
	task.SetStatus(aicommon.AITaskState_Skipped)
	task.Cancel()

	// 构建 timeline 消息
	baseMessage := "用户主动跳过了当前子任务，可能是用户觉得当前任务意义不重要，或者当前信息已经足够作出决定了，请你不要质疑，直接开始执行下一个子任务"
	timelineMessage := baseMessage
	if userReason != "" {
		timelineMessage = baseMessage + "。用户给出的理由: " + userReason
	}

	c.Timeline.PushText(c.AcquireId(), "[user-skiped-subtask] 任务 %s (%s) 被用户主动跳过: %s", task.Index, task.Name, timelineMessage)

	c.EmitInfo("subtask %s (%s) skipped by user", task.Index, task.Name)

	// 发送同步响应
	c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skip_subtask_in_plan", map[string]any{
		"success":       true,
		"subtask_index": subtaskIndex,
		"subtask_name":  task.Name,
		"reason":        userReason,
		"message":       timelineMessage,
	}, event.SyncID)

	return nil
}

// HandleRedoSubtaskInPlan 处理重做子任务的同步事件
// 用户可以中断当前子任务，添加额外信息到 timeline，然后重新执行该任务
// 输入参数:
//   - subtask_index: 子任务的索引，如 "1-1", "1-2"（必需）
//   - user_message: 用户提供的额外信息，用于辅助 AI 更好地执行任务（必需）
//
// 注意：此函数不会返回错误导致整体中断，而是通过同步响应返回失败信息
func (c *Coordinator) HandleRedoSubtaskInPlan(event *ypb.AIInputEvent) error {
	// 容错处理：捕获可能的 panic
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("HandleRedoSubtaskInPlan panic recovered: %v", r)
			c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "redo_subtask_in_plan", map[string]any{
				"success": false,
				"error":   utils.InterfaceToString(r),
			}, event.SyncID)
		}
	}()

	// 辅助函数：发送失败响应（不返回错误）
	sendFailResponse := func(errMsg string) {
		c.EmitError(errMsg)
		c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "redo_subtask_in_plan", map[string]any{
			"success": false,
			"error":   errMsg,
		}, event.SyncID)
	}

	// 解析参数
	var params map[string]interface{}
	if event.SyncJsonInput != "" {
		if err := jsonextractor.ExtractStructuredJSON(event.SyncJsonInput, jsonextractor.WithObjectCallback(func(data map[string]any) {
			params = data
		})); err != nil {
			sendFailResponse("parse redo_subtask_in_plan params failed: " + err.Error())
			return nil
		}
	}

	if params == nil {
		sendFailResponse("redo_subtask_in_plan params is nil")
		return nil
	}

	subtaskIndex := utils.InterfaceToString(params["subtask_index"])
	if subtaskIndex == "" {
		sendFailResponse("subtask_index is required for redo_subtask_in_plan")
		return nil
	}

	// 用户消息是必须的
	userMessage := utils.InterfaceToString(params["user_message"])
	if userMessage == "" {
		sendFailResponse("user_message is required for redo_subtask_in_plan")
		return nil
	}

	// 查找子任务
	task := c.FindSubtaskByIndex(subtaskIndex)
	if task == nil {
		sendFailResponse("subtask not found by index: " + subtaskIndex)
		return nil
	}

	if task.GetStatus() != aicommon.AITaskState_Completed {
		c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "redo_subtask_in_plan", map[string]any{
			"success":       false,
			"subtask_index": subtaskIndex,
			"subtask_name":  task.Name,
			"user_message":  userMessage,
			"message":       "only completed subtasks can be redone",
		}, event.SyncID)
		return nil
	}

	// 构建 timeline 消息 - 包含用户的额外信息
	timelineMessage := strings.Join([]string{
		"用户请求重新执行当前子任务，并提供了以下额外信息来辅助任务执行:",
		"",
		"<用户补充信息>",
		userMessage,
		"</用户补充信息>",
		"",
		"请 AI 认真解读用户提供的信息，理解用户的真实意图，并据此调整任务执行策略，确保更好地满足用户需求。",
	}, "\n")

	// 先添加 timeline 消息
	c.Timeline.PushText(c.AcquireId(), "[user-redo-subtask] 任务 %s (%s) 被用户请求重新执行:\n%s", task.Index, task.Name, timelineMessage)

	c.EmitInfo("subtask %s (%s) will be redone with user message", task.Index, task.Name)

	task.SetContext(c.GetContext())
	c.AppendTask(task)

	// 发送同步响应
	c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "redo_subtask_in_plan", map[string]any{
		"success":       true,
		"subtask_index": subtaskIndex,
		"subtask_name":  task.Name,
		"user_message":  userMessage,
		"message":       timelineMessage,
	}, event.SyncID)

	return nil
}
