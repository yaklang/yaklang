package antlr4yak

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

	"github.com/davecgh/go-spew/spew"
)

const YAKC_CACHE_MAX_LENGTH = 300

type Engine struct {
	rootSymbol            *yakvm.SymbolTable
	vm                    *yakvm.VirtualMachine
	strictMode            bool
	sourceFilePathPointer *string
	// debug
	debug         bool // 内部debug
	debugMode     bool // 外部debugger
	debugCallBack func(*yakvm.Debugger)
	debugInit     func(*yakvm.Debugger)
	sandboxMode   bool

	callFuncCallback func(caller *yakvm.Value, wavy bool, args []*yakvm.Value)
}

func (e *Engine) RuntimeInfo(infoType string, params ...any) (res any, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	frame := e.GetVM().VMStack.Peek()
	if frame == nil {
		return nil, fmt.Errorf("not found runtime.GetInfo")
	}
	runtimeLib, ok := frame.(*yakvm.Frame).GlobalVariables.Load("runtime")
	if !ok || runtimeLib == nil {
		return nil, fmt.Errorf("current frame not import runtime lib")
	}
	getInfoFun := runtimeLib.(map[string]any)["GetInfo"]
	if getInfoFun == nil {
		return nil, fmt.Errorf("not found runtime.GetInfo")
	}
	return getInfoFun.(func(string, ...any) (any, error))(infoType, params...)
}

func (e *Engine) SetStrictMode(b bool) {
	if e == nil {
		return
	}
	e.strictMode = b
}

func New() *Engine {
	table := yakvm.NewSymbolTable()
	vm := yakvm.NewWithSymbolTable(table)
	engine := &Engine{
		rootSymbol: table,
		vm:         vm,
	}
	InjectContextBuiltinFunction(engine)
	// evalFunc := func(ctx context.Context, code string) {
	// 	codes, err := engine.Compile(code)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	if err = vm.ExecYakCode(ctx, code, codes, yakvm.Inline); err != nil {
	// 		panic(err)
	// 	}
	// }
	// runtimeLibs := map[string]interface{}{
	// 	"Breakpoint": func() {
	// 		if engine.debugMode {
	// 			debugger := engine.vm.GetDebugger()
	// 			if debugger != nil {
	// 				debugger.HandleForBreakPoint()
	// 			}
	// 		}
	// 	},
	// }

	// engine.ImportLibs(map[string]interface{}{
	// 	"eval": evalFunc,
	// 	"yakfmt": func(code string) string {
	// 		newCode, err := New().FormattedAndSyntaxChecking(code)
	// 		if err != nil {
	// 			log.Errorf("format and syntax checking met error: %s", err)
	// 			return code
	// 		}
	// 		return newCode
	// 	},
	// 	"yakfmtWithError": func(code string) (string, error) {
	// 		return New().FormattedAndSyntaxChecking(code)
	// 	},
	// 	"getScopeInspects": func() ([]*ScopeValue, error) {
	// 		return engine.GetScopeInspects()
	// 	},
	// })
	return engine
}

func (n *Engine) SetSourceFilePath(path string) {
	n.sourceFilePathPointer = &path
}

func (n *Engine) SetDebugInit(callback func(*yakvm.Debugger)) {
	n.debugInit = callback
}

func (n *Engine) SetDebugCallback(callback func(*yakvm.Debugger)) {
	n.debugCallBack = callback
}

func (n *Engine) SetDebugMode(debug bool) {
	n.debugMode = debug
}

func (n *Engine) EnableStrictMode() {
	n.strictMode = true
}

func (n *Engine) GetSymNames() []string {
	return nil
}

func (n *Engine) CopyVars() map[string]interface{} {
	return nil
}

func (n *Engine) GetVM() *yakvm.VirtualMachine {
	return n.vm
}

