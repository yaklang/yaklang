package aid

import (
	"context"
	"fmt"
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

type Config struct {
	m  *sync.Mutex
	id string

	eventInputChan chan *InputEvent
	epm            *endpointManager

	// need to think
	coordinatorAICallback AICallbackType
	planAICallback        AICallbackType

	// no need to think, low level
	taskAICallback AICallbackType
	tools          []*aitool.Tool
	eventHandler   func(e *Event)

	// memory
	memory *Memory

	// stream waitgroup
	streamWaitGroup *sync.WaitGroup

	debugPrompt bool
	debugEvent  bool
	autoAgree   bool

	// sync
	syncMutex *sync.RWMutex
	syncMap   map[string]func() any

	inputConsumption  *int64
	outputConsumption *int64

	// RiskControl
	riskCtrl *riskControl
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
		c.inputConsumptionCallback(estimateTokens([]byte(request.GetPrompt())))
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

func newConfig(ctx context.Context) *Config {
	id := uuid.New()
	c := &Config{
		m:                 new(sync.Mutex),
		id:                id.String(),
		epm:               newEndpointManagerContext(ctx),
		streamWaitGroup:   new(sync.WaitGroup),
		memory:            NewMemory(),
		syncMutex:         new(sync.RWMutex),
		syncMap:           make(map[string]func() any),
		inputConsumption:  new(int64),
		outputConsumption: new(int64),
		riskCtrl:          new(riskControl),
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

	if err := WithTools(buildinaitools.GetBasicBuildInTools()...)(c); err != nil {
		log.Errorf("get basic build in tools: %v", err)
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

func WithAutoAgree(autoAgree bool) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.autoAgree = autoAgree
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
		config.coordinatorAICallback = config.wrapper(cb)
		config.taskAICallback = config.wrapper(cb)
		config.planAICallback = config.wrapper(cb)
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
