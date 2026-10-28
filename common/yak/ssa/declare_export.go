package ssa

func (p *Program) GetExportType(name string) (Type, bool) {
	if p == nil {
		return nil, false
	}
	p.initConcurrentMaps()
	return p.ExportType.Get(name)
}

func (p *Program) GetExportValue(name string) Value {
	if p == nil {
		return nil
	}
	p.initConcurrentMaps()
	if v, ok := p.ExportValue.Get(name); ok {
		return v
	}
	return nil
}

func (p *Program) SetExportType(name string, t Type) {
	if p == nil {
		return
	}
	p.initConcurrentMaps()
	p.ExportType.Set(name, t)
}

func (p *Program) SetExportValue(name string, v Value) {
	if p == nil {
		return
	}
	p.initConcurrentMaps()
	p.ExportValue.Set(name, v)
}

func (p *Program) SetReExportInfo(reexportName, from, exportName string, isNameSpaceExport, isWildCardExport bool) {
	if p == nil {
		return
	}
	p.initConcurrentMaps()
	p.ReExportTable.Set(reexportName, &ReExportInfo{
		FilePath:          from,
		ExportName:        exportName,
		IsNameSpaceExport: isNameSpaceExport,
		IsWildCardExport:  isWildCardExport,
	})
}

func (p *Program) SetWildCardReExportInfo(from string) {
	if p == nil {
		return
	}
	p.initConcurrentMaps()
	p.ReExportTable.Set("*", &ReExportInfo{
		FilePath:          from,
		ExportName:        "*",
		IsNameSpaceExport: false,
		IsWildCardExport:  true,
	})
}

func (p *Program) GetReExportInfo(reexportName string) *ReExportInfo {
	if p == nil {
		return nil
	}
	p.initConcurrentMaps()
	if info, ok := p.ReExportTable.Get(reexportName); ok {
		return info
	}
	return nil
}

func (p *Program) GetWildCardReExportInfo() *ReExportInfo {
	if p == nil {
		return nil
	}
	p.initConcurrentMaps()
	if info, ok := p.ReExportTable.Get("*"); ok {
		return info
	}
	return nil
}
