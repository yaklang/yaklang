package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

func (p *Program) GetExportType(name string) Type {
	if t, ok := p.externType[name]; ok {
		return t
	}
	return nil
}

func (p *Program) GetExportValue(name string) Value {
	if v, ok := p.ExportValue[name]; ok {
		return v
	}
	return nil
}

func (p *Program) SetExportType(name string, t Type) {
	if p.ExportType == nil {
		p.ExportType = make(map[string]Type)
	}
	p.ExportType[name] = t
}

func (p *Program) SetExportValue(name string, v Value) {
	if p.ExportValue == nil {
		p.ExportValue = make(map[string]Value)
	}
	p.ExportValue[name] = v
}

func (p *Program) checkImportRelationship(lib *Program) error {
	currentLib, ok := p.UpStream[lib.Name]
	if ok {
		if currentLib != lib {
			return utils.Errorf("program library not contain this program")
		}
	} else {
		p.UpStream[lib.Name] = lib
	}
	return nil
}

func (p *Program) setImportType(name, path string, t Type) {
	if p.importType == nil {
		p.importType = make(map[string]map[string]Type)
	}
	if p.importType[path] == nil {
		p.importType[path] = make(map[string]Type)
	}
	p.importType[path][name] = t
}
func (p *Program) setImportValue(name, path string, v Value) {
	if p.importValue == nil {
		p.importValue = make(map[string]map[string]Value)
	}
	if p.importValue[path] == nil {
		p.importValue[path] = make(map[string]Value)
	}
	p.importValue[path][name] = v
}
func (p *Program) ImportTypeFromLib(lib *Program, name string) (Type, error) {
	if err := p.checkImportRelationship(lib); err != nil {
		return nil, err
	}
	if t, ok := lib.ExportType[name]; !ok {
		return nil, utils.Errorf("library %s not contain value %s", lib.Name, name)
	} else {
		p.setImportType(name, lib.pkgName, t)
		if p.ImportValueCallback != nil {
			p.ImportTypeCallback(name, lib.pkgName, t, p)
		}
		return t, nil
	}
}

func (p *Program) ImportValueFromLib(lib *Program, name string) (Value, error) {
	if err := p.checkImportRelationship(lib); err != nil {
		return nil, err
	}
	// get value
	v, ok := lib.ExportValue[name]
	if !ok {
		return nil, utils.Errorf("library %s not contain value %s", lib.Name, name)
	}
	p.setImportValue(name, lib.pkgName, v)
	if p.ImportValueCallback != nil {
		p.ImportValueCallback(name, lib.pkgName, v, p)
	}
	return v, nil
}

func (p *Program) ImportAll(lib *Program) error {
	if err := p.checkImportRelationship(lib); err != nil {
		return err
	}
	for name, v := range lib.ExportValue {
		p.setImportValue(name, lib.pkgName, v)
		if p.ImportValueCallback != nil {
			p.ImportValueCallback(name, lib.pkgName, v, p)
		}
	}
	for name, t := range lib.ExportType {
		p.setImportType(name, lib.pkgName, t)
		if p.ImportValueCallback != nil {
			p.ImportTypeCallback(name, lib.pkgName, t, p)
		}
	}
	return nil
}
