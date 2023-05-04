package antlr4nasl

import (
	"context"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/lib"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/visitors"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"os"
)

type Engine struct {
	compiler          *visitors.Compiler
	vm                *yakvm.VirtualMachine
	naslLibsPath      string
	description       bool
	sourceCode        string
	scriptObj         *NaslScriptInfo
	scripts           []string
	host              string
	smokeOnCodeHandle func(code *yakvm.Code) bool
}

func New() *Engine {
	table := yakvm.NewSymbolTable()
	vm := yakvm.NewWithSymbolTable(table)
	vm.GetConfig().SetClosureSupport(false)
	vm.GetConfig().SetFunctionNumberCheck(false)
	vm.GetConfig().SetYVMMode(yakvm.NASL)
	engine := &Engine{
		compiler: visitors.NewCompilerWithSymbolTable(table),
		vm:       vm,
	}

	engine.compiler.SetNaslLib(GetNaslLibKeys())
	engine.compiler.AddVisitHook(func(c *visitors.Compiler, ctx antlr.ParserRuleContext) {
		if start := ctx.GetStart(); start != nil {
			c.SetStartPosition(start.GetLine(), start.GetColumn())
		}
		if end := ctx.GetStop(); end != nil {
			c.SetStopPosition(end.GetLine(), end.GetColumn())
		}
	})
	vm.SetVar("__method_proxy__", func(params [][]interface{}) interface{} {
		var funName string
		if params != nil && len(params) > 0 && len(params[0]) == 1 {
			if v, ok := params[0][0].(int); ok {
				name, ok := engine.compiler.GetSymbolTable().GetNameByVariableId(v)
				if ok {
					funName = name
				}
			}
		}
		fn := NaslLib[funName]
		naslParams := &NaslBuildInMethodParam{
			mapParams: make(map[string]*yakvm.Value),
		}
		for _, p := range params[1:] {
			name, ok := engine.compiler.GetSymbolTable().GetNameByVariableId(p[0].(int))
			if ok {
				naslParams.mapParams[name] = yakvm.NewAutoValue(p[1])
			}
			naslParams.listParams = append(naslParams.listParams, yakvm.NewAutoValue(p[1]))
		}
		return fn(engine, naslParams)
		//panic("call build in method error: not found symbol id")
	})
	vm.SetVar("__OpCallCallBack__", func(name string) {
		if name == "http_keepalive_send_recvs" {
			println()
		}
	})
	vm.ImportLibs(lib.NaslBuildInNativeMethod)
	engine.scriptObj = NewNaslScriptObject()
	return engine
}
func (engine *Engine) AddSmokeOnCode(condation func(code *yakvm.Code) bool) {
	engine.smokeOnCodeHandle = condation
}
func (engine *Engine) LoadScript(path string) {
	engine.scripts = append(engine.scripts, path)
}
func (engin *Engine) CallNativeFunction(name string, mapParam map[string]interface{}, sliceParam []interface{}) (interface{}, error) {
	params := NewNaslBuildInMethodParam()
	for _, i1 := range sliceParam {
		params.listParams = append(params.listParams, yakvm.NewAutoValue(i1))
	}
	for k, v := range mapParam {
		params.mapParams[k] = yakvm.NewAutoValue(v)
	}
	if fn, ok := NaslLib[name]; ok {
		return fn(engin, params), nil
	}
	return nil, utils.Errorf("not found build in method: %s", name)

}

func (engine *Engine) Scan(target string, ports string) error {
	engine.host = target
	log.Infof("start syn scan target: %s, ports: %s", target, ports)
	//portres, err := SynScan(target, ports)
	//if err != nil {
	//	return err
	//}
	//native_lib.Set_kb_item("Host/scanned", portres)
	params := make(map[string]interface{})
	params["name"] = "Host/scanned"
	params["value"] = []int{443}
	engine.CallNativeFunction("set_kb_item", params, nil)
	for _, script := range engine.scripts {
		err := engine.RunFile(script)
		if err != nil {
			log.Errorf("run script %s met error: %s", script, err)
		}
	}
	return nil
}
func (engine *Engine) SynScan(target string, ports string) ([]int, error) {
	return SynScan(target, ports)
	//native_lib.Set_kb_item("Host/scanned", res)
}
func (engine *Engine) Init() {
	engine.vm.ImportLibs(lib.NaslBuildInNativeMethod)
	engine.vm.ImportLibs(lib.BuildInVars)
}
func (e *Engine) Compile(code string) error {
	e.compiler.SetExternalVariableNames(e.vm.GetExternalVariableNames())

	ok := e.compiler.Compile(code)
	if !ok {
		return e.compiler.GetMergeError()
	}
	return nil
}
func (e *Engine) SafeRunFile(path string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Error(e)
		}
	}()
	return e.RunFile(path)
}
func (e *Engine) RunFile(path string) error {
	code, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	e.compiler.SetSourceCodeFilePath(path)
	return e.Eval(string(code))
}
func (e *Engine) Eval(code string) error {
	defer func() {
		if e := recover(); e != nil {
			if v, ok := e.(*yakvm.VMPanicSignal); ok {
				log.Infof("script exit with value: %v", v.Info)
			} else {
				panic(e)
			}
		}
	}()
	recoverCode := e.compiler.SetSourceCode(code)
	defer func() { recoverCode() }()
	e.sourceCode = code
	e.scriptObj.Script = code
	e.vm.SetVar("__this__", e.scriptObj)
	e.vm.SetVar("description", e.description)
	err := e.Compile(code)
	if err != nil {
		return err
	}
	err = e.vm.ExecYakCode(context.Background(), code, e.compiler.GetCodes(), yakvm.None)
	if err != nil {
		return err
	}
	if e.description {
		if e.scriptObj != nil && e.scriptObj.OID != "" {
			return e.scriptObj.Save()
		}
	}
	return nil
}
func (e *Engine) SafeEval(code string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Error(e)
		}
	}()
	err = e.Eval(code)
	return
}
func (e *Engine) SetIncludePath(p string) {
	e.naslLibsPath = p
}
func (e *Engine) GetVirtualMachine() *yakvm.VirtualMachine {
	return e.vm
}
func (e *Engine) GetCompiler() *visitors.Compiler {
	return e.compiler
}

func (e *Engine) SetDescription(b bool) {
	e.description = b
}