func (n *Engine) CallYakFunctionNative(ctx context.Context, function *yakvm.Function, params ...interface{}) (interface{}, error) {
	if function == nil {
		return nil, utils.Error("no function")
	}
	paramsValue := make([]*yakvm.Value, len(params))
	for i, v := range params {
		paramsValue[i] = yakvm.NewAutoValue(v)
	}
	return n.vm.ExecYakFunction(ctx, function, yakvm.YakVMValuesToFunctionMap(function, paramsValue, n.vm.GetConfig().GetFunctionNumberCheck()), yakvm.None)
}

func (n *Engine) SafeCallYakFunctionNativeWithFrameCallback(ctx context.Context, frameCallback func(*yakvm.Frame), function *yakvm.Function, params ...interface{}) (result interface{}, err error) {
	defer func() {
		if e := recover(); e != nil {
			if v, ok := e.(error); ok {
				err = v
			} else {
				err = utils.Error(e)
			}
		}
	}()
	return n.CallYakFunctionNativeWithFrameCallback(ctx, frameCallback, function, params...)
}

func (n *Engine) CallYakFunctionNativeWithFrameCallback(ctx context.Context, frameCallback func(*yakvm.Frame), function *yakvm.Function, params ...interface{}) (interface{}, error) {
	if function == nil {
		return nil, utils.Error("no function")
	}
	paramsValue := make([]*yakvm.Value, len(params))
	for i, v := range params {
		paramsValue[i] = yakvm.NewAutoValue(v)
	}
	return n.vm.ExecYakFunctionEx(ctx, function, yakvm.YakVMValuesToFunctionMap(function, paramsValue, n.vm.GetConfig().GetFunctionNumberCheck()), frameCallback, yakvm.None)
}

func (n *Engine) SafeCallYakFunction(ctx context.Context, funcName string, params []interface{}) (result interface{}, err error) {
	defer func() {
		if e := recover(); e != nil {
			if v, ok := e.(error); ok {
				err = v
			} else {
				err = utils.Error(e)
			}
		}
	}()
	return n.CallYakFunction(ctx, funcName, params)
}

// 函数调用时如果不加锁，并发会有问题
func (n *Engine) CallYakFunction(ctx context.Context, funcName string, params []interface{}) (interface{}, error) {
	i, ok := n.GetVar(funcName)
	if !ok {
		return nil, utils.Errorf("function %s not found", funcName)
	}
	if i == nil {
		return nil, utils.Errorf("function %s is nil", funcName)
	}

	f, ok := i.(*yakvm.Function)
	if !ok {
		return nil, utils.Errorf("cannot convert %s to yakvm.Function, got type: %T", funcName, i)
	}

	if f == nil {
		return nil, utils.Errorf("function %s is nil after type assertion", funcName)
	}

	return n.CallYakFunctionNative(ctx, f, params...)
	//
	//returnValueReciver := "__global_return__"
	//paramStrList := []string{}
	//for index, param := range params {
	//	paramName := fmt.Sprintf("__global_param_%d__", index)
	//	n.vm.SetVars(paramName, param)
	//	paramStrList = append(paramStrList, paramName)
	//}
	//
	//code := fmt.Sprintf("%s = %s(%s)", returnValueReciver, funcName, strings.Join(paramStrList, ","))
	//err := n.SafeEval(code)
	//if err != nil {
	//	return nil, err
	//}
	//returnValue := n.Var(returnValueReciver)
	//return returnValue, nil
}

// RunFile 手动执行Yak脚本一般都是从文件开始执行，这种情况建议使用RunFile执行代码，便于报错时提供文件路径信息
func (n *Engine) RunFile(ctx context.Context, path string) error {
	n.sourceFilePathPointer = &path
	defer func() {
		n.sourceFilePathPointer = nil
	}()
	codeB, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return n.Eval(ctx, string(codeB))
}

// LoadCode 暂时使用Eval的方式加载代码，后面可以进行优化，只编译声明语句和assign语句
func (n *Engine) LoadCode(ctx context.Context, code string, table map[string]interface{}) error {
	codes, err := n.Compile(code)
	if err != nil {
		return err
	}
	n.vm.ImportLibs(table)
	return n.vm.ExecYakCode(ctx, code, codes, yakvm.Trace)
}

