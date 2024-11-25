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

func fakeImportValue(lib *Program, name string) Value {
	builder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(VirtualFunctionName))
	if value, ok := lib.ExportValue[name]; !ok && lib.VirtualImport {
		val := builder.EmitUndefined(name)
		lib.ExportValue[name] = val
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

		// newFunction := builder.NewFunc(name)
		// newFunction.SetMethodName(name)
		// newFunction.SetType(NewFunctionType(fmt.Sprintf("%s-__construct", name), []Type{}, nil, true))
		// bluePrint.RegisterMagicMethod(Constructor, newFunction)
		return bluePrint
	} else {
		return t
	}
}
