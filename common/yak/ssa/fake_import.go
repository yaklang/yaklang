package ssa

import (
	"strings"
)

func (p *Program) GenerateVirtualLib(packagePath string) (*Program, error) {
	app := p.GetApplication()
	lib := app.NewLibrary(packagePath, []string{})
	// The canonical @virtual function stays so LazyBuild still has an entry.
	// Importers do not emit on it; each materialization owns a private function.
	lib.GetAndCreateFunctionBuilder(packagePath, string(VirtualFunctionName))
	_, err := app.checkImportRelationship(lib)
	return lib, err
}

// withVirtualBuilder runs fn on this library's single @virtual builder.
// virtualEmitMu covers that builder's cursor and is not reentrant. fn must
// not call withVirtualBuilder or GetAndCreateFunctionBuilder for @virtual
// on this library. storeField does not take virtualEmitMu; it takes the
// function's emitMu, which the same goroutine may reenter.
func (lib *Program) withVirtualBuilder(fn func(*FunctionBuilder)) {
	if lib == nil || fn == nil {
		return
	}
	lib.virtualEmitMu.Lock()
	defer lib.virtualEmitMu.Unlock()
	lib.builderMu.Lock()
	builder := lib.getAndCreateFunctionBuilder(lib.PkgName, string(VirtualFunctionName))
	lib.builderMu.Unlock()
	if builder != nil && builder.Function != nil {
		unlock := builder.Function.lockEmit()
		defer unlock()
	}
	fn(builder)
}

func (lib *Program) exportType(name string) (Type, bool) {
	if lib == nil {
		return nil, false
	}
	lib.initConcurrentMaps()
	t, ok := lib.ExportType.Get(name)
	return t, ok && t != nil
}

func (lib *Program) rememberExportType(name string, t Type) Type {
	lib.initConcurrentMaps()
	var stored Type
	lib.ExportType.Update(name, func(old Type, loaded bool) (Type, bool) {
		if loaded && old != nil {
			stored = old
			return old, false
		}
		stored = t
		return t, true
	})
	return stored
}

func (lib *Program) exportValue(name string) (Value, bool) {
	if lib == nil {
		return nil, false
	}
	lib.initConcurrentMaps()
	v, ok := lib.ExportValue.Get(name)
	return v, ok && v != nil
}

func (lib *Program) rememberExportValue(name string, v Value) Value {
	lib.initConcurrentMaps()
	var stored Value
	lib.ExportValue.Update(name, func(old Value, loaded bool) (Value, bool) {
		if loaded && old != nil {
			stored = old
			return old, false
		}
		stored = v
		return v, true
	})
	return stored
}

func fakeGetValue(lib *Program, name string) Value {
	if value, ok := lib.exportValue(name); ok {
		return value
	}
	return nil
}

func fakeGetType(lib *Program, name string, token ...CanStartStopToken) Type {
	v, _, _ := lib.virtualImportGroup.Do("type:"+name, func() (any, error) {
		if t, ok := lib.exportType(name); ok {
			return t, nil
		}
		if !lib.IsVirtualImport() {
			return nil, nil
		}
		var blueprint *Blueprint
		lib.withVirtualBuilder(func(builder *FunctionBuilder) {
			blueprint = builder.CreateBlueprint(name, token...)
		})
		return lib.rememberExportType(name, blueprint), nil
	})
	if v == nil {
		return nil
	}
	typ, _ := v.(Type)
	return typ
}

func fakeImportValue(lib *Program, name string) Value {
	v, _, _ := lib.virtualImportGroup.Do("value:"+name, func() (any, error) {
		if value, ok := lib.exportValue(name); ok {
			return value, nil
		}
		if !lib.IsVirtualImport() {
			return nil, nil
		}
		var val Value
		lib.withVirtualBuilder(func(builder *FunctionBuilder) {
			val = builder.EmitUndefined(name)
		})
		if b, ok := ToBasicType(val.GetType()); ok {
			packagename := lib.PkgName
			if packagename == "" {
				packagename = "main"
			}
			t := NewBasicType(b.Kind, b.GetName())
			t.AddFullTypeName(packagename)
			val.SetType(t)
		}
		return lib.rememberExportValue(name, val), nil
	})
	if v == nil {
		return nil
	}
	val, _ := v.(Value)
	return val
}

func fakeImportType(lib *Program, name string, token CanStartStopToken) Type {
	v, _, _ := lib.virtualImportGroup.Do("type:"+name, func() (any, error) {
		candidates := []string{name}
		if strings.Contains(name, ".") {
			candidates = append(candidates, strings.ReplaceAll(name, ".", "$"))
		}
		for _, candidate := range candidates {
			if t, ok := lib.exportType(candidate); ok {
				return t, nil
			}
			if lib.Blueprint != nil {
				if t, ok := lib.Blueprint.Get(candidate); ok && t != nil {
					return lib.rememberExportType(candidate, t), nil
				}
			}
		}
		if !lib.IsVirtualImport() {
			return nil, nil
		}
		// Shared @virtual builder. The lock is virtualEmitMu, not the
		// magic-method map: GetMagicMethod must not take it.
		var bluePrint *Blueprint
		lib.withVirtualBuilder(func(builder *FunctionBuilder) {
			builder.SetEditor(lib.GetCurrentEditor())
			bluePrint = builder.CreateBlueprint(name, token)
		})
		return lib.rememberExportType(name, bluePrint), nil
	})
	if v == nil {
		return nil
	}
	typ, _ := v.(Type)
	return typ
}
