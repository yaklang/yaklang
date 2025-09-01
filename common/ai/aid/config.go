package aid

import (
	"bytes"
	"context"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aispec"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var _ aicommon.AICaller = &Coordinator{}
var _ aicommon.AICaller = &planRequest{}
var _ aicommon.AICaller = &AiTask{}
var _ aicommon.AICallerConfigIf = &Config{}

type Config struct {
	*aicommon.Emitter
	*aicommon.BaseCheckpointableStorage

	ctx    context.Context
	cancel context.CancelFunc

	startInputEventOnce sync.Once
	wg                  sync.WaitGroup // sub task wait group

	idSequence  int64
	idGenerator func() int64

	m  *sync.Mutex
	id string

	startHotpatchOnce  sync.Once
	hotpatchOptionChan *chanx.UnlimitedChan[Option]

	eventInputChan chan *InputEvent
	epm            *aicommon.EndpointManager

	// plan mocker
	planMocker               func(*Config) *PlanResponse
	allowPlanUserInteract    bool  // allow user to interact before planning.
	planUserInteractMaxCount int64 // max user interact count before planning, default is 3

	// origin ai callback
	originalAICallback    aicommon.AICallbackType //!!!! provide branch tasks
	coordinatorAICallback aicommon.AICallbackType // need to think
	planAICallback        aicommon.AICallbackType
	taskAICallback        aicommon.AICallbackType // no need to think, low level

	// asyncGuardian can auto collect event handler data
	guardian     *aicommon.AsyncGuardian
	eventHandler func(e *schema.AiOutputEvent)

	saveEvent bool

	// tool manager
	aiToolManager       *buildinaitools.AiToolManager
	aiToolManagerOption []buildinaitools.ToolManagerOption

	// memory
	persistentMemory          []string
	memory                    *Memory
	timelineRecordLimit       int
	timelineContentSizeLimit  int
	timelineTotalContentLimit int
	keywords                  []string // task keywords, maybe tools name ,help ai to plan

	debugPrompt bool
	debugEvent  bool

	// AI can ask human for help?
	allowRequireForUserInteract bool

	// do not use it directly, use doAgree() instead
	agreePolicy         aicommon.AgreePolicyType
	agreeInterval       time.Duration
	agreeAIScore        float64
	agreeRiskCtrl       *riskControl
	agreeManualCallback func(context.Context, *Config) (aitool.InvokeParams, error)

	//review suggestion

	// sync
	syncMutex *sync.RWMutex
	syncMap   map[string]func() any

	inputConsumption  *int64
	outputConsumption *int64

	aiCallTokenLimit       int64
	aiAutoRetry            int64
	aiTransactionAutoRetry int64

	resultHandler          func(*Config)
	extendedActionCallback map[string]func(config *Config, action *aicommon.Action)

	promptHook     func(string) string
	generateReport bool

	forgeName string // if coordinator is create from a forge, this is the forge name

	maxTaskContinue int64

	aiTaskRuntime *runtime

	disableOutputEventType []string
}

func (c *Config) GetTimelineRecordLimit() int64 {
	return int64(c.timelineRecordLimit)
}

func (c *Config) GetTimelineContentSizeLimit() int64 {
	return int64(c.timelineContentSizeLimit)
}

func (c *Config) GetRuntimeId() string {
	return c.id
}

func (c *Config) Feed(endpointId string, params aitool.InvokeParams) {
	if c.epm != nil {
		c.epm.Feed(endpointId, params)
	}
}

func (c *Config) CallAfterReview(seq int64, reviewQuestion string, userInput aitool.InvokeParams) {
	c.memory.PushUserInteraction(aicommon.UserInteractionStage_Review, seq, reviewQuestion, string(utils.Jsonify(userInput)))
}

func (c *Config) CallAfterInteractiveEventReleased(eventID string, invoke aitool.InvokeParams) {
	c.memory.StoreInteractiveUserInput(eventID, invoke)
}

func (c *Config) CallAIResponseOutputFinishedCallback(s string) {
	c.ProcessExtendedActionCallback(s)
}

func (c *Config) NewAIResponse() *aicommon.AIResponse {
	return aicommon.NewAIResponse(c)
}

func (c *Config) GetAITransactionAutoRetryCount() int64 {
	return c.aiTransactionAutoRetry
}

func (c *Config) GetContext() context.Context {
	return c.ctx
}

func (c *Config) GetEmitter() *aicommon.Emitter {
	return c.Emitter
}

func (c *Config) GetEndpointManager() *aicommon.EndpointManager {
	return c.epm
}

func (c *Config) HandleSearch(query string, items *omap.OrderedMap[string, []string]) ([]*searchtools.KeywordSearchResult, error) {
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
		"Memory":          c.memory,
		"UserRequirement": query,
		"ToolsLists":      toolsLists,
	})
	if err != nil {
		return nil, err
	}
	var callResults []*searchtools.KeywordSearchResult

	err = c.callAiTransaction(prompt, c.CallAI, func(response *aicommon.AIResponse) error {
		action, err := aicommon.ExtractActionFromStream(response.GetUnboundStreamReader(false), "keyword_search")
		if err != nil {
			log.Errorf("extract aitool-keyword-search action failed: %v", err)
			return utils.Errorf("extract aitool-keyword-search failed: %v", err)
		}
		tools := action.GetInvokeParamsArray("matches")
		if len(tools) > 0 {
			for _, toolInfo := range tools {
				callResults = append(callResults, &searchtools.KeywordSearchResult{
					Tool:            toolInfo.GetString("tool"),
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

func (c *Config) InitToolManager() error {
	if c.aiToolManager == nil {
		c.aiToolManager = buildinaitools.NewToolManager(append(c.aiToolManagerOption, buildinaitools.WithSearcher(func(query string, searchList []*aitool.Tool) ([]*aitool.Tool, error) {
			keywords := omap.NewOrderedMap[string, []string](nil)
			toolMap := map[string]*aitool.Tool{}
			for _, tool := range searchList {
				keywords.Set(tool.GetName(), tool.GetKeywords())
				toolMap[tool.GetName()] = tool
			}
			searchResults, err := c.HandleSearch(query, keywords)
			if err != nil {
				return nil, err
			}
			tools := []*aitool.Tool{}
			for _, result := range searchResults {
				tools = append(tools, toolMap[result.Tool])
			}
			return tools, nil
		}))...)
	}
	return nil
}

func (c *Config) MakeInvokeParams() aitool.InvokeParams {
	p := make(aitool.InvokeParams)
	p["runtime_id"] = c.id
	return p
}

func (c *Config) Add(delta int) {
	c.wg.Add(delta)
	return
}

func (c *Config) Done() {
	c.wg.Done()
	return
}

func (c *Config) Wait() {
	c.wg.Wait()
	log.Info("aid.Config 's wg is waiting done, all tasks finished, start to check stream...")
	c.WaitForStream()
	log.Info("aid.Config 's all stream waitgroup is done, all tasks finished")
	return
}

func (c *Config) AcquireId() int64 {
	return c.idGenerator()
}

func (c *Config) GetSequenceStart() int64 {
	return c.idSequence
}

func (c *Config) getCurrentTaskPlan() *AiTask {
	return c.aiTaskRuntime.RootTask
}

func (c *Config) CallAI(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
	for _, cb := range []aicommon.AICallbackType{
		c.taskAICallback,
		c.coordinatorAICallback,
		c.planAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(c, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

func (c *Config) setAgreePolicy(policy aicommon.AgreePolicyType) {
	c.agreePolicy = policy
}

func (c *Config) CallAIResponseConsumptionCallback(current int) {
	atomic.AddInt64(c.outputConsumption, int64(current))
}

func (c *Config) inputConsumptionCallback(current int) {
	atomic.AddInt64(c.inputConsumption, int64(current))
}

func (c *Config) GetInputConsumption() int64 {
	return atomic.LoadInt64(c.inputConsumption)
}

func (c *Config) GetMemory() *Memory {
	return c.memory
}

func (c *Config) GetOutputConsumption() int64 {
	return atomic.LoadInt64(c.outputConsumption)
}

func (c *Config) IsCtxDone() bool {
	select {
	case <-c.ctx.Done():
		return true
	default:
		return false
	}
}

func (c *Config) SetSyncCallback(i SyncType, callback func() any) {
	c.syncMutex.Lock()
	defer c.syncMutex.Unlock()
	c.syncMap[string(i)] = callback
}

func (c *Config) ProcessExtendedActionCallback(resp string) {
	actions := aicommon.ExtractAllAction(resp)
	for _, action := range actions {
		if cb, ok := c.extendedActionCallback[action.Name()]; ok {
			cb(c, action)
		}
	}
}

func (c *Config) ReleaseInteractiveEvent(eventID string, invoke aitool.InvokeParams) {
	c.EmitInteractiveRelease(eventID, invoke)
	c.CallAfterInteractiveEventReleased(eventID, invoke)
}

func initDefaultTools(c *Config) error { // set config default tools
	if err := WithTools(buildinaitools.GetBasicBuildInTools()...)(c); err != nil {
		return utils.Wrapf(err, "get basic build-in tools fail")
	}

	return nil
}

func initDefaultAICallback(c *Config) error { // set config default tools
	defaultAICallback := aicommon.AIChatToAICallbackType(ai.Chat)
	if defaultAICallback == nil {
		return nil
	}
	if err := WithAICallback(defaultAICallback)(c); err != nil {
		return err
	}
	return nil
}

func (c *Config) loadToolsViaOptions() error {
	if c.memory != nil {
		memoryTools, err := c.memory.CreateBasicMemoryTools()
		if err != nil {
			return utils.Errorf("create memory tools: %v", err)
		}
		if err := WithTools(memoryTools...)(c); err != nil {
			log.Errorf("load memory tools: %v", err)
			return err
		}
	}
	if c.allowRequireForUserInteract {
		userPromptTool, err := c.CreateRequireUserInteract()
		if err != nil {
			log.Errorf("create user prompt tool: %v", err)
			return err
		}
		if err := WithTools(userPromptTool)(c); err != nil {
			log.Errorf("load require for user prompt tools: %v", err)
			return err
		}
	}
	return nil
}

func NewConfig(ctx context.Context) *Config {
	offset := rand.Int64N(3000)
	id := uuid.New().String()
	return newConfigEx(ctx, id, offset)
}

func newConfigEx(ctx context.Context, id string, offsetSeq int64) *Config {
	var idGenerator = new(int64)
	log.Debugf("coordinator with %v offset: %v", id, offsetSeq)

	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)

	c := &Config{
		ctx: ctx, cancel: cancel,
		idSequence: atomic.AddInt64(idGenerator, offsetSeq),
		idGenerator: func() int64 {
			return atomic.AddInt64(idGenerator, 1)
		},
		agreePolicy:                 aicommon.AgreePolicyManual,
		agreeAIScore:                0.5,
		agreeRiskCtrl:               new(riskControl),
		agreeInterval:               10 * time.Second,
		m:                           new(sync.Mutex),
		id:                          id,
		epm:                         aicommon.NewEndpointManagerContext(ctx),
		memory:                      nil, // default mem cannot create in config
		guardian:                    aicommon.NewAsyncGuardian(ctx, id),
		syncMutex:                   new(sync.RWMutex),
		syncMap:                     make(map[string]func() any),
		inputConsumption:            new(int64),
		outputConsumption:           new(int64),
		aiCallTokenLimit:            int64(1000 * 30),
		aiAutoRetry:                 5,
		aiTransactionAutoRetry:      5,
		allowRequireForUserInteract: true,
		timelineRecordLimit:         10,
		timelineContentSizeLimit:    30 * 1024,
		aiToolManagerOption:         make([]buildinaitools.ToolManagerOption, 0),
		planUserInteractMaxCount:    3,
		maxTaskContinue:             10,
	}
	c.epm.SetConfig(c)
	if err := initDefaultTools(c); err != nil {
		log.Errorf("init default tools: %v", err)
	}

	if err := initDefaultAICallback(c); err != nil {
		log.Errorf("init default ai callback: %v", err)
	}

	c.Emitter = aicommon.NewEmitter(c.id, func(e *schema.AiOutputEvent) error {
		c.emitBaseHandler(e)
		return nil
	})
	c.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(c.id, consts.GetGormProjectDatabase())

	return c
}

type Option func(config *Config) error

func WithCoordinatorId(id string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.id = id
		return nil
	}
}

func WithSequence(seq int64) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		var idGenerator = new(int64)
		config.idSequence = atomic.AddInt64(idGenerator, seq)
		config.idGenerator = func() int64 {
			return atomic.AddInt64(idGenerator, 1)
		}
		return nil
	}
}

func WithExtendedActionCallback(name string, cb func(config *Config, action *aicommon.Action)) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if config.extendedActionCallback == nil {
			config.extendedActionCallback = make(map[string]func(config *Config, action *aicommon.Action))
		}
		config.extendedActionCallback[name] = cb
		return nil
	}
}

func WithDisallowRequireForUserPrompt() Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.allowRequireForUserInteract = false
		return nil
	}
}

func WithManualAssistantCallback(cb func(context.Context, *Config) (aitool.InvokeParams, error)) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.agreeManualCallback = cb
		return nil
	}
}

