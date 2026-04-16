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
	Class        string   `json:"class,omitempty"`
	Machine      string   `json:"machine,omitempty"`
	ByteOrder    string   `json:"byte_order,omitempty"`
	EntryAddress string   `json:"entry_address,omitempty"`
	SectionCount int      `json:"section_count,omitempty"`
	SegmentCount int      `json:"segment_count,omitempty"`
	Sections     []string `json:"sections,omitempty"`
	Segments     []string `json:"segments,omitempty"`
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
