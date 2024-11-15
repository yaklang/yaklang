package ssa

func (p *Program) GenerateVirtualLib(packagePath string) (*Program, error) {
	lib := p.NewLibrary(packagePath, []string{})
	lib.PkgName = packagePath
	lib.GetAndCreateFunctionBuilder(packagePath, string(VirtualFunctionName))
	_, err := p.checkImportRelationship(lib)
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
		return bluePrint
	} else {
		return t
	}
}
