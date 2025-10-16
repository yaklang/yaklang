package sfreport

import (
	"fmt"
	"slices"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type File struct {
	Path      string            `json:"path"`    // file path
	Length    int64             `json:"length"`  // length of the code
	Hash      map[string]string `json:"hash"`    // hash of the code
	Content   string            `json:"content"` // long text
	LineCount int               `json:"line_count"`

	Risks []string `json:"risks"` // risk hash list
}

func NewFile(editor *memedit.MemEditor, r *Report) *File {
	ret := &File{
		Path:      editor.GetUrl(),
		Length:    int64(editor.GetLength()),
		LineCount: editor.GetLineCount(),
		Hash: map[string]string{
			"md5":    editor.SourceCodeMd5(),
			"sha1":   editor.SourceCodeSha1(),
			"sha256": editor.SourceCodeSha256(),
		},
	}

	if r.ReportType == IRifyFullReportType || r.config.showFileContent {
		ret.Content = editor.GetSourceCode()
	} else {
		// only show the first 100 characters
		ret.Content = fmt.Sprintf("%s...", editor.GetSourceCode(100))
	}

	return ret
}

func (f *File) SaveToDB(db *gorm.DB, programName string) error {
	if db == nil {
		return utils.Error("Save File to DB failed: db is nil")
	}
	editor := memedit.NewMemEditorWithFileUrl(f.Content, f.Path)
	editor.SetProgramName(programName)
	irSource := ssadb.MarshalFile(editor)
	if err := irSource.Save(db); err != nil {
		return utils.Wrapf(err, "Save File to DB failed")
	}
	return nil
}

func (f *File) AddRisk(risk *Risk) {
	if slices.Contains(f.Risks, risk.GetHash()) {
		return
	}
	f.Risks = append(f.Risks, risk.GetHash())
}
