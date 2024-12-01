package java2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"path/filepath"
	"strings"
)

const (
	SERVLET_PATH              = "javax.servlet"
	SERVLET_REQUEST_DISPATHCE = "getRequestDispatcher"
	SERVLET_TEMPLATE_PREFIX   = "webapp"
)

const FRAMEWORK_DEFAULT_CLASSPATH = "src/main"

func (y *builder) HookMemberCallMethod(obj ssa.Value, key ssa.Value, args ...ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	if obj == nil || key == nil {
		return
	}
	y.RegisterServletSupport(obj, key, args...)
}

func (y *builder) RegisterServletSupport(obj ssa.Value, key ssa.Value, args ...ssa.Value) {
	typ := obj.GetType()
	if typ == nil || !strings.Contains(strings.Join(typ.GetFullTypeNames(), "."), SERVLET_PATH) {
		return
	}
	app := y.GetProgram().GetApplication()
	if app == nil {
		return
	}
	if key.String() == SERVLET_REQUEST_DISPATHCE {
		if len(args) != 1 {
			return
		}
		jspPath := args[0].String()
		path := filepath.Join(FRAMEWORK_DEFAULT_CLASSPATH, SERVLET_TEMPLATE_PREFIX, jspPath)
		t := app.TryGetTemplate(path)
		if t == nil {
			return
		}
		p := app.GetSubProgram(t.GetPkgName())
		jspBlueprint := p.GetBluePrint(t.GetClassName())
		if jspBlueprint == nil {
			return
		}

		jspMethod := t.GetTemplateServerName()
		jspObj := y.EmitUndefined(t.GetClassName())
		if utils.IsNil(jspObj) {
			return
		}
		jspObj.SetType(jspBlueprint)
		methodCall := y.ReadMemberCallMethod(jspObj, y.EmitConstInst(jspMethod))
		jspArgs := []ssa.Value{obj, y.EmitConstInstNil()}
		y.EmitCall(y.NewCall(methodCall, jspArgs))
	}
}
