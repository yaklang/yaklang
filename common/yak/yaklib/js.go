package yaklib

import (
	"crypto/rand"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"github.com/dop251/goja_nodejs/buffer"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javascript"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/embed"
)

type jsFunction func(v ...any) (goja.Value, error)

var (
	defaultJSRuntime = goja.New()
	bigMaxIntMap     = map[reflect.Kind]*big.Int{
		reflect.Int8:   big.NewInt(math.MaxInt8),
		reflect.Uint8:  big.NewInt(math.MaxUint8),
		reflect.Int16:  big.NewInt(math.MaxInt16),
		reflect.Uint16: big.NewInt(math.MaxUint16),
		reflect.Int32:  big.NewInt(math.MaxInt32),
		reflect.Uint32: big.NewInt(math.MaxUint32),
	}
)

var JSExports = map[string]interface{}{
	"PoweredBy":            "github.com/dop251/goja",
	"New":                  _jsNewEngine,
	"Run":                  _run,
	"CallFunctionFromCode": _jsCallFuncFromCode,

	// RunOptions
	"libCryptoJSV3": _libCryptoJSV3,
	"libCryptoJSV4": _libCryptoJSV4,
	"libJSRSASign":  _libJSRSASign,
	"libJsEncrypt":  _libJsEncrypt,
	"withVariable":  _withVariable,
	"withVariables": _withVariables,

	// AST
	"ASTWalk":   javascript.BasicJavaScriptASTWalker,
	"Parse":     _Parse,
	"GetSTType": GetStatementType,

	// Value
	"NullValue":      goja.Null(),
	"UndefinedValue": goja.Undefined(),
	"FalseValue":     defaultJSRuntime.ToValue(false),
	"ToValue":        defaultJSRuntime.ToValue,
	"NaNValue":       goja.NaN(),
	"TrueValue":      defaultJSRuntime.ToValue(true),

	// function
	"GetObjectFunction": _getObjectFunction,
	"GetFunction":       _getFunction,
}

func init() {
	require.RegisterCoreModule(console.ModuleName, console.RequireWithPrinter(defaultStdPrinter))
}

// GetStatementType 返回 JS AST 节点的类型名（去掉 *ast. 前缀，导出名为 js.GetSTType）
// 参数:
//   - st: JS AST 节点（如 js.Parse 的返回值或其子节点）
//
// 返回值:
//   - 节点类型名字符串
//
// Example:
// ```
// tree = js.Parse("function add(a, b) { return a + b; }")~
// println(js.GetSTType(tree))   // OUT: Program
// assert js.GetSTType(tree) == "Program", "parsed root node should be Program"
// ```
func GetStatementType(st interface{}) string {
	typ := strings.Replace(reflect.TypeOf(st).String(), "*ast.", "", 1)
	return typ
}

type jsLibrary struct {
	program *goja.Program
	name    string
	version string
}

type JsRunConfig struct {
	variables map[string]any
	libs      []*jsLibrary
}

func newJsRunConfig() *JsRunConfig {
	return &JsRunConfig{
		variables: make(map[string]any),
	}
}

type jsRunOpts func(*JsRunConfig)

func jsRunWithLibs(libs ...*jsLibrary) jsRunOpts {
	return func(c *JsRunConfig) {
		c.libs = append(c.libs, libs...)
	}
}

// withVariable 为 JS 运行设置单个全局变量（导出名为 js.withVariable）
// 参数:
//   - name: 变量名
//   - value: 变量值
//
// 返回值:
//   - JS 运行选项
//
// Example:
// ```
// _, value = js.Run("a + b", js.withVariable("a", 1), js.withVariable("b", 2))~
// println(value.ToInteger())   // OUT: 3
// assert value.ToInteger() == 3, "a + b with injected variables should be 3"
// ```
func _withVariable(name string, value any) jsRunOpts {
	return func(c *JsRunConfig) {
		c.variables[name] = value
	}
}

// withVariables 为 JS 运行批量设置多个全局变量（导出名为 js.withVariables）
// 参数:
//   - vars: 变量名到变量值的映射
//
// 返回值:
//   - JS 运行选项
//
// Example:
// ```
// _, value = js.Run("a + b", js.withVariables({"a": 10, "b": 5}))~
// println(value.ToInteger())   // OUT: 15
// assert value.ToInteger() == 15, "a + b with injected variables should be 15"
// ```
func _withVariables(vars map[string]any) jsRunOpts {
	return func(c *JsRunConfig) {
		for k, v := range vars {
			c.variables[k] = v
		}
	}
}