func WithAgreeYOLO(i ...bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if len(i) > 0 {
			if i[0] {
				config.setAgreePolicy(aicommon.AgreePolicyYOLO)
			} else {
				config.setAgreePolicy(aicommon.AgreePolicyManual)
			}
		} else {
			config.setAgreePolicy(aicommon.AgreePolicyYOLO)
		}
		return nil
	}
}

func WithAgreePolicy(policy aicommon.AgreePolicyType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(policy)
		return nil
	}
}

func WithAIAgree() Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(aicommon.AgreePolicyAI)
		return nil
	}
}

func WithAgreeManual(cb ...func(context.Context, *Config) (aitool.InvokeParams, error)) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(aicommon.AgreePolicyManual)
		if len(cb) > 0 {
			config.agreeManualCallback = cb[0]
		}

		return nil
	}
}

func WithToolManagerOptions(i ...buildinaitools.ToolManagerOption) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.aiToolManagerOption = append(config.aiToolManagerOption, i...)
		return nil
	}
}

func WithAgreeAuto(interval time.Duration) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(aicommon.AgreePolicyAIAuto)
		config.agreeInterval = interval
		return nil
	}
}

func WithAllowRequireForUserInteract(opts ...bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if len(opts) > 0 {
			config.allowRequireForUserInteract = opts[0]
			return nil
		}
		config.allowRequireForUserInteract = true
		return nil
	}
}

