package ssa

import (
	"path"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func (p *Program) CreateEditor(raw []byte, filepath string) *memedit.MemEditor {
	edit := memedit.NewMemEditorByBytes(raw)
	filepath = path.Join(p.GetProgramName(), filepath)
	edit.SetUrl(filepath)
	folder, file := path.Split(filepath)
	edit.SetFolderPath(folder)
	edit.SetFileName(file)
	edit.SetProgramName(p.GetProgramName())
	p.SaveEditor(edit)
	return edit
}

func (p *Program) SaveEditor(e *memedit.MemEditor) {
	ir := ssadb.MarshalFile(e)

	// 	ssadb.MarshalFile(e, folderPath)
	p.Cache.editorCache.Add("", ir)
}

func (p *Program) SaveFolder(folderPath []string) {
	ir := ssadb.MarshalFolder(p.GetProgramName(), folderPath)
	p.Cache.editorCache.Add("", ir)
}
