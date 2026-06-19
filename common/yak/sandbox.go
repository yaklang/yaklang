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

// library 生成一个沙箱配置项，向沙箱注入外部库(函数/变量表)
// 参数:
//   - lib: 要注入的外部库，键为名称，值为函数或变量
//
// 返回值:
//   - 可传给 sandbox.Create 的配置项
//
// Example:
// ```
// // VARS: 注入自定义变量后在沙箱表达式中使用
// sb = sandbox.Create(sandbox.library({"base": 10}))
// result = sb.ExecuteAsExpression("base + 5")~
// // STDOUT: 打印求值结果
// println(result)   // OUT: 15
// // assert: 注入的变量可在沙箱中使用
// assert result == 15, "injected library variable should be usable in sandbox"
// ```
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

// Create 创建一个沙箱(Sandbox)，用于在受限环境中执行表达式或代码
// 参数:
//   - opts: 可选配置，如 sandbox.library(...) 注入外部库
//
// 返回值:
//   - 沙箱对象，可调用 ExecuteAsExpression / ExecuteAsBoolean 等方法
//
// Example:
// ```
// // VARS: 创建沙箱并求值表达式
// sb = sandbox.Create()
// result = sb.ExecuteAsExpression("1 + 1")~
// // STDOUT: 打印求值结果
// println(result)   // OUT: 2
// // assert: 锁定结论
// assert result == 2, "sandbox should evaluate the expression"
// ```
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
