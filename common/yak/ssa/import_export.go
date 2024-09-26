package ssa

import "github.com/yaklang/yaklang/common/utils"

func (p *Program) GetExportType(name string) Type {
	if t, ok := p.ExportType[name]; ok {
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
func (p *Program) getImportValue(name string) Value {
	if p.importType == nil {
		return nil
	}
	if v, ok := p.importValue[name]; ok {
		return v
	}
	return nil
}

func (p *Program) setImportType(name string, t Type) {
	if p.importType == nil {
		p.importType = make(map[string]Type)
	}
	p.importType[name] = t
}

func (p *Program) setImportValue(name string, v Value) {
	if p.importValue == nil {
		p.importValue = make(map[string]Value)
	}
	p.importValue[name] = v
}

func (p *Program) ImportType(lib *Program, name string) (Type, error) {
	if err := p.checkImportRelationship(lib); err != nil {
		return nil, err
	}
	t, ok := lib.ExportType[name]
	if !ok {
		return nil, utils.Errorf("library %s not contain type %s", lib.Name, name)
	}
	p.setImportType(name, t)
	return t, nil
}

func (p *Program) ImportValue(lib *Program, name string) (Value, error) {
	if err := p.checkImportRelationship(lib); err != nil {
		return nil, err
	}

	// get value
	v, ok := lib.ExportValue[name]
	if !ok {
		return nil, utils.Errorf("library %s not contain value %s", lib.Name, name)
	}

	p.setImportValue(name, v)
	return v, nil
}

func (p *Program) ImportAll(lib *Program) error {
	if err := p.checkImportRelationship(lib); err != nil {
		return err
	}

	for name, v := range p.ExportValue {
		p.setImportValue(name, v)
	}
	for name, t := range p.ExportType {
		p.setImportType(name, t)
	}

	return nil
}