func (n *Engine) GetFntable() map[string]any {
	r := n.vm.GetRuntimeGlobalVar()
	results := r.Flat()
	return results
}

func (n *Engine) GetCurrentScope() (*yakvm.Scope, error) {
	frame := n.vm.CurrentFM()
	if frame == nil {
		return nil, utils.Error("current frame empty")
	}
	return frame.CurrentScope(), nil
}

func (n *Engine) GetLastStackValue() (*yakvm.Value, error) {
	frame := n.vm.CurrentFM()
	if frame == nil {
		return nil, utils.Error("current frame empty")
	}
	val := frame.GetLastStackValue()
	if val == nil {
		return nil, utils.Errorf("last Stack Value is nil/undefined")
	}
	return val, nil
}

type ScopeValue struct {
	Name         string
	Id           int
	Value        interface{}
	ValueVerbose string
}

func (n *Engine) GetScopeInspects() ([]*ScopeValue, error) {
	scope, err := n.GetCurrentScope()
	if err != nil {
		return nil, err
	}
	ids := scope.GetAllIdInScopes()
	var vals []*ScopeValue
	for _, id := range ids {
		name := scope.GetNameById(id)
		if name == "" {
			continue
		}
		val, ok := scope.GetValueByID(id)
		if !ok {
			continue
		}
		vals = append(vals, &ScopeValue{
			Name:         name,
			Id:           id,
			Value:        val.Value,
			ValueVerbose: spew.Sdump(val.Value),
		})
	}
	val, _ := n.GetLastStackValue()
	if val != nil {
		vals = append(vals, &ScopeValue{
			Name:         "_",
			Id:           0,
			Value:        val.Value,
			ValueVerbose: spew.Sdump(val.Value),
		})
	}
	sort.SliceStable(vals, func(i, j int) bool {
		return vals[i].Id < vals[j].Id
	})
	return vals, nil
}

func (n *Engine) OverrideRuntimeGlobalVariables(parent map[string]any) {
	n.vm.SetVars(parent)

	//var parentLib map[string]interface{}
	//
	//if v, ok := n.vm.GetGlobalVar()[parent]; ok {
	//	if v1, ok := v.(map[string]interface{}); ok {
	//		parentLib = v1
	//	}
	//} else {
	//	parentLib = make(map[string]interface{})
	//	n.vm.GetGlobalVar()[parent] = parentLib
	//}
	//for k, v2 := range libs {
	//	parentLib[k] = v2
	//}
}

func (n *Engine) ImportLibs(libs map[string]interface{}) {
	n.vm.ImportLibs(libs)
}

func (n *Engine) Var(name string) interface{} {
	v, _ := n.vm.GetVar(name)
	return v
}

func (n *Engine) GetVar(name string) (interface{}, bool) {
	return n.vm.GetVar(name)
}

func (n *Engine) SetVars(m map[string]any) {
	n.vm.SetVars(m)
}

func (n *Engine) CompileWithCurrentScope(code string) ([]*yakvm.Code, error) {
	scope, err := n.GetCurrentScope()
	if err != nil {
		return nil, err
	}
	compiler, err := n._compile(code, scope.GetSymTable())
	if err != nil {
		return nil, err
	}
	return compiler.GetOpcodes(), err
}

func (n *Engine) Compile(code string) ([]*yakvm.Code, error) {
	compiler, err := n._compile(code, n.rootSymbol)
	if err != nil {
		return nil, err
	}
	return compiler.GetOpcodes(), err
}

func (n *Engine) MustCompile(code string) []*yakvm.Code {
	compiler, err := n._compile(code, n.rootSymbol)
	if err != nil {
		panic(err)
	}
	return compiler.GetOpcodes()
}

func (n *Engine) _compile(code string, symbolTable *yakvm.SymbolTable) (*yakast.YakCompiler, error) {
	compiler := yakast.NewYakCompilerWithSymbolTable(symbolTable)
	compiler.SetStrictMode(n.strictMode)
	if n.strictMode {
		compiler.SetExternalVariableNames(n.vm.GetExternalVariableNames())
	}
	if n.sourceFilePathPointer != nil {
		compiler.CompileSourceCodeWithPath(code, n.sourceFilePathPointer)
	} else {
		compiler.Compiler(code)
	}
	if len(compiler.GetErrors()) > 0 {
		return nil, compiler.GetErrors()
	}
	return compiler, nil
}