var jsRunOptsCache = utils.NewTTLCache[jsRunOpts]()

// libCryptoJSV3 是一个 JS 运行选项参数，用于在运行 JS 代码时嵌入 CryptoJS 3.3.0 库
// 返回值:
//   - JS 运行选项
//
// Example:
// ```
// _, value = js.Run(`CryptoJS.HmacSHA256("Message", "secret").toString();`, js.libCryptoJSV3())~
// assert value.String() == "aa747c502a898200f9e4fa21bac68136f886a0e27aec70ba06daf2e2a5cb5597", "HmacSHA256 should be deterministic"
// ```
func _libCryptoJSV3() jsRunOpts {
	var (
		opt jsRunOpts
		ok  bool
	)

	if opt, ok = jsRunOptsCache.Get("libCryptoJSV3"); !ok {
		src, _ := embed.Asset("data/js-libs/cryptojs/3.3.0/cryptojs.min.js.gz")
		prog, _ := goja.Compile("CryptoJS-3.3.0", string(src), false)
		opt = jsRunWithLibs(&jsLibrary{prog, "CryptoJS", "3.3.0"})
		jsRunOptsCache.Set("libCryptoJSV3", opt)
	}
	return opt
}

// libCryptoJSV4 是一个 JS 运行选项参数，用于在运行 JS 代码时嵌入 CryptoJS 4.2.0 库
// 返回值:
//   - JS 运行选项
//
// Example:
// ```
// _, value = js.Run(`CryptoJS.HmacSHA256("Message", "secret").toString();`, js.libCryptoJSV4())~
// assert value.String() == "aa747c502a898200f9e4fa21bac68136f886a0e27aec70ba06daf2e2a5cb5597", "HmacSHA256 should be deterministic"
// ```
func _libCryptoJSV4() jsRunOpts {
	var (
		opt jsRunOpts
		ok  bool
	)

	if opt, ok = jsRunOptsCache.Get("libCryptoJSV4"); !ok {
		src, _ := embed.Asset("data/js-libs/cryptojs/4.2.0/cryptojs.min.js.gz")
		prog, _ := goja.Compile("CryptoJS-4.2.0", string(src), false)
		opt = jsRunWithLibs(&jsLibrary{prog, "CryptoJS", "4.2.0"})
		jsRunOptsCache.Set("libCryptoJSV4", opt)
	}
	return opt
}

// libJSRSASign 是一个 JS 运行选项参数，用于在运行 JS 代码时嵌入 jsrsasign 10.8.6 库
// 返回值:
//   - JS 运行选项
//
// Example:
// ```
// // 示意性示例，需要有效的 PEM 公钥
// _, value = js.Run(`KEYUTIL.getKey(pemPublicKey).encrypt("yaklang")`, js.libJSRSASign())~
// println(value.String())
// ```
func _libJSRSASign() jsRunOpts {
	var (
		opt jsRunOpts
		ok  bool
	)

	if opt, ok = jsRunOptsCache.Get("libJSRSASign"); !ok {
		src, _ := embed.Asset("data/js-libs/jsrsasign/10.8.6/jsrsasign-all-min.js.gz")
		prog, _ := goja.Compile("jsrsasign-10.8.6", string(src), false)
		opt = jsRunWithLibs(&jsLibrary{prog, "jsrsasign", "10.8.6"})
		jsRunOptsCache.Set("libJSRSASign", opt)
	}
	return opt
}

// libJsEncrypt 是一个 JS 运行选项参数，用于在运行 JS 代码时嵌入 JSEncrypt 3.3.2 库
// 返回值:
//   - JS 运行选项
//
// Example:
// ```
// _, value = js.Run("var encrypt = new JSEncrypt(); 'ok';", js.libJsEncrypt())~
// assert value.String() == "ok", "JSEncrypt lib should be embedded successfully"
// ```
func _libJsEncrypt() jsRunOpts {
	var (
		opt jsRunOpts
		ok  bool
	)
	if opt, ok = jsRunOptsCache.Get("libJsEncrypt"); !ok {
		src, _ := embed.Asset("data/js-libs/jsencrypt/3.3.2/jsencrypt.min.js.gz")
		prog, _ := goja.Compile("jsencrypt-3.3.2", string(src), false)
		opt = jsRunWithLibs(&jsLibrary{prog, "jsencrypt", "3.3.2"})
		jsRunOptsCache.Set("libJsEncrypt", opt)
	}
	return opt
}

