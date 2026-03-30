package ssa

import (
	"path"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func (p *Program) CreateEditor(raw []byte, filepath string, save ...bool) *memedit.MemEditor {
	edit := memedit.NewMemEditorByBytes(raw)
	folder, file := path.Split(filepath)

	edit.SetFolderPath(folder)
	edit.SetFileName(file)
	edit.SetProgramName(p.GetProgramName())
	if len(save) == 0 || save[0] {
		p.SaveEditor(edit)
	}
	return edit
}

func (p *Program) SaveEditor(e *memedit.MemEditor) {
	if p.DatabaseKind == ProgramCacheMemory || p.Cache == nil {
		return
	}
	hash := e.GetIrSourceHash()
	log.Debugf("SaveEditor: program=%s, path=%s, hash=%s", p.GetProgramName(), e.GetFolderPath()+e.GetFilename(), hash)
	if !p.Cache.reserveSourceHashForSave(hash) {
		return
	}
	ir := ssadb.MarshalFile(e, hash)
	p.Cache.editorCache.Add(hash, ir)
}

func (p *Program) SaveFolder(folderPath []string) {
	if p.DatabaseKind == ProgramCacheMemory || p.Cache == nil {
		return
	}
	ir := ssadb.MarshalFolder(folderPath)
	if !p.Cache.reserveSourceHashForSave(ir.SourceCodeHash) {
		return
	}
	p.Cache.editorCache.Add(ir.SourceCodeHash, ir)
}
