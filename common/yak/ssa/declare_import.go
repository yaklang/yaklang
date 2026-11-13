package ssa

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"golang.org/x/exp/maps"
)

type importDeclareItem struct {
	pkgName      string
	aliasPkgName string

	// mu guards typ and val. Prehandler goroutines import into one item while
	// queries read it, so the maps need their own lock on top of the ordered
	// index that stores the item.
	mu sync.RWMutex
	// all          bool
	typ map[string]Type
	val map[string]Value
}

func (i *importDeclareItem) getType(name string) (Type, bool) {
	if i == nil {
		return nil, false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	t, ok := i.typ[name]
	return t, ok
}

func (i *importDeclareItem) setType(name string, t Type) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.typ[name] = t
}

func (i *importDeclareItem) getValue(name string) (Value, bool) {
	if i == nil {
		return nil, false
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	v, ok := i.val[name]
	return v, ok
}

func (i *importDeclareItem) setValue(name string, v Value) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.val[name] = v
}

func (i *importDeclareItem) copyTypesFrom(src map[string]Type) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	maps.Copy(i.typ, src)
}

func (i *importDeclareItem) copyValuesFrom(src map[string]Value) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	maps.Copy(i.val, src)
}

// appendFixImportCallback registers one deferred repair. Callbacks are
// appended by import functions on prehandler goroutines and drained later.
func (p *Program) appendFixImportCallback(fn func()) {
	if p == nil || fn == nil {
		return
	}
	p.fixImportMu.Lock()
	defer p.fixImportMu.Unlock()
	p.fixImportCallback = append(p.fixImportCallback, fn)
}

// snapshotFixImportCallbacks copies the callback list so a drain never holds
// the lock while callbacks run and import functions append more work.
func (p *Program) snapshotFixImportCallbacks() []func() {
	if p == nil {
		return nil
	}
	p.fixImportMu.RLock()
	defer p.fixImportMu.RUnlock()
	callbacks := make([]func(), len(p.fixImportCallback))
	copy(callbacks, p.fixImportCallback)
	return callbacks
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
		t, ok := idi.getType(name)
		ret = t
		return ok
	})
	return ret, ret != nil
}

func (p *Program) TryReadImportDeclare(name string) (Type, bool) {
	var ret Type = nil
	readImportDecl(p, func(idi *importDeclareItem) bool {
		t, ok := idi.getType(name)
		ret = t
		if ret == nil {
			for libName := range p.importDeclares.SnapshotMap() {
				lib, exist := p.GetLibrary(libName)
				if lib != nil && exist {
					ret, ok = lib.GetExportType(name)
				}
				if ok && ret != nil {
					break
				}
				lib.GlobalVariablesBlueprint.Build()
				if _, ok = lib.GetGlobalVariable(name); ok {
					ret = lib.GlobalVariablesBlueprint
					return true
				}
			}

		}
		return ok
	})
	return ret, ret != nil
}

func (p *Program) ReadImportValue(name string) (Value, bool) {
	var ret Value = nil
	readImportDecl(p, func(idi *importDeclareItem) bool {
		v, ok := idi.getValue(name)
		ret = v
		return ok
	})
	return ret, ret != nil
}

func (p *Program) ReadImportTypeWithPkg(pkgName, name string) (Type, bool) {
	if imp, ok := p.importDeclares.Get(pkgName); ok {
		return imp.getType(name)
	}
	return nil, false
}

func (p *Program) ReadImportValueWithPkg(pkgName, name string) (Value, bool) {
	if imp, ok := p.importDeclares.Get(pkgName); ok {
		return imp.getValue(name)
	}
	return nil, false
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
		pkg = importDecl.GetOrSet(lib.Name, &importDeclareItem{
			pkgName: lib.Name,
			typ:     make(map[string]Type),
			val:     make(map[string]Value),
		})
	}
	return pkg, nil
}

func (p *Program) ImportTypeFromLib(lib *Program, name string, token CanStartStopToken) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	importType := fakeImportType(lib, name, token)
	pkg.setType(name, importType)
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx+1 < len(name) {
		pkg.setType(name[idx+1:], importType)
	}
	return err
}

func (p *Program) ImportValueFromLib(lib *Program, names ...string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	for _, name := range names {
		value := fakeImportValue(lib, name)
		pkg.setValue(name, value)
		p.appendFixImportCallback(func() {
			if real := lib.GetExportValue(name); real != nil {
				if value != nil && value != real {
					ReplaceAllValue(value, real)
				}
				pkg.setValue(name, real)
			}
		})
	}
	return err
}
func getBlueprintForImportType(lib *Program, className string) (*Blueprint, error) {
	if lib == nil {
		return nil, utils.Errorf("library is nil")
	}
	t := fakeImportType(lib, className, nil)
	if t == nil {
		return nil, utils.Errorf("library %s not contain type: %s", lib.Name, className)
	}
	blueprint, ok := ToClassBluePrintType(t)
	if !ok {
		return nil, utils.Errorf("no support to blueprint")
	}
	return blueprint, nil
}

func (p *Program) ImportTypeStaticAll(lib *Program, classname string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	blueprint, err := getBlueprintForImportType(lib, classname)
	if err != nil {
		return err
	}
	blueprint.Build()
	p.appendFixImportCallback(func() {
		//fix
		blueprint.StaticMember.ForEach(func(s string, values []Value) bool {
			if len(values) > 0 {
				pkg.setValue(s, values[len(values)-1])
			}
			return true
		})
		for s, function := range blueprint.StaticMethod {
			pkg.setValue(s, function)
		}
	})
	blueprint.StaticMember.ForEach(func(s string, values []Value) bool {
		if len(values) > 0 {
			pkg.setValue(s, values[len(values)-1])
		}
		return true
	})
	for s, function := range blueprint.StaticMethod {
		pkg.setValue(s, function)
	}
	return nil
}
func (p *Program) ImportTypeStaticMemberFromLib(lib *Program, clsName string, names ...string) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	build := func(blueprint *Blueprint, name string) {
		p.appendFixImportCallback(func() {
			blueprint.StaticMember.ForEach(func(s string, values []Value) bool {
				if name == s && len(values) > 0 {
					pkg.setValue(s, values[len(values)-1])
				}
				return true
			})
			for s, function := range blueprint.StaticMethod {
				if name == s {
					pkg.setValue(s, function)
				}
			}
		})
		blueprint.StaticMember.ForEach(func(s string, values []Value) bool {
			if name == s && len(values) > 0 {
				pkg.setValue(s, values[len(values)-1])
			}
			return true
		})
		for s, function := range blueprint.StaticMethod {
			if name == s {
				pkg.setValue(s, function)
			}
		}
	}
	blueprint, err := getBlueprintForImportType(lib, clsName)
	if err != nil {
		return utils.JoinErrors(err)
	}
	blueprint.Build()
	for _, name := range names {
		build(blueprint, name)
	}
	return nil
}

func (p *Program) ImportAll(lib *Program) error {
	pkg, err := p.checkImportRelationship(lib)
	if err != nil {
		return err
	}
	lib.initConcurrentMaps()
	pkg.copyTypesFrom(lib.snapshotExternTypes())
	pkg.copyValuesFrom(lib.ExportValue.GetAll())
	return nil
}
