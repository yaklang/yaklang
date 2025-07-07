package java2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

const (
	SPRING_PATH          = "org.springframework"
	SPRING_UI_MODEL_NAME = "org.springframework.ui.Model"
)

func hookSpringBootReturn(y *singleFileBuilder, value ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	if value == nil {
		return
	}
	app := y.GetProgram().GetApplication()
	if app == nil {
		return
	}

	// check if is in controller
	if y.isInController {
		// check if is freemarker file
		path := value.String()
		fPath := app.GetProjectConfigValue("spring.freemarker.prefix") + path + app.GetProjectConfigValue("spring.freemarker.suffix")
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
		methodCall := y.ReadMemberCallMethod(fbObj, y.EmitConstInstPlaceholder(fbMethod))
		jspArgs := []ssa.Value{y.GetUIModel(), y.GetUIModel()}
		y.EmitCall(y.NewCall(methodCall, jspArgs))
	}
}

func hookSpringBootMemberCallMethod(y *singleFileBuilder, obj ssa.Value, key ssa.Value, args ...ssa.Value) {
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
