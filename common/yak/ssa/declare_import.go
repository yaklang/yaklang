package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"golang.org/x/exp/maps"
)

type importDeclareItem struct {
	pkgName      string
	aliasPkgName string
	// all          bool
	typ   map[string]Type
	val   map[string]Value
	_func map[string]Functions
}

func importFunction(name string, _func *Function, table map[string]Functions) {
	if table == nil {
		table = make(map[string]Functions)
	}
	functions, exit := table[name]
	if !exit {
		table[name] = Functions{_func}
	} else {
		if !functions.CheckFunctionExitByHash(_func.hash) {
			functions = append(functions, _func)
		}
	}
}

func readImportDecl(p *Program, get func(*importDeclareItem) bool) {

	len := p.importDeclares.Len()
	idecl := p.importDeclares
	if p.importCoverInner {
		// read from last to first
		for i := len - 1; i >= 0; i-- {
			item, ok := idecl.GetByIndex(i)
			if ok && get(item) {
				return // stop
			}
		}
	} else {
		// read from first to last
		for i := 0; i < len; i++ {
			item, ok := idecl.GetByIndex(i)
			if ok && get(item) {
				return // stop
			}
		}
	}

}

/// ===================================== read import

func (p *Program) ReadImportType(name string) (Type, bool) {
	var ret Type = nil
	readImportDecl(p, func(idi *importDeclareItem) bool {
		t, ok := idi.typ[name]
		ret = t
		return ok
	})
	return ret, ret != nil
}

func (p *Program) ReadImportValue(name string) (Value, bool) {
	var ret Value = nil
	readImportDecl(p, func(idi *importDeclareItem) bool {
		v, ok := idi.val[name]
		ret = v
		return ok
	})
	return ret, ret != nil
}

func (p *Program) ReadImportTypeWithPkg(pkgName, name string) (Type, bool) {
	if imp, ok := p.importDeclares.Get(pkgName); ok {
		typ, ok := imp.typ[name]
		return typ, ok
	} else {
		return nil, false
	}
}

func (p *Program) ReadImportValueWithPkg(pkgName, name string) (Value, bool) {
	if imp, ok := p.importDeclares.Get(pkgName); ok {
		val, ok := imp.val[name]
		return val, ok
	} else {
		return nil, false
	}
}

/// ===================================== import

func (p *Program) checkImportRelationship(lib *Program) (*importDeclareItem, error) {
	currentLib, ok := p.UpStream.Get(lib.Name)
	if ok {
		if currentLib != lib {
			return nil, utils.Errorf("program library not contain this program")
		}
	} else {
		p.UpStream.Set(lib.Name, lib)
	}
	importDecl := p.importDeclares
	if importDecl == nil {
		importDecl = omap.NewOrderedMap[string, *importDeclareItem](nil)
		p.importDeclares = importDecl
	}
	pkg, ok := importDecl.Get(lib.Name)
	if !ok {
		pkg = &importDeclareItem{
			pkgName: lib.Name,
			typ:     make(map[string]Type),
			val:     make(map[string]Value),
			_func:   make(map[string]Functions),
		}
		importDecl.Set(lib.Name, pkg)
	}
	return pkg, nil
}

func (p *Program) ImportTypeFromLib(lib *Program, names ...string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	for _, name := range names {
		importType := fakeImportType(lib, name)
		pkg.typ[name] = importType
	}
	return err
}

func (p *Program) ImportValueFromLib(lib *Program, names ...string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	for _, name := range names {
		value := getOrFakeImportValue(lib, name, false)
		pkg.val[name] = value
	}
	return err
}
func (p *Program) ImportFunctionFromLib(lib *Program, names ...string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	for _, name := range names {
		_func := getOrFakeImportValue(lib, name, true)
		if function, b := ToFunction(_func); b {
			importFunction(name, function, pkg._func)
		}
	}
	return nil
}
func (p *Program) ImportTypeStaticAll(lib *Program, classname string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	t, ok := lib.ExportType[classname]
	if !ok {
		return utils.Errorf("library %s not contain type: %s", lib.Name, classname)
	}
	blueprint, b := ToClassBluePrintType(t)
	if !b {
		return utils.Errorf("no support to blueprint")
	}
	p.fixImportCallback = append(p.fixImportCallback, func() {
		//fix
		for s, value := range blueprint.StaticMember {
			pkg.val[s] = value
		}
		for s, functions := range blueprint.StaticMethod {
			pkg._func[s] = functions
		}
	})
	for s, value := range blueprint.StaticMember {
		pkg.val[s] = value
	}

	for s, functions := range blueprint.StaticMethod {
		for _, f := range functions {
			importFunction(s, f, pkg._func)
		}
	}
	return nil
}
func (p *Program) ImportTypeStaticMemberFromLib(lib *Program, clsName string, names ...string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	build := func(blueprint *Blueprint, name string) {
		p.fixImportCallback = append(p.fixImportCallback, func() {
			for s, value := range blueprint.StaticMember {
				if name == s {
					pkg.val[s] = value
				}
			}
			for s, function := range blueprint.StaticMethod {
				if name == s {
					pkg._func[s] = function
				}
			}
		})
		for s, value := range blueprint.StaticMember {
			if name == s {
				pkg.val[s] = value
			}
		}
		for s, function := range blueprint.StaticMethod {
			if name == s {
				pkg._func[s] = function
			}
		}
	}
	if v, ok := lib.ExportType[clsName]; !ok {
		err = utils.JoinErrors(err, utils.Errorf("library %s not contain type %s", lib.Name, clsName))
		return err
	} else {
		blueprint, b := ToClassBluePrintType(v)
		if !b {
			errx := utils.Errorf("no support other type")
			return errx
		}
		for _, name := range names {
			build(blueprint, name)
		}
	}
	return nil
}

func (p *Program) ImportAll(lib *Program) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	maps.Copy(pkg.typ, lib.externType)
	maps.Copy(pkg.val, lib.ExportValue)
	_ = pkg
	return nil
}