func WithAICallback(cb aicommon.AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		warpedCb := config.wrapper(cb)
		config.originalAICallback = cb
		config.coordinatorAICallback = warpedCb
		config.taskAICallback = warpedCb
		config.planAICallback = warpedCb
		return nil
	}
}

func WithToolManager(manager *buildinaitools.AiToolManager) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.aiToolManager = manager
		return nil
	}
}

func WithMemory(m *Memory) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		m.ClearRuntimeConfig()
		config.memory = m
		return nil
	}
}

func WithTaskAICallback(cb aicommon.AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.taskAICallback = config.wrapper(cb)
		return nil
	}
}

func WithCoordinatorAICallback(cb aicommon.AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.coordinatorAICallback = config.wrapper(cb)
		return nil
	}
}

func WithPlanAICallback(cb aicommon.AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.planAICallback = config.wrapper(cb)
		return nil
	}
}

func WithSystemFileOperator() Option {
	return func(config *Config) error {
		tools, err := fstools.CreateSystemFSTools()
		if err != nil {
			return utils.Errorf("create system fs tools: %v", err)
		}
		return WithTools(tools...)(config)
	}
}

func WithJarOperator() Option {
	return func(config *Config) error {
		tools, err := fstools.CreateJarOperator()
		if err != nil {
			return utils.Errorf("create jar operator tools: %v", err)
		}
		return WithTools(tools...)(config)
	}
}

