package sfreport

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type File struct {
	Path         string            `json:"path"`           // file path
	Length       int64             `json:"length"`         // length of the code
	Hash         map[string]string `json:"hash"`           // hash of the code
	IrSourceHash string            `json:"ir_source_hash"` // Md5(ProgramName + FolderPath + FileName + SourceCode)
	Content      string            `json:"content"`        // long text
	LineCount    int               `json:"line_count"`

	Risks []string `json:"risks"` // risk hash list
}

func NewFile(editor *memedit.MemEditor, r *Report) *File {
	return editor2File(editor, r.ReportType == IRifyFullReportType || r.config.showFileContent)
}

func (f *File) SaveToDB(db *gorm.DB, programName string) error {
	if db == nil {
		return utils.Error("Save File to DB failed: db is nil")
	}
	// 若 content 为空且已有完整内容，避免覆盖导致审计缺失
	if f != nil && f.Content == "" && f.IrSourceHash != "" {
		var existing ssadb.IrSource
		err := db.Where("source_code_hash = ?", f.IrSourceHash).First(&existing).Error
		if err == nil {
			if existing.QuotedCode != "" && existing.QuotedCode != "\"\"" {
				return nil
			}
		} else if !gorm.IsRecordNotFoundError(err) {
			return utils.Wrapf(err, "Save File to DB failed")
		}
	}
	editor, err := file2Editor(f, programName)
	if err != nil {
		return err
	}
	irSource := ssadb.MarshalFile(editor, f.IrSourceHash)
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

func editor2File(editor *memedit.MemEditor, fullCode bool) *File {
	ret := &File{
		Path:         editor.GetUrl(),
		Length:       int64(editor.GetLength()),
		LineCount:    editor.GetLineCount(),
		IrSourceHash: editor.GetIrSourceHash(),
		Hash: map[string]string{
			"md5":    editor.SourceCodeMd5(),
			"sha1":   editor.SourceCodeSha1(),
			"sha256": editor.SourceCodeSha256(),
		},
	}
	if fullCode {
		ret.Content = editor.GetSourceCode()
	} else {
		ret.Content = fmt.Sprintf("%s...", editor.GetSourceCode(100))
	}
	return ret
}

func file2Editor(file *File, programName string) (*memedit.MemEditor, error) {
	if programName == "" {
		return nil, utils.Error("file2Editor: programName cannot be empty")
	}

	editor := memedit.NewMemEditor(file.Content)
	editor.SetUrl(file.Path)
	editor.SetProgramName(programName)

	if file.Path == "" {
		return editor, nil
	}

	cleanPath := strings.TrimPrefix(file.Path, "/")
	parts := strings.Split(cleanPath, "/")

	// setFolderPath and fileName
	if len(parts) >= 2 {
		fileName := parts[len(parts)-1]
		editor.SetFileName(fileName)

		if len(parts) > 2 {
			folderPath := "/" + strings.Join(parts[1:len(parts)-1], "/")
			editor.SetFolderPath(folderPath)
		} else {
			editor.SetFolderPath("/")
		}
	}
	return editor, nil
}
