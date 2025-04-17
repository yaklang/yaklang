package sfreport

import (
	"fmt"
	"slices"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

type File struct {
	Path    string            `json:"path"`    // file path
	Length  int64             `json:"length"`  // length of the code
	Hash    map[string]string `json:"hash"`    // hash of the code
	Content string            `json:"content"` // long text

	Risks []string `json:"risks"` // risk hash list
}

func NewFile(reportType ReportType, editor *memedit.MemEditor) *File {
	ret := &File{
		Path:   editor.GetFilename(),
		Length: int64(editor.GetLength()),
		Hash: map[string]string{
			"md5":    editor.SourceCodeMd5(),
			"sha1":   editor.SourceCodeSha1(),
			"sha256": editor.SourceCodeSha256(),
		},
	}

	switch reportType {
	case IRifyReportType:
		// only show the first 100 characters
		ret.Content = fmt.Sprintf("%s...", editor.GetSourceCode(100))
	case IRifyFullReportType:
		ret.Content = editor.GetSourceCode()
	}

	return ret
}

func (f *File) AddRisk(risk *Risk) {
	if slices.Contains(f.Risks, risk.Hash) {
		return
	}
	f.Risks = append(f.Risks, risk.Hash)
}
