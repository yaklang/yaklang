package ssa

func (p *Program) GenerateVirtualLib(packagePath string) (*Program, error) {
	app := p.GetApplication()
	lib := app.NewLibrary(packagePath, []string{})
	lib.PkgName = packagePath
	lib.GetAndCreateFunctionBuilder(packagePath, string(VirtualFunctionName))
	_, err := app.checkImportRelationship(lib)
	return lib, err
}

func fakeGetValue(lib *Program, name string) Value {
	builder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(VirtualFunctionName))
	value := builder.ReadValue(name)
	return value
}
func fakeGetType(lib *Program, name string, token ...CanStartStopToken) Type {
	builder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(VirtualFunctionName))
	blueprint := builder.CreateBlueprint(name, token...)
	return blueprint
}
func fakeImportValue(lib *Program, name string) Value {
	builder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(VirtualFunctionName))
	if value, ok := lib.ExportValue[name]; !ok && lib.IsVirtualImport() {
		val := builder.EmitUndefined(name)
		lib.ExportValue[name] = val
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
	if t, ok := lib.ExportType[name]; !ok && lib.IsVirtualImport() {
		builder.SetEmptyRange()
		bluePrint := builder.CreateBlueprint(name)
		lib.ExportType[name] = bluePrint
		return bluePrint
	} else {
		return t
	}
}