func WithOmniSearchTool() Option {
	return func(config *Config) error {
		tools, err := searchtools.CreateOmniSearchTools()
		if err != nil {
			return utils.Errorf("create omnisearch tools: %v", err)
		}
		return WithTools(tools...)(config)
	}
}

func WithAiToolsSearchTool() Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if config.aiToolManagerOption == nil {
			config.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		config.aiToolManagerOption = append(config.aiToolManagerOption,
			buildinaitools.WithSearchEnabled(true))
		return nil
	}
}

func WithAiForgeSearchTool() Option {
	return func(config *Config) error {
		forgeSearchTools, err := searchtools.CreateAISearchTools(
			func(query string, searchList []*schema.AIForge) ([]*schema.AIForge, error) {
				keywords := omap.NewOrderedMap[string, []string](nil)
				forgeMap := map[string]*schema.AIForge{}
				for _, forge := range searchList {
					keywords.Set(forge.GetName(), forge.GetKeywords())
					forgeMap[forge.GetName()] = forge
				}
				searchResults, err := config.HandleSearch(query, keywords)
				if err != nil {
					return nil, err
				}
				forges := []*schema.AIForge{}
				for _, result := range searchResults {
					forges = append(forges, forgeMap[result.Tool])
				}
				return forges, nil
			},
			func() []*schema.AIForge {
				forgeList, err := yakit.GetAllAIForge(consts.GetGormProfileDatabase())
				if err != nil {
					log.Errorf("yakit.GetAllAIForge: %v", err)
					return nil
				}
				return forgeList
			}, searchtools.SearchForgeName,
		)
		if err != nil {
			return utils.Errorf("create ai forge search tools fail: %v", err)
		}
		return WithTools(forgeSearchTools...)(config)
	}
}

