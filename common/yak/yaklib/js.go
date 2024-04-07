package yaklib

// otto
import (
	"github.com/yaklang/yaklang/common/javascript"
	"github.com/yaklang/yaklang/common/javascript/otto"
	"github.com/yaklang/yaklang/common/javascript/otto/ast"
	"github.com/yaklang/yaklang/common/javascript/otto/parser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/embed"
)

var JSOttoExports = map[string]interface{}{
	"PoweredBy":            "github.com/robertkrimen/otto",
	"New":                  _jsNewEngine,
	"Run":                  _run,
	"CallFunctionFromCode": _jsCallFuncFromCode,

	// RunOptions
	"libCryptoJSV3": _libCryptoJSV3,

	// AST
	"ASTWalk":   javascript.BasicJavaScriptASTWalker,
	"Parse":     _Parse,
	"GetSTType": javascript.GetStatementType,

	// Value
	"NullValue":      otto.NullValue,
	"UndefinedValue": otto.UndefinedValue,
	"FalseValue":     otto.FalseValue,
	"ToValue":        otto.ToValue,
	"NaNValue":       otto.NaNValue,
	"TrueValue":      otto.TrueValue,
}

type JsRunConfig struct {
	libs []string
}

func newJsRunConfig() *JsRunConfig {
	return &JsRunConfig{}
}

type jsRunOpts func(*JsRunConfig)

func jsRunWithLibs(libs ...string) jsRunOpts {
	return func(c *JsRunConfig) {
		c.libs = append(c.libs, libs...)
	}
}

// libCryptoJSV3 是一个JS运行选项参数，用于在运行JS代码时嵌入CryptoJS 3.3.0库
// Example:
// ```
// _, value = js.Run(`CryptoJS.HmacSHA256("Message", "secret").toString();`, js.libCryptoJSV3())~
// println(value.String())
// ```
func _libCryptoJSV3() jsRunOpts {
	src, _ := embed.Asset("data/js-libs/cryptojs/3.3.0/cryptojs.min.js")
	return jsRunWithLibs(string(src))
}

// libJSRSASign 是一个JS运行选项参数，用于在运行JS代码时嵌入jsrsasign 10.8.6库
// Example:
// ```
// _, value = js.Run(`KEYUTIL.getKey(pemPublicKey).encrypt("yaklang")`, js.libJSRSASign())~
// println(value.String())
// ```
func _libJSRSASign() jsRunOpts {
	src, _ := embed.Asset("data/js-libs/jsrsasign/10.8.6/jsrsasign-all-min.js")
	return jsRunWithLibs(string(src))
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
// val = engine.Eval("1+1")~.ToInteger()~
// println(val)
// ```
func _jsNewEngine() *otto.Otto {
	return otto.New()
}

// Run 创建新的JS引擎并运行传入的代码并返回JS引擎结构体引用，运行值和错误
// 第一个参数为运行的代码字符串
// 后续参数为零个到多个运行选项，用于对此次运行进行配置，例如嵌入常用的JS第三方库等
// Example:
// ```
// _, value = js.Run(`CryptoJS.HmacSHA256("Message", "secret").toString();`, js.libCryptoJSV3())~
// println(value.String())
// ```
func _run(src any, opts ...jsRunOpts) (*otto.Otto, otto.Value, error) {
	config := newJsRunConfig()
	for _, opt := range opts {
		opt(config)
	}
	vm := _jsNewEngine()
	for _, src := range config.libs {
		_, err := vm.Run(src)
		if err != nil {
			return vm, otto.UndefinedValue(), err
		}
	}

	value, err := vm.Run(src)
	return vm, value, err
}

// CallFunctionFromCode 从传入的代码中调用指定的JS函数并返回调用结果
// 它的第一个参数为包含JS代码的字符串
// 第二个参数为要调用的JS函数名
// 后续参数为零个到多个函数参数
// Example:
// ```
// value = js.CallFunctionFromCode(`function add(a, b) { return a + b; }`, "add", 1, 2)~
// println(value.String())
// ```
func _jsCallFuncFromCode(src any, funcName string, params ...interface{}) (otto.Value, error) {
	vm := _jsNewEngine()
	script, err := vm.Compile("", src)
	if err != nil {
		return otto.UndefinedValue(), err
	}
	_, err = vm.Run(script)
	if err != nil {
		return otto.UndefinedValue(), err
	}
	vName, err := vm.Get(funcName)
	if err != nil {
		return otto.UndefinedValue(), err
	}

	if !vName.IsFunction() {
		return otto.UndefinedValue(), utils.Errorf("[%v] is not a valid js function", funcName)
	}

	return vName.Call(otto.UndefinedValue(), params...)
}
