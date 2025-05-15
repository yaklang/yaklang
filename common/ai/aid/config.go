package aid

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/searchtools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type AICaller interface {
	callAI(*AIRequest) (*AIResponse, error)
}

var _ AICaller = &Coordinator{}
var _ AICaller = &planRequest{}
var _ AICaller = &aiTask{}

type AgreePolicyType string

const (
	AgreePolicyYOLO AgreePolicyType = "yolo"
	// auto: auto agree, should with interval at least 10 seconds
	AgreePolicyAuto AgreePolicyType = "auto"
	// manual: block until user agree
	AgreePolicyManual AgreePolicyType = "manual"
	// ai: use ai to agree, is ai is not agree, will use manual
	AgreePolicyAI AgreePolicyType = "ai"
	// ai-auto: use ai to agree, if ai is not agree, will use auto in auto interval
	AgreePolicyAIAuto AgreePolicyType = "ai-auto"
)

type Config struct {
	ctx    context.Context
	cancel context.CancelFunc

	startInputEventOnce sync.Once

	idSequence  int64
	idGenerator func() int64

	m  *sync.Mutex
	id string

	eventInputChan chan *InputEvent
	epm            *endpointManager

	// plan mocker
	planMocker func(*Config) *PlanResponse

	// need to think
	coordinatorAICallback AICallbackType
	planAICallback        AICallbackType

	// no need to think, low level
	taskAICallback AICallbackType
	toolAICallback SimpleAiCallbackType

	tools        []*aitool.Tool

	// asyncGuardian can auto collect event handler data
	guardian     *asyncGuardian
	eventHandler func(e *Event)

	enableToolSearch bool
	// tool manager
	aiToolManager *buildinaitools.AiToolManager

	// task ai resp callback
	taskAIRespCallback func(string, *Config)

	// memory
	persistentMemory          []string
	memory                    *Memory
	timelineLimit             int
	timelineContentLimit      int
	timelineTotalContentLimit int

	// stream waitgroup
	streamWaitGroup *sync.WaitGroup

	debugPrompt bool
	debugEvent  bool

	// AI can ask human for help?
	allowRequireForUserInteract bool

	// do not use it directly, use doAgree() instead
	agreePolicy         AgreePolicyType
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
	extendedActionCallback map[string]func(config *Config, action *Action)
}

func (c *Config) MakeInvokeParams() aitool.InvokeParams {
	p := make(aitool.InvokeParams)
	p["runtime_id"] = c.id
	return p
}

func (c *Config) AcquireId() int64 {
	return c.idGenerator()
}

func (c *Config) GetSequenceStart() int64 {
	return c.idSequence
}

func (c *Config) CallAI(request *AIRequest) (*AIResponse, error) {
	return c.callAI(request)
}

func (c *Config) callAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
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

func (c *Config) setAgreePolicy(policy AgreePolicyType) {
	c.agreePolicy = policy
}

func (c *Config) outputConsumptionCallback(current int) {
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

func (c *Config) SetSyncCallback(i SyncType, callback func() any) {
	c.syncMutex.Lock()
	defer c.syncMutex.Unlock()
	c.syncMap[string(i)] = callback
}

func (c *Config) emit(e *Event) {
	c.m.Lock()
	defer c.m.Unlock()

	if c.guardian != nil {
		c.guardian.feed(e)
	}

	if c.eventHandler == nil {
		if e.IsStream {
			if c.debugEvent {
				fmt.Print(string(e.StreamDelta))
			}
			return
		}

		if e.Type == EVENT_TYPE_CONSUMPTION {
			if c.debugEvent {
				log.Info(e.String())
			}
			return
		}
		if c.debugEvent {
			log.Info(e.String())
		} else {
			log.Info(utils.ShrinkString(e.String(), 200))
		}
		return
	}
	c.eventHandler(e)
}

func (c *Config) ProcessExtendedActionCallback(resp string) {
	actions := ExtractAllAction(resp)
	for _, action := range actions {
		if cb, ok := c.extendedActionCallback[action.Name()]; ok {
			cb(c, action)
		}
	}
}

func (c *Config) ReleaseInteractiveEvent(eventID string, invoke aitool.InvokeParams) {
	c.emitInteractiveRelease(eventID, invoke)
	c.memory.StoreInteractiveUserInput(eventID, invoke)
}

func initDefaultTools(c *Config) error { // set config default tools
	if err := WithTools(buildinaitools.GetBasicBuildInTools()...)(c); err != nil {
		return utils.Wrapf(err, "get basic build-in tools fail")
	}

	memoryTools, err := c.memory.CreateBasicMemoryTools()
	if err != nil {
		return utils.Errorf("create memory tools: %v", err)
	}
	if err := WithTools(memoryTools...)(c); err != nil {
		return err
	}
	return nil
}

func (c *Config) loadToolsViaOptions() error {
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

func newConfig(ctx context.Context) *Config {
	offset := rand.Int64N(3000)
	id := uuid.New().String()
	return newConfigEx(ctx, id, offset)
}

func newConfigEx(ctx context.Context, id string, offsetSeq int64) *Config {
	m := GetDefaultMemory()
	var idGenerator = new(int64)
	log.Infof("coordinator with %v offset: %v", id, offsetSeq)

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
		agreePolicy:                 AgreePolicyManual,
		agreeAIScore:                0.5,
		agreeRiskCtrl:               new(riskControl),
		agreeInterval:               10 * time.Second,
		m:                           new(sync.Mutex),
		id:                          id,
		epm:                         newEndpointManagerContext(ctx),
		streamWaitGroup:             new(sync.WaitGroup),
		memory:                      m,
		guardian:                    newAysncGuardian(ctx, id),
		syncMutex:                   new(sync.RWMutex),
		syncMap:                     make(map[string]func() any),
		inputConsumption:            new(int64),
		outputConsumption:           new(int64),
		aiCallTokenLimit:            int64(1000 * 30),
		aiAutoRetry:                 5,
		aiTransactionAutoRetry:      5,
		allowRequireForUserInteract: true,
		timelineLimit:               10,
		timelineContentLimit:        30 * 1024,
	}
	c.epm.config = c // review
	if err := initDefaultTools(c); err != nil {
		log.Errorf("init default tools: %v", err)
	}
	return c
}

type Option func(config *Config) error

func WithRuntimeID(id string) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.id = id
		return nil
	}
}