func WithEnableToolsName(toolsName ...string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if config.aiToolManagerOption == nil {
			config.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		config.aiToolManagerOption = append(config.aiToolManagerOption, buildinaitools.WithEnabledTools(toolsName))
		return nil
	}
}

func WithDisableToolsName(toolsName ...string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if config.aiToolManagerOption == nil {
			config.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		config.aiToolManagerOption = append(config.aiToolManagerOption, buildinaitools.WithDisableTools(toolsName))
		return nil
	}
}

func WithTool(tool *aitool.Tool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if config.aiToolManagerOption == nil {
			config.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		config.aiToolManagerOption = append(config.aiToolManagerOption, buildinaitools.WithExtendTools([]*aitool.Tool{tool}, true))
		return nil
	}
}

func WithTools(tool ...*aitool.Tool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if config.aiToolManagerOption == nil {
			config.aiToolManagerOption = make([]buildinaitools.ToolManagerOption, 0)
		}
		config.aiToolManagerOption = append(config.aiToolManagerOption, buildinaitools.WithExtendTools(tool, true))
		return nil
	}
}

func WithDebugPrompt(i ...bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if len(i) > 0 {
			config.debugPrompt = i[0]
			return nil
		}
		config.debugPrompt = true
		return nil
	}
}

func WithEventHandler(h func(e *schema.AiOutputEvent)) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.eventHandler = h
		return nil
	}
}

func WithEventInputChan(ch chan *InputEvent) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		config.eventInputChan = ch
		return nil
	}
}

func WithHotpatchOptionChanFactory(handle func() *chanx.UnlimitedChan[Option]) Option {
	return func(config *Config) error {
		return WithHotpatchOptionChan(handle())(config)
	}
}

func WithHotpatchOptionChan(ch *chanx.UnlimitedChan[Option]) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.hotpatchOptionChan = ch
		return nil
	}
}

func WithDebug(i ...bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if len(i) > 0 {
			config.debugPrompt = i[0]
			config.debugEvent = i[0]
			return nil
		}
		config.debugPrompt = true
		config.debugEvent = true
		return nil
	}
}

func WithResultHandler(h func(*Config)) Option {
	return func(config *Config) error {
		config.resultHandler = h
		return nil
	}
}

func WithAppendPersistentMemory(i ...string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.persistentMemory = append(config.persistentMemory, i...)
		return nil
	}
}

