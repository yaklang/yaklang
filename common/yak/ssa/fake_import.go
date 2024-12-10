package ssa

func (p *Program) GenerateVirtualLib(packagePath string) (*Program, error) {
	app := p.GetApplication()
	lib := app.NewLibrary(packagePath, []string{})
	lib.PkgName = packagePath
	lib.VirtualImport = true
	lib.GetAndCreateFunctionBuilder(packagePath, string(VirtualFunctionName))
	_, err := app.checkImportRelationship(lib)
	return lib, err
}

func getOrFakeImportValue(lib *Program, name string, isFunc bool) Value {
	builder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(VirtualFunctionName))
	if value, ok := lib.ExportValue[name]; !ok && lib.VirtualImport {
		var val Value
		if !isFunc {
			val = builder.EmitUndefined(name)
			lib.SetExportValue(name, val)
		} else {
			_func := builder.NewFunc(name)
			_func.SetType(NewFunctionType(name, []Type{}, nil, false))
			val = _func
			lib.SetExportFunction(name, _func)
		}
		if b, ok := ToBasicType(val.GetType()); ok {
			packagename := lib.PkgName
			if packagename == "" {
				packagename = "main"
			}
			t := NewBasicType(b.Kind, b.GetName())
			t.AddFullTypeName(packagename)
			val.SetType(t)
		}
		return val
	} else {
		return value
	}
}
func fakeImportType(lib *Program, name string) Type {
	builder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(VirtualFunctionName))
	if t, ok := lib.ExportType[name]; !ok && lib.VirtualImport {
		bluePrint := builder.CreateBluePrint(name)
		lib.ExportType[name] = bluePrint
		builder.ClassConstructor(bluePrint, []Value{})
		return bluePrint
	} else {
		return t
	}
}
