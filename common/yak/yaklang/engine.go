package yaklang

import (
	"context"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"os"
)

var yaklangLibs map[string]interface{}

func init() {
	yaklangLibs = make(map[string]interface{})
}
func Import(mod string, v interface{}) {
	yaklangLibs[mod] = v
}

// -----------------------------------------------------------------------------

// A YakEngine represents the yaklangspec compiler and executor.
type YaklangEngine interface {
	HaveEvaluatedCode() bool
	SafeEval(context.Context, string) error
	Eval(context.Context, string) error
	CallYakFunction(context.Context, string, []interface{}) (interface{}, error)
	LoadCode(context.Context, string, map[string]interface{}) error
	SetVar(string, interface{})
	Var(string) interface{}
	GetVar(string) (interface{}, bool)
	ImportLibs(map[string]interface{})
	GetFntable() map[string]interface{}
	GetSymNames() []string
	CopyVars() map[string]interface{}
	Marshal(string, []byte) ([]byte, error)
	ExecYakc(context.Context, []byte, []byte) error
	SafeExecYakc(context.Context, []byte, []byte) error
	ExecYakcWithCode(context.Context, []byte, []byte, string) error
	SafeExecYakcWithCode(context.Context, []byte, []byte, string) error
	SetDebugMode(bool)
	SetDebugInit(func(*yakvm.Debugger))
	SetDebugCallback(func(*yakvm.Debugger))
}

func IsNew() bool {
	return os.Getenv("YAKMODE") == "vm"
}

func NewSandbox(vars map[string]interface{}) *antlr4yak.Engine {
	engine := antlr4yak.New()
	if os.Getenv("YAKMODE") == "strict" {
		engine.EnableStrictMode()
	}
	engine.ImportLibs(vars)
	return engine
}

func NewAntlrEngine() YaklangEngine {
	engine := antlr4yak.New()
	if os.Getenv("YVMMODE") == "strict" {
		engine.EnableStrictMode()
	}
	engine.ImportLibs(yaklangLibs)
	engine.ImportSubLibs("yakit", map[string]interface{}{
		"AutoInitYakit": func() {
			if client := yaklib.AutoInitYakit(); client != nil {
				yaklib.SetEngineClient(engine, client)
			}
		},
	})
	yaklib.SetEngineClient(engine, yaklib.GetYakitClientInstance())
	return engine
}
func New() YaklangEngine {
	return NewAntlrEngine()
}
