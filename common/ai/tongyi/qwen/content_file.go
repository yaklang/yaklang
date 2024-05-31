package qwen

type FileContent struct {
	File string `json:"file,omitempty"`
	Text string `json:"text,omitempty"`
}

func (vc FileContent) GetBlob() string {
	return vc.File
}

// FileContent currently used for pdf_extracter plugin only.
type FileContentList []FileContent

var _ IQwenContentMethods = &FileContentList{}

func NewFileContentList() *FileContentList {
	vl := FileContentList(make([]FileContent, 0))
	return &vl
}

func (fclist *FileContentList) ToBytes() []byte {
	if fclist == nil || len(*fclist) == 0 {
		return []byte("")
	}
	return []byte((*fclist)[0].Text)
}

func (fclist *FileContentList) ToString() string {
	if fclist == nil || len(*fclist) == 0 {
		return ""
	}
	return (*fclist)[0].Text
}

func (fclist *FileContentList) SetText(s string) {
	if fclist == nil {
		panic("FileContentList is nil")
	}
	*fclist = append(*fclist, FileContent{Text: s})
}

func (fclist *FileContentList) SetFile(url string) {
	fclist.SetBlob(url)
}

func (fclist *FileContentList) SetBlob(url string) {
	if fclist == nil {
		panic("FileContentList is nil or empty")
	}
	*fclist = append(*fclist, FileContent{File: url})
}

func (fclist *FileContentList) PopFileContent() (FileContent, bool) {
	blobContent, hasFile := popBlobContent(fclist)

	if content, ok := blobContent.(FileContent); ok {
		return content, hasFile
	}
	return FileContent{}, false
}

func (fclist *FileContentList) AppendText(s string) {
	if fclist == nil || len(*fclist) == 0 {
		panic("FileContentList is nil or empty")
	}
	(*fclist)[0].Text += s
}

func (fclist *FileContentList) ConvertToBlobList() []IBlobContent {
	if fclist == nil {
		panic("FileContentList is nil or empty")
	}

	list := make([]IBlobContent, len(*fclist))
	for i, v := range *fclist {
		list[i] = v
	}
	return list
}

func (fclist *FileContentList) ConvertBackFromBlobList(list []IBlobContent) {
	if fclist == nil {
		panic("VLContentList is nil or empty")
	}

	*fclist = make([]FileContent, len(list))
	for i, v := range list {
		if content, ok := v.(FileContent); ok {
			(*fclist)[i] = content
		}
	}
}
