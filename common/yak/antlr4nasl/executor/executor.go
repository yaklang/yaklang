package executor

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/buildin_script"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/lib"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/visitors"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

//	func (e *ExecContext) CallNativeFunction(name string, mapParam map[string]interface{}, sliceParam []interface{}) (interface{}, error) {
//		params := NewNaslBuildInMethodParam()
//		for _, i1 := range sliceParam {
//			params.listParams = append(params.listParams, yakvm.NewAutoValue(i1))
//		}
//		for k, v := range mapParam {
//			params.mapParams[k] = yakvm.NewAutoValue(v)
//		}
//		if fn, ok := NaslLib[name]; ok {
//			return fn(engin, params), nil
//		}
//		return nil, utils.Errorf("not found build in method: %s", name)
//
// }

type Executor struct {
	*yakvm.VirtualMachine
	debug        bool
	dbcache      bool
	naslLibsPath string
	Compiler     *visitors.Compiler
	sourceCode   string
	logger       *log.Logger
	buildinLib   map[string]func(param *NaslBuildInMethodParam) any
}

func NewWithContext() *Executor {
	table := yakvm.NewSymbolTable()
	vm := yakvm.NewWithSymbolTable(table)
	vm.GetConfig().SetClosureSupport(false)
	vm.GetConfig().SetFunctionNumberCheck(false)
	vm.GetConfig().SetYVMMode(yakvm.NASL)
	executor := &Executor{
		Compiler:       visitors.NewCompilerWithSymbolTable(table),
		VirtualMachine: vm,
		buildinLib:     map[string]func(param *NaslBuildInMethodParam) any{},
		logger:         log.GetLogger("NASL Logger"),
		//loadedScriptsLock: &sync.Mutex{},
		//scriptExecMutexs:  make(map[string]*sync.Mutex),
	}
	executor.Compiler.RegisterVisitHook("__positions_hook", func(c *visitors.Compiler, ctx antlr.ParserRuleContext) {
		if start := ctx.GetStart(); start != nil {
			c.SetStartPosition(start.GetLine(), start.GetColumn())
		}
		if end := ctx.GetStop(); end != nil {
			c.SetStopPosition(end.GetLine(), end.GetColumn())
		}
	})
	m := make(map[string]any)
	m["__method_proxy__"] = func(params [][]interface{}) interface{} {
		var funName string
		if params != nil && len(params) > 0 && len(params[0]) == 1 {
			if v, ok := params[0][0].(int); ok {
				name, ok := executor.Compiler.GetSymbolTable().GetNameByVariableId(v)
				if ok {
					funName = name
				}
			}
		}
		naslParams := &NaslBuildInMethodParam{
			MapParams: make(map[string]*yakvm.Value),
		}
		for _, p := range params[1:] {
			name, ok := executor.Compiler.GetSymbolTable().GetNameByVariableId(p[0].(int))
			if ok {
				naslParams.MapParams[name] = yakvm.NewAutoValue(p[1])
			}
			naslParams.ListParams = append(naslParams.ListParams, yakvm.NewAutoValue(p[1]))
		}
		fn, ok := executor.buildinLib[funName]
		if !ok {
			panic(fmt.Sprintf("not found buildin method %s", funName))
		}
		return fn(naslParams)
	}
	m["__OpCallCallBack__"] = func(name string) {
		// 做一些函数调试的工作
		if name == "http_recv_headers2" {
			print()
		}
	}
	m["__nasl_global_var_table"] = make(map[int]*yakvm.Value)
	m["__function__include"] = func(name string) (interface{}, error) {
		if !strings.HasSuffix(name, ".inc") {
			panic(fmt.Sprintf("include file name must end with .inc"))
		}
		return nil, executor.EvalInclude(name)
	}
	m["__function__assert"] = func(n int, msg string) (interface{}, error) {
		b := n != 0
		if !b {
			panic(msg)
		}
		return nil, nil
	}
	vm.SetVars(m)
	vm.ImportLibs(lib.NaslBuildInNativeMethod)
	executor.ImportLibs(lib.NaslBuildInNativeMethod)
	executor.ImportLibs(lib.BuildInVars)
	return executor
}
func NewNaslExecutor() *Executor {
	return NewWithContext()
}

//	func (engine *Executor) GetScriptMuxByName(name string) *sync.Mutex {
//		engine.scriptExecMutexsLock.Lock()
//		defer engine.scriptExecMutexsLock.Unlock()
//		if v, ok := engine.scriptExecMutexs[name]; ok {
//			return v
//		}
//		engine.scriptExecMutexs[name] = &sync.Mutex{}
//		return engine.scriptExecMutexs[name]
//	}
//func (engine *Executor) RegisterBuildInMethodHook(name string, hook func(origin script_core.NaslBuildInMethod, engine *Executor, params *NaslBuildInMethodParam) (interface{}, error)) {
//	engine.buildInMethodHook[name] = hook
//}
//func (engine *Executor) UnRegisterBuildInMethodHook(name string) {
//	delete(engine.buildInMethodHook, name)
//}