// Parse 对传入的 JS 代码进行解析并返回 AST 语法树
// 参数:
//   - code: JS 源代码字符串
//
// 返回值:
//   - 解析得到的 AST 程序节点
//   - 错误信息
//
// Example:
// ```
// tree = js.Parse(`function add(a, b) { return a + b; }`)~
// assert tree != nil, "parse should return a non-nil AST"
// println(js.GetSTType(tree))   // OUT: Program
// ```
func _Parse(code string) (*ast.Program, error) {
	JSast, err := parser.ParseFile(nil, "", code, 0)
	if err != nil {
		return JSast, err
	}
	return JSast, nil
}

// New 创建一个新的 JS 引擎（goja Runtime）并返回
// 参数:
//   - opts: 零个或多个运行选项，如嵌入第三方库或预置变量
//
// 返回值:
//   - JS 引擎对象，可调用 RunString 执行 JS 代码
//
// Example:
// ```
// engine = js.New()
// val = engine.RunString("1+1")~.ToInteger()
// println(val)   // OUT: 2
// assert val == 2, "js engine should evaluate 1+1 to 2"
// ```
func _jsNewEngine(opts ...jsRunOpts) *goja.Runtime {
	config := newJsRunConfig()
	for _, opt := range opts {
		opt(config)
	}

	vm := goja.New()
	for k, v := range config.variables {
		vm.Set(k, v)
	}

	// enable require function and console and buffer module
	new(require.Registry).Enable(vm)
	// use custom printer
	console.Enable(vm)
	buffer.Enable(vm)
	// set crypto module for CryptoJS
	vm.Set(
		"crypto", map[string]any{
			"getRandomValues": getRandomValues,
		},
	)

	for _, lib := range config.libs {
		_, err := vm.RunProgram(lib.program)
		if err != nil {
			log.Errorf("run js lib[%s] error: %v", lib.name, err)
			return vm
		}
	}

	return vm
}

// Run 创建新的 JS 引擎并运行传入代码，返回引擎、运行结果值和错误
// 会尝试自动导入代码中用到的库（CryptoJS 默认导入 V4 版本）
// 参数:
//   - src: 要运行的 JS 代码字符串
//   - opts: 零个或多个运行选项，如嵌入第三方库或预置变量
//
// 返回值:
//   - JS 引擎对象
//   - 运行结果值
//   - 错误信息
//
// Example:
// ```
// _, value = js.Run("1+1")~
// println(value.ToInteger())   // OUT: 2
// assert value.ToInteger() == 2, "js.Run should evaluate 1+1 to 2"
// ```
func _run(src any, opts ...jsRunOpts) (*goja.Runtime, goja.Value, error) {
	code := utils.InterfaceToString(src)
	opts = append(opts, autoImportLib(code)...)
	vm := _jsNewEngine(opts...)

	value, err := vm.RunString(code)
	return vm, value, err
}

// CallFunctionFromCode 从传入代码中调用指定的 JS 函数并返回结果
// 参数:
//   - src: 包含 JS 代码的字符串
//   - funcName: 要调用的 JS 函数名
//   - params: 零个或多个函数参数
//
// 返回值:
//   - 函数调用的返回值
//   - 错误信息
//
// Example:
// ```
// value = js.CallFunctionFromCode(`function add(a, b) { return a + b; }`, "add", 1, 2)~
// println(value.ToInteger())   // OUT: 3
// assert value.ToInteger() == 3, "add(1,2) should be 3"
// ```
func _jsCallFuncFromCode(src any, funcName string, params ...interface{}) (goja.Value, error) {
	vm := _jsNewEngine()
	prog, err := goja.Compile("", utils.InterfaceToString(src), false)
	if err != nil {
		return goja.Undefined(), err
	}
	_, err = vm.RunProgram(prog)
	if err != nil {
		return goja.Undefined(), err
	}
	v := vm.Get(funcName)

	if f, ok := goja.AssertFunction(v); !ok {
		return goja.Undefined(), utils.Errorf("[%v] is not a valid js function", funcName)
	} else {
		vmParams := make([]goja.Value, len(params))
		for i, p := range params {
			vmParams[i] = vm.ToValue(p)
		}
		return f(goja.Undefined(), vmParams...)
	}
}

