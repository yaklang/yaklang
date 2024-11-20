package yak

import (
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

type Sandbox struct {
	config *SandboxConfig
	engine *antlr4yak.Engine
	mutex  *sync.Mutex
}

type SandboxConfig struct {
	lib               map[string]any
	importYaklangLibs bool
}

type SandboxOption func(*SandboxConfig)

func WithSandbox_ExternalLib(lib map[string]any) SandboxOption {
	return func(config *SandboxConfig) {
		if config.lib == nil {
			config.lib = make(map[string]any)
		}
		for k, v := range lib {
			config.lib[k] = v
		}
	}
}

func WithYaklang_Libs(b bool) SandboxOption {
	return func(config *SandboxConfig) {
		config.importYaklangLibs = b
	}
}

func NewSandbox(opts ...SandboxOption) *Sandbox {
	c := &SandboxConfig{}
	for _, opt := range opts {
		opt(c)
	}

	if c.lib == nil {
		c.lib = make(map[string]any)
	}
	var engine *antlr4yak.Engine

	if c.importYaklangLibs {
		engine = yaklang.NewAntlrEngine()
	} else {
		engine = antlr4yak.New()
	}

	if os.Getenv("YAKMODE") == "strict" {
		engine.EnableStrictMode()
	}
	engine.ImportLibs(c.lib)
	engine.SetSandboxMode(true)

	return &Sandbox{
		config: c,
		engine: engine,
		mutex:  new(sync.Mutex),
	}
}

func (s Sandbox) ExecuteAsExpressionRaw(code string, vars map[string]any) (ret any, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.engine.ExecuteAsExpression(code, vars)
}

func (s *Sandbox) ExecuteAsExpression(code string, vars ...any) (ret any, err error) {
	merged := make(map[string]any)
	for _, v := range vars {
		for k, v := range utils.InterfaceToGeneralMap(v) {
			merged[k] = v
		}
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.engine.ExecuteAsExpression(code, merged)
}

func (s *Sandbox) ExecuteAsBoolean(code string, vars ...any) (ret bool, err error) {
	merged := make(map[string]any)
	for _, v := range vars {
		for k, v := range utils.InterfaceToGeneralMap(v) {
			merged[k] = v
		}
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.engine.ExecuteAsBooleanExpression(code, merged)
}

var SandboxExports = map[string]any{
	"Create":  NewSandbox,
	"library": WithSandbox_ExternalLib,
}
