package ssa

func (p *Program) GetExportType(name string) (Type, bool) {
	t, ok := p.ExportType[name]
	return t, ok
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

func (p *Program) SetExportFunction(name string, v *Function) {
	importFunction(name, v, p.ExportFunc)
}
func (p *Program) SetExportFunctions(name string, v Functions) {
	for _, function := range v {
		importFunction(name, function, p.ExportFunc)
	}
}
