package ssa

import "github.com/yaklang/yaklang/common/utils"

func (p *Program) GenerateVirtualLib(packagePath string) (*Program, error) {
	app := p.GetApplication()
	lib := app.NewLibrary(packagePath, []string{})
	lib.PkgName = packagePath
	lib.VirtualImport = true
	lib.GetAndCreateFunctionBuilder(packagePath, string(VirtualFunctionName))
	_, err := app.checkImportRelationship(lib)
	return lib, err
}

func getOrFakeImportValue(lib *Program, name string, isFunc bool) (val Value) {
	builder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(VirtualFunctionName))
	defer func() {
		if utils.IsNil(val) {
			if isFunc {
				_func := builder.NewFunc(name)
				_func.SetType(NewFunctionType(name, []Type{}, nil, false))
				lib.SetExportFunction(name, _func)
				val = _func
			} else {
				val = builder.EmitUndefined(name)
				lib.SetExportValue(name, val)
			}
		}
		if isFunc {
			if b, ok := ToBasicType(val.GetType()); ok {
				packagename := lib.PkgName
				if packagename == "" {
					packagename = "main"
				}
				t := NewBasicType(b.Kind, b.GetName())
				t.AddFullTypeName(packagename)
				val.SetType(t)
			}
		}
	}()
	if isFunc {
		if functions, ok := lib.ExportFunc[name]; ok {
			val = functions[0]
		}
	} else {
		if v, ok := lib.ExportValue[name]; ok {
			val = v
		}
	}
	return val
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
