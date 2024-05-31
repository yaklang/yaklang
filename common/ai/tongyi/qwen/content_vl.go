package qwen

type VLContent struct {
	Image string `json:"image,omitempty"`
	Text  string `json:"text,omitempty"`
}

var _ IBlobContent = VLContent{}

func (vc VLContent) GetBlob() string {
	return vc.Image
}

// VLContentList is used for multi-modal generation.
type VLContentList []VLContent

var _ IQwenContentMethods = &VLContentList{}

func NewVLContentList() *VLContentList {
	vl := VLContentList(make([]VLContent, 0))
	return &vl
}

func (vlist *VLContentList) ToBytes() []byte {
	if vlist == nil || len(*vlist) == 0 {
		return []byte("")
	}
	return []byte((*vlist)[0].Text)
}

func (vlist *VLContentList) ToString() string {
	if vlist == nil || len(*vlist) == 0 {
		return ""
	}
	return (*vlist)[0].Text
}

func (vlist *VLContentList) SetText(s string) {
	if vlist == nil {
		panic("VLContentList is nil")
	}
	*vlist = append(*vlist, VLContent{Text: s})
}

func (vlist *VLContentList) SetImage(url string) {
	vlist.SetBlob(url)
}

func (vlist *VLContentList) SetBlob(url string) {
	if vlist == nil {
		panic("VLContentList is nil or empty")
	}
	*vlist = append(*vlist, VLContent{Image: url})
}

func (vlist *VLContentList) PopImageContent() (VLContent, bool) {
	blobContent, hasAudio := popBlobContent(vlist)

	if content, ok := blobContent.(VLContent); ok {
		return content, hasAudio
	}
	return VLContent{}, false
}

func (vlist *VLContentList) AppendText(s string) {
	if vlist == nil || len(*vlist) == 0 {
		panic("VLContentList is nil or empty")
	}
	(*vlist)[0].Text += s
}

func (vlist *VLContentList) ConvertToBlobList() []IBlobContent {
	if vlist == nil {
		panic("VLContentList is nil or empty")
	}

	list := make([]IBlobContent, len(*vlist))
	for i, v := range *vlist {
		list[i] = v
	}
	return list
}

func (vlist *VLContentList) ConvertBackFromBlobList(list []IBlobContent) {
	if vlist == nil {
		panic("VLContentList is nil or empty")
	}

	*vlist = make([]VLContent, len(list))
	for i, v := range list {
		if content, ok := v.(VLContent); ok {
			(*vlist)[i] = content
		}
	}
}
