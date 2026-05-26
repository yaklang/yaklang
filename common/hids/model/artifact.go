//go:build hids

package model

type Artifact struct {
	Path       string          `json:"path,omitempty"`
	Exists     bool            `json:"exists,omitempty"`
	SizeBytes  int64           `json:"size_bytes,omitempty"`
	FileType   string          `json:"file_type,omitempty"`
	TypeSource string          `json:"type_source,omitempty"`
	Magic      string          `json:"magic,omitempty"`
	MimeType   string          `json:"mime_type,omitempty"`
	Extension  string          `json:"extension,omitempty"`
	Hashes     *ArtifactHashes `json:"hashes,omitempty"`
	ELF        *ELFArtifact    `json:"elf,omitempty"`
}

type ArtifactHashes struct {
	SHA256 string `json:"sha256,omitempty"`
	MD5    string `json:"md5,omitempty"`
}

type ELFArtifact struct {
	Class        string               `json:"class,omitempty"`
	Machine      string               `json:"machine,omitempty"`
	ByteOrder    string               `json:"byte_order,omitempty"`
	EntryAddress string               `json:"entry_address,omitempty"`
	SectionCount int                  `json:"section_count,omitempty"`
	SegmentCount int                  `json:"segment_count,omitempty"`
	Sections     []string             `json:"sections,omitempty"`
	Segments     []string             `json:"segments,omitempty"`
	SectionItems []ELFSectionArtifact `json:"section_items,omitempty"`
	SegmentItems []ELFSegmentArtifact `json:"segment_items,omitempty"`
}

type ELFSectionArtifact struct {
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Flags    string `json:"flags,omitempty"`
	Addr     string `json:"addr,omitempty"`
	Offset   int64  `json:"offset,omitempty"`
	Size     int64  `json:"size,omitempty"`
	IsSymTab bool   `json:"is_symtab,omitempty"`
	IsStrTab bool   `json:"is_strtab,omitempty"`
}

type ELFSegmentArtifact struct {
	Type   string `json:"type,omitempty"`
	Flags  string `json:"flags,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	VAddr  string `json:"vaddr,omitempty"`
	FileSz int64  `json:"filesz,omitempty"`
	MemSz  int64  `json:"memsz,omitempty"`
	IsCode bool   `json:"is_code,omitempty"`
	IsData bool   `json:"is_data,omitempty"`
}

func CloneArtifact(input *Artifact) *Artifact {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Hashes = CloneArtifactHashes(input.Hashes)
	cloned.ELF = CloneELFArtifact(input.ELF)
	return &cloned
}

func CloneArtifactHashes(input *ArtifactHashes) *ArtifactHashes {
	if input == nil {
		return nil
	}
	cloned := *input
	return &cloned
}

func CloneELFArtifact(input *ELFArtifact) *ELFArtifact {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.Sections = cloneArtifactStringSlice(input.Sections)
	cloned.Segments = cloneArtifactStringSlice(input.Segments)
	cloned.SectionItems = cloneELFSectionArtifacts(input.SectionItems)
	cloned.SegmentItems = cloneELFSegmentArtifacts(input.SegmentItems)
	return &cloned
}

func cloneArtifactStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneELFSectionArtifacts(values []ELFSectionArtifact) []ELFSectionArtifact {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]ELFSectionArtifact, len(values))
	copy(cloned, values)
	return cloned
}

func cloneELFSegmentArtifacts(values []ELFSegmentArtifact) []ELFSegmentArtifact {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]ELFSegmentArtifact, len(values))
	copy(cloned, values)
	return cloned
}

func ELFSectionDetailItems(values []ELFSectionArtifact) []map[string]any {
	if len(values) == 0 {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0, len(values))
	for _, item := range values {
		items = append(items, map[string]any{
			"name":      item.Name,
			"type":      item.Type,
			"flags":     item.Flags,
			"addr":      item.Addr,
			"offset":    item.Offset,
			"size":      item.Size,
			"is_symtab": item.IsSymTab,
			"is_strtab": item.IsStrTab,
		})
	}
	return items
}

func ELFSegmentDetailItems(values []ELFSegmentArtifact) []map[string]any {
	if len(values) == 0 {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0, len(values))
	for _, item := range values {
		items = append(items, map[string]any{
			"type":    item.Type,
			"flags":   item.Flags,
			"offset":  item.Offset,
			"vaddr":   item.VAddr,
			"filesz":  item.FileSz,
			"memsz":   item.MemSz,
			"is_code": item.IsCode,
			"is_data": item.IsData,
		})
	}
	return items
}
