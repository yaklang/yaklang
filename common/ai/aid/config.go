package aid

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"sync"
	"time"
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

	debugPrompt bool
	debugEvent  bool
}

func (c *Config) wrapper(i AICallbackType) AICallbackType {
	return func(request *AIRequest) (*AIResponse, error) {
		if c.debugPrompt {
			log.Infof(strings.Repeat("=", 20)+"AIRequest"+strings.Repeat("=", 20)+"\n%v\n", request.GetPrompt())
		}
		resp, err := i(request)
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
		m:   new(sync.Mutex),
		id:  id.String(),
		epm: newEndpointManager(),
	}
	go func() {
		log.Infof("config %s started, start to handle receiving loop", c.id)
		logOnce := new(sync.Once)
		for {
			if c.eventInputChan == nil {
				logOnce.Do(func() {
					log.Infof("event input chan is nil, will retry in 1 second")
				})
				select {
				case <-time.After(time.Second):
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
				c.epm.feed(event.Id, event.Params)
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
