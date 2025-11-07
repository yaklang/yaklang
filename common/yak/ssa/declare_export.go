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

func (p *Program) SetReExportInfo(reexportName, from, exportName string, isNameSpaceExport, isWildCardExport bool) {
	if p.ReExportTable == nil {
		p.ReExportTable = make(map[string]*ReExportInfo)
	}
	p.ReExportTable[reexportName] = &ReExportInfo{
		FilePath:          from,
		ExportName:        exportName,
		IsNameSpaceExport: isNameSpaceExport,
		IsWildCardExport:  isWildCardExport,
	}
}

func (p *Program) SetWildCardReExportInfo(from string) {
	if p.ReExportTable == nil {
		p.ReExportTable = make(map[string]*ReExportInfo)
	}
	p.ReExportTable["*"] = &ReExportInfo{
		FilePath:          from,
		ExportName:        "*",
		IsNameSpaceExport: false,
		IsWildCardExport:  true,
	}
}

func (p *Program) GetReExportInfo(reexportName string) *ReExportInfo {
	if info, ok := p.ReExportTable[reexportName]; ok {
		return info
	}
	return nil
}

func (p *Program) GetWildCardReExportInfo() *ReExportInfo {
	if info, ok := p.ReExportTable["*"]; ok {
		return info
	}
	return nil
}
