package script_core

import "context"

// yakEngine := yaklang.New()
//
//	yakEngine.SetVars(map[string]any{
//		"params": args,
//	})
//	code := fmt.Sprintf("result = %s(params...)", methodName)
//	err := yakEngine.SafeEval(context.Background(), code)
//	if err != nil {
//		return nil, utils.Errorf("call yak method `%s` error: %v", methodName, err)
//	}
//	val, ok := yakEngine.GetVar("result")
//	if !ok {
//		return nil, nil
//	}
type YakScriptEngine interface {
	SafeEval(context.Context, string) error
	SetVars(vars map[string]any)
	GetVar(name string) (any, bool)
}

var YakScriptEngineGetter func() YakScriptEngine

func SetYakScriptEngineGetter(f func() YakScriptEngine) {
	YakScriptEngineGetter = f
}
