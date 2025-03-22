package aid

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
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

func (c *Config) emit(e *Event) {
	c.m.Lock()
	defer c.m.Unlock()
	if c.eventHandler == nil {
		log.Info(e.String())
		return
	}
	c.eventHandler(e)
}

func newConfig() *Config {
	id := uuid.New()
	c := &Config{
		m:  new(sync.Mutex),
		id: id.String(),
	}

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
		config.coordinatorAICallback = cb
		config.taskAICallback = cb
		config.planAICallback = cb
		return nil
	}
}

func WithTaskAICallback(cb AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.taskAICallback = cb
		return nil
	}
}

func WithCoordinatorAICallback(cb AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.coordinatorAICallback = cb
		return nil
	}
}

func WithPlanAICallback(cb AICallbackType) Option {
	return func(config *Config) error {
		config.m.Lock()
		defer config.m.Unlock()
		config.planAICallback = cb
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