func (e *Executor) SetLib(lib map[string]func(param *NaslBuildInMethodParam) any) {
	e.buildinLib = lib
}

//	func (e *Executor) IsScriptLoaded(scriptName string) bool {
//		e.loadedScriptsLock.Lock()
//		defer e.loadedScriptsLock.Unlock()
//		_, ok := e.loadedScripts[scriptName]
//		return ok
//	}

func (engine *Executor) SetIncludePath(path string) {
	engine.naslLibsPath = path
}

func (engine *Executor) Debug(bool2 ...bool) {
	if len(bool2) == 0 {
		engine.debug = true
	} else {
		engine.debug = bool2[0]
	}
}

func (e *Executor) Compile(code string) error {
	e.Compiler.SetExternalVariableNames(e.GetExternalVariableNames())
	e.Compiler.Debug(e.debug)
	ok := e.Compiler.Compile(code)
	if !ok {
		return e.Compiler.GetMergeError()
	}
	return nil
}
func (e *Executor) IsDebug() bool {
	return e.debug
}
func (e *Executor) SafeRunFile(path string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Error(e)
		}
	}()
	return e.RunFile(path)
}

//func (e *Executor) RunScript(script *script_core.NaslScriptInfo) error {
//	e.logger.Debugf("Running script %s", script.OriginFileName)
//	//e.ctx.scriptObj = script
//	return e.safeEvalWithFileName(script.Script, script.OriginFileName)
//}

func (e *Executor) EvalInclude(name string) error {
	// 优先从本地文件中查找，否则从内置的文件中查找
	var sourceBytes []byte
	libPath := path.Join(e.naslLibsPath, name)
	codes, err := os.ReadFile(libPath)
	if err == nil {
		sourceBytes = codes
	}
	//本地文件加载失败，从内置文件中加载
	if sourceBytes == nil {
		data, err := buildin_script.FS.ReadFile("nasl-incs/" + name)
		if err != nil {
			err = utils.Errorf("not found include file: %s", name)
			e.logger.Error(err)
			return err
		}
		sourceBytes = data
	}
	return e.safeEvalWithFileName(string(sourceBytes), name)
}

//	func (e *Executor) LoadScript(path string) (*NaslScriptInfo, error) {
//		e.SetDescription(true)
//		oldIns := e.GetScriptObject()
//		defer func() {
//			e.SetDescription(false)
//			e.scriptObj = oldIns
//		}()
//		e.scriptObj = NewNaslScriptObject()
//		e.scriptObj.OriginFileName = filepath.Base(path)
//		code, err := os.ReadFile(path)
//		if err != nil {
//			script, err := NewNaslScriptObjectFromDb(e.scriptObj.OriginFileName, e.dbcache)
//			if err != nil {
//				return nil, utils.Errorf("not found script file: %s", path)
//			}
//			return script, err
//		} else {
//			err = e.safeEvalWithFileName(string(code), e.scriptObj.OriginFileName)
//			return e.scriptObj, err
//		}
//	}
func (e *Executor) RunFile(path string) error {
	//e.ctx.scriptObj.OriginFileName = filepath.Base(path)
	code, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	recoverSource := e.Compiler.SetSourceCodeFilePath(path)
	defer recoverSource()
	return e.safeEvalWithFileName(string(code), path)
}

func (e *Executor) Exec(code, fileName string) error {
	defer func() {
		if err := recover(); err != nil {
			if data, ok := err.(*yakvm.VMPanicSignal); ok {
				info := data.AdditionalInfo.(map[string]string)
				code := info["code"]
				msg := info["msg"]
				e.logger.Infof("script [%s] exit with code: %v, msg: %v", fileName, code, msg)
				if e.debug {
					e.logger.Infof("script additional info: %v", data.AdditionalInfo)
				}
			} else {
				panic(err)
			}
		}
	}()
	recoverCode := e.Compiler.SetSourceCode(code)
	defer func() { recoverCode() }()
	e.sourceCode = code
	//e.ctx.scriptObj.Script = code
	err := e.Compile(code)
	if err != nil {
		return err
	}
	cfg := e.GetConfig()
	//if e.debug {
	//	cfg.SetStopRecover(true)
	//}
	e.SetConfig(cfg)
	err = e.ExecYakCode(context.Background(), code, e.Compiler.GetCodes(), yakvm.None)
	if err != nil {
		return err
	}
	return nil
}
func (e *Executor) safeEvalWithFileName(code string, fileName string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			if er, ok := e.(error); ok {
				err = er
			} else {
				err = utils.Error(e)
			}
		}
	}()
	recoverFunc := e.Compiler.SetSourceCodeFilePath(fileName)
	defer recoverFunc()
	err = e.Exec(code, fileName)
	return
}
func (e *Executor) SafeEval(code string) (err error) {
	return e.safeEvalWithFileName(code, "")
}

func (e *Executor) GetCompiler() *visitors.Compiler {
	return e.Compiler
}
