package yaklang

import (
	"os"

	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

var yaklangLibs map[string]interface{}

func init() {
	yaklangLibs = make(map[string]interface{})
}
func Import(mod string, v interface{}) {
	yaklangLibs[mod] = v
}

// -----------------------------------------------------------------------------

func IsNew() bool {
	return true
}

func NewSandbox(vars map[string]interface{}) *antlr4yak.Engine {
	engine := antlr4yak.New()
	if os.Getenv("YAKMODE") == "strict" {
		engine.EnableStrictMode()
	}
	engine.ImportLibs(vars)
	engine.SetSandboxMode(true)
	return engine
}

func NewAntlrEngine() *antlr4yak.Engine {
	engine := antlr4yak.New()
	if os.Getenv("STATIC_CHECK") == "strict" {
		engine.EnableStrictMode()
	}
	engine.ImportLibs(yaklangLibs)
	engine.OverrideRuntimeGlobalVariables(map[string]any{
		"yakit": map[string]interface{}{
			"AutoInitYakit": func() {
				if client := yaklib.AutoInitYakit(); client != nil {
					yaklib.SetEngineClient(engine, client)
				}
			},
		}})
	yaklib.SetEngineClient(engine, yaklib.GetYakitClientInstance())
	return engine
}
func New() *antlr4yak.Engine {
	return NewAntlrEngine()
}
