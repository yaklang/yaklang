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
	if p.DatabaseKind == ProgramCacheMemory {
		return
	}
	ir := ssadb.MarshalFile(e)
	p.Cache.editorCache.Add(e.GetIrSourceHash(), ir)
}

func (p *Program) SaveFolder(folderPath []string) {
	if p.DatabaseKind == ProgramCacheMemory {
		return
	}
	ir := ssadb.MarshalFolder(folderPath)
	p.Cache.editorCache.Add(ir.SourceCodeHash, ir)
}
