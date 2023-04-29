package yaklib

// otto
import (
	"yaklang/common/javascript"
	"yaklang/common/javascript/otto"
	"yaklang/common/utils"
)

var (
	JSOttoExports = map[string]interface{}{
		"PoweredBy":            "github.com/robertkrimen/otto",
		"New":                  _jsNewEngine,
		"Run":                  otto.Run,
		"CallFunctionFromCode": _jsCallFuncFromCode,
		"NullValue":            otto.NullValue,
		"UndefinedValue":       otto.UndefinedValue,
		"FalseValue":           otto.FalseValue,
		"ToValue":              otto.ToValue,
		"NaNValue":             otto.NaNValue,
		"TrueValue":            otto.TrueValue,
		"ASTWalk":              javascript.BasicJavaScriptASTWalker,
	}
)

// create vm
func _jsNewEngine() *otto.Otto {
	return otto.New()
}

func _jsCallFuncFromCode(i interface{}, funcName string, params ...interface{}) (otto.Value, error) {
	code := utils.InterfaceToString(i)
	vm := _jsNewEngine()
	script, err := vm.Compile("", code)
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
