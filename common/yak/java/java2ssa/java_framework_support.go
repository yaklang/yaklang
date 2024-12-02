package java2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"path/filepath"
	"strings"
)

const (
	SERVLET_PATH              = "javax.servlet"
	SERVLET_REQUEST_DISPATHCE = "getRequestDispatcher"
	SERVLET_TEMPLATE_PREFIX   = "webapp"

	SPRING_PATH          = "org.springframework"
	SPRING_UI_MODEL_NAME = "org.springframework.ui.Model"
)

const FRAMEWORK_DEFAULT_CLASSPATH = "src/main"

func (y *builder) HookMemberCallMethod(obj ssa.Value, key ssa.Value, args ...ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	if obj == nil || key == nil {
		return
	}
	y.RegisterMemberCallMethodHookForServlet(obj, key, args...)
	y.RegisterMemberCallMethodHookForSpring(obj, key, args...)
}

func (y *builder) HookReturn(val ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	if val == nil {
		return
	}
	y.RegisterReturnHookForSpring(val)
}

func (y *builder) RegisterMemberCallMethodHookForServlet(obj ssa.Value, key ssa.Value, args ...ssa.Value) {
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

		var jspBlueprint *ssa.Blueprint
		if t.GetPkgName() != "" {
			p := app.GetSubProgram(t.GetPkgName())
			jspBlueprint = p.GetBluePrint(t.GetClassName())
		} else {
			jspBlueprint = app.GetBluePrint(t.GetClassName())
		}

		if jspBlueprint == nil {
			return
		}

		jspMethod := t.GetTemplateServerName()
		jspObj := y.EmitUndefined(t.GetClassName())
		jspObj.SetType(jspBlueprint)
		methodCall := y.ReadMemberCallMethod(jspObj, y.EmitConstInst(jspMethod))
		jspArgs := []ssa.Value{obj, y.EmitConstInstNil()}
		y.EmitCall(y.NewCall(methodCall, jspArgs))
	}
}

func (y *builder) RegisterMemberCallMethodHookForSpring(obj ssa.Value, key ssa.Value, args ...ssa.Value) {
	typ := obj.GetType()
	if typ == nil {
		return
	}
	typeName := strings.Join(typ.GetFullTypeNames(), ".")
	if !strings.Contains(typeName, SPRING_PATH) {
		return
	}

	app := y.GetProgram().GetApplication()
	if app == nil {
		return
	}
	if strings.Contains(typeName, SPRING_UI_MODEL_NAME) {
		y.SetUIModel(obj)
	}
}

func (y *builder) RegisterReturnHookForSpring(val ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	if val == nil {
		return
	}
	app := y.GetProgram().GetApplication()
	if app == nil {
		return
	}

	// check if is in controller
	if y.isInController {
		// check if is freemarker file
		path := val.String()
		fPath := app.GetProjectConfig("spring.freemarker.prefix") + path + app.GetProjectConfig("spring.freemarker.suffix")
		t := app.TryGetTemplate(fPath)
		if t == nil {
			return
		}

		var fBlueprint *ssa.Blueprint
		if t.GetPkgName() != "" {
			p := app.GetSubProgram(t.GetPkgName())
			fBlueprint = p.GetBluePrint(t.GetClassName())
		} else {
			fBlueprint = app.GetBluePrint(t.GetClassName())
		}
		if fBlueprint == nil {
			return
		}

		if y.GetUIModel() == nil {
			return
		}
		fbMethod := t.GetTemplateServerName()
		fbObj := y.EmitUndefined(t.GetClassName())

		fbObj.SetType(fBlueprint)
		methodCall := y.ReadMemberCallMethod(fbObj, y.EmitConstInst(fbMethod))
		jspArgs := []ssa.Value{y.GetUIModel(), y.EmitConstInstNil()}
		y.EmitCall(y.NewCall(methodCall, jspArgs))
	}
}

func (y *builder) SetUIModel(val ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	y.currentUIModel = val
}

func (y *builder) GetUIModel() ssa.Value {
	if y == nil || y.IsStop() {
		return nil
	}
	return y.currentUIModel
}

func (y *builder) ResetUIModel() {
	if y == nil || y.IsStop() {
		return
	}
	y.currentUIModel = nil
}

func isFreemarkerFile(prog *ssa.Program, path string) bool {
	if prog == nil || path == "" {
		return false
	}
	ext := filepath.Ext(path)
	if ext == "" {
		return false
	}
	if ext == ".ftl" {
		return true
	}
	configExt := prog.GetProjectConfig("spring.freemarker.suffix")
	if configExt == "" {
		return false
	}
	return ext == configExt
}
