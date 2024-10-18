package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type importDeclareItem struct {
	pkgName      string
	aliasPkgName string
	// all          bool
	typ map[string]Type
	val map[string]Value
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
	currentLib, ok := p.UpStream[lib.Name]
	if ok {
		if currentLib != lib {
			return nil, utils.Errorf("program library not contain this program")
		}
	} else {
		p.UpStream[lib.Name] = lib
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
		t, ok := lib.GetExportType(name)
		if !ok {
			err = utils.JoinErrors(err, utils.Errorf("library %s not contain value %s", lib.Name, name))
			continue
		}
		pkg.typ[name] = t
	}
	return err
}

func (p *Program) ImportValueFromLib(lib *Program, names ...string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	for _, name := range names {
		// get value
		v, ok := lib.ExportValue[name]
		if !ok {
			err = utils.JoinErrors(err, utils.Errorf("library %s not contain value %s", lib.Name, name))
		}
		pkg.val[name] = v
	}
	return err
}

func (p *Program) ImportAll(lib *Program) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	// pkg.all = true
	_ = pkg
	return nil
}
