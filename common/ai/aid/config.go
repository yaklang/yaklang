package aid

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	tools          []*aitool.Tool
	eventHandler   func(e *Event)

	// tool manager
	aiToolManager buildinaitools.ToolManager

	// memory
	persistentMemory []string
	memory           *Memory
	timeLineLimit    int

	// stream waitgroup
	streamWaitGroup *sync.WaitGroup

	debugPrompt bool
	debugEvent  bool

	// do not use it directly, use doAgree() instead
	agreePolicy    AgreePolicyType
	agreeInterval  time.Duration
	agreeAIScore   float64
	agreeRiskCtrl  *riskControl
	agreeAssistant *AIAssistant

	//review suggestion

	// sync
	syncMutex *sync.RWMutex
	syncMap   map[string]func() any

	inputConsumption  *int64
	outputConsumption *int64

	aiCallTokenLimit int64

	resultHandler          func(*Config)
	extendedActionCallback map[string]func(config *Config, action *Action)
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

func (c *Config) wrapper(i AICallbackType) AICallbackType {
	return func(config *Config, request *AIRequest) (*AIResponse, error) {
		if c.debugPrompt {
			log.Infof(strings.Repeat("=", 20)+"AIRequest"+strings.Repeat("=", 20)+"\n%v\n", request.GetPrompt())
		}
		tokenSize := estimateTokens([]byte(request.GetPrompt()))
		if int64(tokenSize) > c.aiCallTokenLimit {
			go func() {
				c.emitJson(EVENT_TYPE_PRESSURE, "system", map[string]any{
					"message":          "token size is too large",
					"tokenSize":        tokenSize,
					"aiCallTokenLimit": c.aiCallTokenLimit,
				})
			}()
		}
		c.inputConsumptionCallback(tokenSize)
		resp, err := i(config, request)
		if c.debugPrompt {
			resp.Debug(true)
		}
		return resp, err
	}
}

func (c *Config) emit(e *Event) {
	c.m.Lock()
	defer c.m.Unlock()
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

func initDefaultTools(c *Config) error { // set config default tools
	if err := WithTools(buildinaitools.GetBasicBuildInTools()...)(c); err != nil {
		return utils.Wrapf(err, "get basic build-in tools fail")
	}
	memoryTools, err := c.memory.CreateMemoryTools()
	if err != nil {
		return utils.Errorf("create memory tools: %v", err)
	}
	if err := WithTools(memoryTools...)(c); err != nil {
		return err
	}
	return nil
}

func newConfig(ctx context.Context) *Config {
	id := uuid.New()
	m := GetDefaultMemory()

	var idGenerator = new(int64)
	atomic.AddInt64(idGenerator, rand.Int64N(2000))
	c := &Config{
		idGenerator: func() int64 {
			return atomic.AddInt64(idGenerator, 1)
		},
		agreePolicy:       AgreePolicyManual,
		agreeAIScore:      0.5,
		agreeRiskCtrl:     new(riskControl),
		agreeInterval:     10 * time.Second,
		m:                 new(sync.Mutex),
		id:                id.String(),
		epm:               newEndpointManagerContext(ctx),
		streamWaitGroup:   new(sync.WaitGroup),
		memory:            m,
		syncMutex:         new(sync.RWMutex),
		syncMap:           make(map[string]func() any),
		inputConsumption:  new(int64),
		outputConsumption: new(int64),
	}
	go func() {
		log.Infof("config %s started, start to handle receiving loop", c.id)
		logOnce := new(sync.Once)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		consumptionNotification := func() {
			if c.GetInputConsumption() > 0 || c.GetOutputConsumption() > 0 {
				c.emitJson(
					EVENT_TYPE_CONSUMPTION,
					"system",
					map[string]any{
						"input_consumption":  c.GetInputConsumption(),
						"output_consumption": c.GetOutputConsumption(),
					},
				)
			}
		}

		tickerCallback := func() {
			consumptionNotification()
		}
		for {
			if c.eventInputChan == nil {
				logOnce.Do(func() {
					log.Infof("event input chan is nil, will retry in 1 second")
				})
				select {
				case <-ticker.C:
					tickerCallback()
					continue
				case <-ctx.Done():
					return
				}
			}

			select {
			case event, ok := <-c.eventInputChan:
				if !ok {
					return
				}
				if event == nil {
					continue
				}

				if event.IsInteractive || event.Id != "" {
					c.epm.feed(event.Id, event.Params)
					continue
				}

				if event.IsSyncInfo {
					switch event.SyncType {
					case SYNC_TYPE_CONSUMPTION:
						consumptionNotification()
					case SYNC_TYPE_PING:
						c.emitJson(EVENT_TYPE_PONG, "system", map[string]any{
							"now":         time.Now().Format(time.RFC3339),
							"now_unix":    time.Now().Unix(),
							"now_unix_ms": time.Now().UnixMilli(),
						})
					case SYNC_TYPE_PLAN:
						c.syncMutex.RLock()
						callback, _ := c.syncMap[string(SYNC_TYPE_PLAN)]
						c.syncMutex.RUnlock()
						if callback != nil {
							c.emitJson(EVENT_TYPE_PLAN, "system", map[string]any{
								"root_task": callback(),
							})
						} else {
							c.EmitWarning("sync method: %v is not supported yet", SYNC_TYPE_PLAN)
						}

					}
				}
			case <-ticker.C:
				tickerCallback()
				continue
			case <-ctx.Done():
				return
			}
		}
	}()

	if err := initDefaultTools(c); err != nil {
		log.Errorf("init default tools: %v", err)
	}
	return c
}

type Option func(config *Config) error

func WithTool(tool *aitool.Tool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.tools = append(config.tools, tool)
		return nil
	}
}

func WithYOLO(i ...bool) Option {
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

func WithAgreeAIAssistant(a *AIAssistant) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.agreeAssistant = a
		return nil
	}
}

func WithAgreeAuto(auto bool, interval time.Duration) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(AgreePolicyAuto)
		config.agreeInterval = interval
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

func WithAgreeManual() Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(AgreePolicyManual)
		return nil
	}
}

func WithAIAgreeAuto(interval time.Duration) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.setAgreePolicy(AgreePolicyAIAuto)
		config.agreeInterval = interval
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

func WithToolManager(manager buildinaitools.ToolManager) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.aiToolManager = manager
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
		tools, err := searchtools.CreateAiToolsSearchTools(buildinaitools.GetAllTools)
		if err != nil {
			return utils.Errorf("create ai tools search tools: %v", err)
		}
		return WithTools(tools...)(config)
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
		config.timeLineLimit = i
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
		// set persistent prompt
		result := utils.Jsonify(i)
		config.memory.StoreAppendPersistentInfo(fmt.Sprint(`用户原始输入：` + string(result)))

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