func WithToolKeywords(i ...string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.keywords = append(config.keywords, i...)
		return nil
	}
}

func WithTimeLineLimit(i int) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.timelineRecordLimit = i
		return nil
	}
}

func WithTimelineContentLimit(i int) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.timelineContentSizeLimit = i
		return nil
	}
}

func WithPlanMocker(i func(*Config) *PlanResponse) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.planMocker = i
		return nil
	}
}

func WithForgeParams(i any) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		var buf bytes.Buffer
		nonce := utils.RandStringBytes(8)
		buf.WriteString("<user_input_" + nonce + ">\n")
		buf.WriteString(aispec.ShrinkAndSafeToFile(i))
		buf.WriteString("\n</user_input_" + nonce + ">\n")
		// log.Infof("user nonce params: \n%v", buf.String())
		config.memory.PushPersistentData(buf.String())
		return nil
	}
}

func WithDisableToolUse(i ...bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if config == nil {
			log.Error("BUG: config cannot be empty in aid.Config Option")
			return nil
		}

		if config.memory == nil {
			config.memory = GetDefaultMemory()
		}

		if len(i) <= 0 {
			config.memory.DisableTools = true
		} else {
			config.memory.DisableTools = i[0]
		}
		return nil
	}
}

func WithAIAutoRetry(t int) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		config.aiAutoRetry = int64(t)
		return nil
	}
}

func WithAITransactionRetry(t int) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if t > 0 {
			config.aiTransactionAutoRetry = int64(t)
		}
		return nil
	}
}

func WithRiskControlForgeName(forgeName string, callbackType aicommon.AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		config.agreeRiskCtrl.buildinForgeName = forgeName
		config.agreeRiskCtrl.buildinAICallback = callbackType
		return nil
	}
}

func WithGuardianEventTrigger(eventTrigger schema.EventType, callback aicommon.GuardianEventTrigger) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if config.guardian == nil {
			return utils.Error("BUG: guardian cannot be empty (ASYNC Guardian)")
		}
		return config.guardian.RegisterEventTrigger(eventTrigger, callback)
	}
}

func WithGuardianMirrorStreamMirror(streamName string, callback aicommon.GuardianMirrorStreamTrigger) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if config.guardian == nil {
			return utils.Error("BUG: guardian cannot be empty (ASYNC Guardian)")
		}
		return config.guardian.RegisterMirrorStreamTrigger(streamName, callback)
	}
}

func WithPromptHook(c func(string) string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		config.promptHook = c
		return nil
	}
}

func WithGenerateReport(b bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		config.generateReport = b
		return nil
	}
}

func WithForgeName(forgeName string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		config.forgeName = forgeName
		return nil
	}
}

func WithTaskAnalysis(b bool) Option {
	return func(config *Config) error {
		return WithGuardianEventTrigger(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, func(event *schema.AiOutputEvent, emitter aicommon.GuardianEmitter, caller aicommon.AICaller) {
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

				action, err := ExecuteAIForge(config.ctx, "task-analyst", param, WithAICallback(config.originalAICallback))
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

		})(config)
	}
}

func WithMaxTaskContinue(i int64) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if i <= 0 {
			i = 10
		}
		config.maxTaskContinue = i
		return nil
	}
}

func WithQwenNoThink() Option {
	return WithPromptHook(func(origin string) string {
		return origin + "/nothink"
	})
}

func WithAllowPlanUserInteract(i ...bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if len(i) > 0 {
			config.allowPlanUserInteract = i[0]
			return nil
		}
		config.allowPlanUserInteract = true
		return nil
	}
}

func WithPlanUserInteractMaxCount(i int64) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if i <= 0 {
			i = 3
		}
		config.planUserInteractMaxCount = i
		return nil
	}
}

func WithDisableOutputEvent(typeString ...string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if config.disableOutputEventType == nil {
			config.disableOutputEventType = make([]string, 0)
		}
		config.disableOutputEventType = append(config.disableOutputEventType, typeString...)
		return nil
	}
}

func WithSaveEvent(b bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.saveEvent = b
		return nil
	}
}