func WithOffsetSeq(offset int64) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.idSequence = offset
		return nil
	}
}

func WithTool(tool *aitool.Tool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.tools = append(config.tools, tool)
		return nil
	}
}

func WithExtendedActionCallback(name string, cb func(config *Config, action *Action)) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		if config.extendedActionCallback == nil {
			config.extendedActionCallback = make(map[string]func(config *Config, action *Action))
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
				config.setAgreePolicy(AgreePolicyYOLO)
			} else {
				config.setAgreePolicy(AgreePolicyManual)
			}
		} else {
			config.setAgreePolicy(AgreePolicyYOLO)
		}
		return nil
	}
}

func WithAgreePolicy(policy AgreePolicyType) Option {
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
		config.setAgreePolicy(AgreePolicyAI)
		return nil
	}
}

func WithAgreeManual(cb ...func(context.Context, *Config) (aitool.InvokeParams, error)) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(AgreePolicyManual)
		if len(cb) > 0 {
			config.agreeManualCallback = cb[0]
		}

		return nil
	}
}

func WithAgreeAuto(interval time.Duration) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(AgreePolicyAIAuto)
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

func WithTools(tool ...*aitool.Tool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.tools = append(config.tools, tool...)
		return nil
	}
}

func WithAICallback(cb AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		warpedCb := config.wrapper(cb)
		config.toolAICallback = func(msg string) (io.Reader, error) {
			rsp, err := warpedCb(config, NewAIRequest(msg))
			if err != nil {
				return nil, err
			}
			return rsp.GetOutputStreamReader("tool", false, config), nil
		}
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
		config.memory = m
		return nil
	}
}

func WithTaskAICallback(cb AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.taskAICallback = config.wrapper(cb)
		return nil
	}
}

func WithCoordinatorAICallback(cb AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.coordinatorAICallback = config.wrapper(cb)
		return nil
	}
}

func WithPlanAICallback(cb AICallbackType) Option {
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
		config.enableToolSearch = true
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

func WithEventHandler(h func(e *Event)) Option {
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
		config.memory.StoreAppendPersistentInfo(i...)
		return nil
	}
}

func WithTimeLineLimit(i int) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.timelineLimit = i
		return nil
	}
}

func WithTimelineContentLimit(i int) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.timelineContentLimit = i
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
		config.memory.StoreAppendPersistentInfo(buf.String())

		// if cli parameter not nil , should init user data
		if params, ok := i.([]*ypb.ExecParamItem); ok {
			if len(params) > 0 {
				config.memory.StoreCliParameter(params)
			}
		}
		return nil
	}
}

func WithDisableToolUse(i ...bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

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

func WithRiskControlForgeName(forgeName string, callbackType AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		config.agreeRiskCtrl.buildinForgeName = forgeName
		config.agreeRiskCtrl.buildinAICallback = callbackType
		return nil
	}
}

func WithGuardianEventTrigger(eventTrigger EventType, callback GuardianEventTrigger) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if config.guardian == nil {
			return utils.Error("BUG: guardian cannot be empty (ASYNC Guardian)")
		}
		return config.guardian.registerEventTrigger(eventTrigger, callback)
	}
}

func WithGuardianMirrorStreamMirror(streamName string, callback GuardianMirrorStreamTrigger) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()

		if config.guardian == nil {
			return utils.Error("BUG: guardian cannot be empty (ASYNC Guardian)")
		}
		return config.guardian.registerMirrorEventTrigger(streamName, callback)
	}
}