func (n *Engine) EnableDebug() {
	n.debug = true
}

func (n *Engine) SetCallFuncCallback(callback func(caller *yakvm.Value, wavy bool, args []*yakvm.Value)) {
	n.callFuncCallback = callback
}

func (e *Engine) SetSandboxMode(mode bool) { // sandbox mode call use defineFrame
	e.sandboxMode = mode
	e.vm.SetSandboxMode(mode)
}

func (n *Engine) HaveEvaluatedCode() bool {
	return !n.rootSymbol.IsNew()
}

func (n *Engine) Eval(ctx context.Context, code string) error {
	return n.EvalWithInline(ctx, code, false)
}

func (n *Engine) EvalInline(ctx context.Context, code string) error {
	return n.EvalWithInline(ctx, code, true)
}

func (n *Engine) EvalWithInline(ctx context.Context, code string, inline bool) error {
	flag := yakvm.None
	if inline {
		flag = yakvm.Inline
	}

	compiler, err := n._compile(code, n.rootSymbol)
	if err != nil {
		return utils.Errorf("compile error: \n%s", err)
	}
	n.vm.SetDebug(n.debug)
	n.vm.SetDebugMode(n.debugMode, code, compiler.GetOpcodes(), n.debugInit, n.debugCallBack)
	n.vm.SetCallFuncCallback(n.callFuncCallback)
	// yakc缓存
	codes, symtbl := compiler.GetOpcodes(), compiler.GetRootSymbolTable()
	defer func() {
		if len(code) <= YAKC_CACHE_MAX_LENGTH {
			return
		}
		yc, err := n._marshal(symtbl, codes, nil)
		if err != nil {
			return
		}
		SaveYakcCache(code, yc)
	}()

	err = n.vm.ExecYakCode(ctx, code, codes, flag)
	if err != nil {
		return err
	}
	return nil
}

func (n *Engine) SafeEval(ctx context.Context, code string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Error(fmt.Sprint(e))
			// log.Error(err)
		}
	}()
	err = n.Eval(ctx, code)
	return
}

func (n *Engine) SafeEvalInline(ctx context.Context, code string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Error(fmt.Sprint(e))
		}
	}()
	err = n.EvalInline(ctx, code)
	return
}

func (n *Engine) FormattedAndSyntaxChecking(code string) (string, error) {
	compiler, err := n._compile(code, n.rootSymbol)
	if err != nil {
		return "", err
	}
	return compiler.GetFormattedCode(), err
}

func (n *Engine) ExecuteAsBooleanExpression(expr string, dependencies map[string]interface{}) (bool, error) {
	if dependencies != nil {
		n.ImportLibs(dependencies)
	}
	err := n.SafeEvalInline(context.Background(), expr)
	if err != nil {
		return false, err
	}

	val, err := n.GetLastStackValue()
	if err != nil {
		return false, err
	}
	if val == nil {
		return false, nil
	}
	if val.IsBool() {
		return val.Bool(), nil
	}
	if funk.IsEmpty(val.Value) || funk.IsZero(val.Value) {
		return false, nil
	}
	return true, nil
}

func (n *Engine) ExecuteAsExpression(expr string, dependencies map[string]interface{}) (interface{}, error) {
	if dependencies != nil {
		n.ImportLibs(dependencies)
	}

	err := n.SafeEvalInline(context.Background(), expr)
	if err != nil {
		return nil, err
	}

	val, err := n.GetLastStackValue()
	if err != nil {
		return nil, err
	}
	if val == nil {
		return nil, nil
	}
	return val.Value, nil
}

func (n *Engine) SetExternalVarGetter(f func(name string) (any, bool)) {
	n.vm.GetExternalVar = f
}
