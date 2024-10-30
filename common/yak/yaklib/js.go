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

func _withVariable(name string, value any) jsRunOpts {
	return func(c *JsRunConfig) {
		c.variables[name] = value
	}
}

func _withVariables(vars map[string]any) jsRunOpts {
	return func(c *JsRunConfig) {
		for k, v := range vars {
			c.variables[k] = v
		}
	}
}

var jsRunOptsCache = utils.NewTTLCache[jsRunOpts]()

// libCryptoJSV3 是一个JS运行选项参数，用于在运行JS代码时嵌入CryptoJS 3.3.0库
// Example:
// ```
// _, value = js.Run(`CryptoJS.HmacSHA256("Message", "secret").toString();`, js.libCryptoJSV3())~
// println(value.String())
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

// libCryptoJSV4 是一个JS运行选项参数，用于在运行JS代码时嵌入CryptoJS 4.2.0库
// Example:
// ```
// _, value = js.Run(`CryptoJS.HmacSHA256("Message", "secret").toString();`, js.libCryptoJSV4())~
// println(value.String())
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

// libJSRSASign 是一个JS运行选项参数，用于在运行JS代码时嵌入jsrsasign 10.8.6库
// Example:
// ```
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

// _libJsEncrypt 是一个JS运行选项参数，用于在运行JS代码时嵌入JsEncrypt 3.3.2库
// Example:
// ```
// _, value = js.Run("var encrypt = new JSEncrypt();", js._libJsEncrypt())~
// println(value.String())
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

// Parse 对传入的JS代码进行解析并返回解析后的AST树和错误
// Example:
// ```
// code = `function add(a, b) { return a + b; }`
// tree = js.Parse(code)~
// dump(tree)
// ```
func _Parse(code string) (*ast.Program, error) {
	JSast, err := parser.ParseFile(nil, "", code, 0)
	if err != nil {
		return JSast, err
	}
	return JSast, nil
}

// New 创建新的JS引擎并返回
// Example:
// ```
// engine = js.New()
// val = engine.RunString("1+1")~.ToInteger()~
// println(val)
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

// Run 创建新的JS引擎并运行传入的代码并返回JS引擎结构体引用，运行值和错误
// 第一个参数为运行的代码字符串
// 后续参数为零个到多个运行选项，用于对此次运行进行配置，例如嵌入常用的JS第三方库等
// 现在会尝试自动导入代码中使用到的库, CryptoJS会导入V4版本
// Example:
// ```
// _, value = js.Run(`CryptoJS.HmacSHA256("Message", "secret").toString();`, js.libCryptoJSV3())~
// println(value.String())
// ```
func _run(src any, opts ...jsRunOpts) (*goja.Runtime, goja.Value, error) {
	code := utils.InterfaceToString(src)
	opts = append(opts, autoImportLib(code)...)
	vm := _jsNewEngine(opts...)

	value, err := vm.RunString(code)
	return vm, value, err
}

// CallFunctionFromCode 从传入的代码中调用指定的JS函数并返回调用结果
// 第一个参数为包含JS代码的字符串
// 第二个参数为要调用的JS函数名
// 后续参数为零个到多个函数参数
// Example:
// ```
// value = js.CallFunctionFromCode(`function add(a, b) { return a + b; }`, "add", 1, 2)~
// println(value.String())
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

// GetObjectFunction 将传入的Value转换为可以调用的对象(Object)函数
// 第一个参数为JS引擎
// 第二个参数为Object名字
// 第三个参数为方法名字
// Example:
// ```
// vm, _ = js.Run(`a = {
// d: 3,
// add(a) {return this.d+a},
// }`)~
// add, ok = js.GetObjectFunction(vm, "a", "add")
// if ok {
// println(add(1).ToInteger()) // 4
// }
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

// GetFunction 将传入的Value转换为可以调用的对象(Object)函数
// 第一个参数为JS引擎
// 第二个参数为函数名字
// Example:
// ```
// vm, _ = js.Run(`function sum(a, b) {return a+b;}`)~
// sum, ok = js.GetFunction(vm,"sum")
// if ok {
// println(sum(2,3).ToInteger()) // 5
// }
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
