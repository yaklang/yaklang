package ssa

func (p *Program) GetType(name string) Type {
	return p.getTypeEx(name, "")
}
func (p *Program) GetTypeWithPkgName(name, pkg string) Type {
	return p.getTypeEx(name, pkg)
}
func (prog *Program) getTypeEx(name, pkg string) Type {
	getType := func() (Type, bool) {
		if typ, ok := prog.externType[name]; ok {
			return typ, true
		}
		return nil, false
	}
	if typ, ok := readDeclareWithImport[Type](
		prog, pkg, name,
		prog.ReadImportTypeWithPkg,
		prog.ReadImportType,
		getType,
	); ok {
		return typ
	}
	return nil
}

func (p *Program) GetClassBlueprintEx(name string, pkg string, token ...CanStartStopToken) *Blueprint {
	if p == nil {
		return nil
	}

	getInCurrent := func() (Type, bool) {
		if blueprint, exit := p.Blueprint.Get(name); exit {
			if !p.PreHandler() {
				blueprint.Build()
			}
			return blueprint, true
		}
		return nil, false
	}

	// //TODO:  merge  this to getType
	if typ, ok := readDeclareWithImport[Type](
		p, pkg, name,
		p.ReadImportTypeWithPkg,
		p.ReadImportType,
		getInCurrent,
	); ok {
		if c, ok := typ.(*Blueprint); ok {
			if !p.PreHandler() {
				c.Build()
			}
			return c
		}
	}
	//if p.IsVirtualImport() {
	//	fakeType := fakeGetType(p, name, token...)
	//	blueprint, ok := ToClassBluePrintType(fakeType)
	//	if ok {
	//		return blueprint
	//	}
	//}
	return nil

}

func (prog *Program) GetFunctionEx(name, pkg string) *Function {
	getFunc := func() (Value, bool) {
		if fun, ok := prog.Funcs.Get(name); ok {
			if !prog.PreHandler() {
				fun.Build()
			}
			return fun, true
		}
		return nil, false
	}

	if val, ok := readDeclareWithImport[Value](prog, pkg, name,
		prog.ReadImportValueWithPkg,
		prog.ReadImportValue,
		getFunc,
	); ok {
		if fun, ok := ToFunction(val); ok {
			//todo: fix se
			if !prog.PreHandler() {
				fun.Build()
			}
			return fun
		}
	}
	return nil
}

func (p *Program) GetFunction(name string, pkg string) *Function {
	return p.GetFunctionEx(name, pkg)
}

func readDeclareWithImport[T any](
	prog *Program,
	pkg, name string,
	getImportWithPkg func(string, string) (T, bool),
	getImport func(string) (T, bool),
	getCurrent func() (T, bool),
) (T, bool) {
	var empty T
	//search lib
	if pkg != "" && pkg != prog.PkgName {
		if t, ok := getImportWithPkg(pkg, name); ok {
			return t, true
		}
		return empty, false
	}

	// search name
	if prog.importCoverCurrent {
		// first import then current
		if t, ok := getImport(name); ok {
			return t, true
		}
		if t, ok := getCurrent(); ok {
			return t, true
		}
	} else {
		// first current then import
		if t, ok := getCurrent(); ok {
			return t, true
		}
		if t, ok := getImport(name); ok {
			return t, true
		}
	}
	return empty, false
}