// GetObjectFunction 从 JS 引擎中取出某个对象的方法并转换为可调用函数
// 参数:
//   - vm: JS 引擎
//   - thisName: 对象名
//   - funcName: 方法名
//
// 返回值:
//   - 可调用的 JS 函数
//   - 是否成功获取
//
// Example:
// ```
// vm, _ = js.Run(`a = { d: 3, add(a) {return this.d+a} }`)~
// add, ok = js.GetObjectFunction(vm, "a", "add")
// assert ok, "should get object method add"
// assert add(1)~.ToInteger() == 4, "a.add(1) should be 4"
// ```
func _getObjectFunction(vm *goja.Runtime, thisName, funcName string) (jsFunction, bool) {
	this := vm.Get(thisName)
	obj := this.ToObject(vm)
	if obj == nil {
		return nil, false
	}
	value := obj.Get(funcName)
	if utils.IsNil(value) {
		return nil, false
	}

	return _toObjectFunction(vm, this, value)
}

// GetFunction 从 JS 引擎中取出某个全局函数并转换为可调用函数
// 参数:
//   - vm: JS 引擎
//   - funcName: 函数名
//
// 返回值:
//   - 可调用的 JS 函数
//   - 是否成功获取
//
// Example:
// ```
// vm, _ = js.Run(`function sum(a, b) {return a+b;}`)~
// sum, ok = js.GetFunction(vm, "sum")
// assert ok, "should get function sum"
// assert sum(2, 3)~.ToInteger() == 5, "sum(2,3) should be 5"
// ```
func _getFunction(vm *goja.Runtime, funcName string) (jsFunction, bool) {
	return _toObjectFunction(vm, goja.Undefined(), vm.Get(funcName))
}

func _toObjectFunction(vm *goja.Runtime, this, value goja.Value) (jsFunction, bool) {
	callable, ok := goja.AssertFunction(value)
	if !ok {
		return nil, false
	}
	return func(v ...any) (goja.Value, error) {
		values := lo.Map(v, func(i any, _ int) goja.Value {
			return vm.ToValue(i)
		})
		return callable(this, values...)
	}, true
}

func autoImportLib(code string) (opts []jsRunOpts) {
	if strings.Contains(code, "CryptoJS()") || strings.Contains(code, "CryptoJS.") {
		opts = append(opts, _libCryptoJSV4())
	}
	if strings.Contains(code, "KEYUTIL()") || strings.Contains(code, "KEYUTIL.") || strings.Contains(code, "KJUR.") {
		opts = append(opts, _libJSRSASign())
	}
	if strings.Contains(code, "JSEncrypt()") {
		opts = append(opts, _libJsEncrypt())
	}
	return
}

func getRandomValues(call goja.FunctionCall, runtime *goja.Runtime) goja.Value {
	arg := call.Argument(0)
	refArg := reflect.ValueOf(arg.Export())
	refArgType := refArg.Type()
	refArgKind := refArgType.Kind()
	obj := arg.ToObject(runtime)
	if refArgKind != reflect.Slice && refArgKind == reflect.Array {
		return runtime.NewGoError(utils.Error("crypto.getRandomValues first arg type not valid"))
	}
	elemType := refArgType.Elem()
	if bigInt, ok := bigMaxIntMap[elemType.Kind()]; !ok {
		return runtime.NewGoError(utils.Error("crypto.getRandomValues first arg type not valid"))
	} else {
		for i := int64(0); i < int64(refArg.Len()); i++ {
			nBig, err := rand.Int(rand.Reader, bigInt)
			if err != nil {
				return runtime.NewGoError(err)
			}
			err = obj.Set(strconv.FormatInt(i, 10), uint32(nBig.Int64()))
			if err != nil {
				return runtime.NewGoError(err)
			}
		}
		return arg
	}
}
