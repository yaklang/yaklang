package aitool

const MaxOutputFileTokens int64 = 40 * 1024

type OutputFileInfo struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Content string `json:"content,omitempty"`
}

func (o *OutputFileInfo) IsSafeSize() bool {
	return o.Size <= MaxOutputFileTokens
}
