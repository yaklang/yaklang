package aicommon

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func init() {
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeFile,
		func() AttachedResourceData { return NewAttachedFileResourceData("") },
		CONTEXT_PROVIDER_KEY_FILE_PATH,
		"filepath",
		"file-path",
	)
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeKnowledgeBase,
		func() AttachedResourceData { return &AttachedKnowledgeBaseResourceData{} },
		CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE,
	)
}

const (
	AttachedFileKindMissing   = "missing"
	AttachedFileKindDirectory = "directory"
	AttachedFileKindText      = "text"
	AttachedFileKindImage     = "image"
	AttachedFileKindBinary    = "binary"
)

type DefaultAttachedResourceData struct {
	ResourceType string
	Key          string
	Value        string
}

func NewDefaultAttachedResourceData(typ string, key string) *DefaultAttachedResourceData {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		typ = AttachedResourceTypeDefault
	}
	return &DefaultAttachedResourceData{
		ResourceType: typ,
		Key:          strings.TrimSpace(key),
	}
}

func (d *DefaultAttachedResourceData) Type() string {
	if strings.TrimSpace(d.ResourceType) == "" {
		return AttachedResourceTypeDefault
	}
	return d.ResourceType
}

func (d *DefaultAttachedResourceData) Unmarshal(raw string) error {
	d.Value = strings.TrimSpace(raw)
	return nil
}

func (d *DefaultAttachedResourceData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *DefaultAttachedResourceData) ToAttachData(reactloop ReActLoopIF) string {
	var emitter *Emitter
	if reactloop != nil {
		emitter = reactloop.GetEmitter()
	}

	inline, spillNote := inlineOrSpillAttachedText("default_attached_resource", d.Value, AttachedDefaultResourceInlineLimit, emitter)
	var b strings.Builder
	b.WriteString("## Attached Resource\n\n")
	b.WriteString(fmt.Sprintf("- Resource Type: %s\n", d.Type()))
	if d.Key != "" {
		b.WriteString(fmt.Sprintf("- Resource Key: %s\n", d.Key))
	}
	b.WriteString("- Handling: no structured attached-resource parser is registered for this type; the raw value is provided as user-attached context.\n\n")
	b.WriteString("### Raw Value\n")
	if spillNote != "" {
		b.WriteString(spillNote)
		b.WriteString("\n\nInline preview:\n```\n")
		b.WriteString(inline)
		b.WriteString("\n```\n")
	} else {
		b.WriteString("```\n")
		b.WriteString(inline)
		b.WriteString("\n```\n")
	}
	return strings.TrimSpace(b.String())
}

type AttachedFileResourceData struct {
	Key       string
	Path      string
	Kind      string
	MIMEType  string
	Size      int64
	StatError string
}

func NewAttachedFileResourceData(key string) *AttachedFileResourceData {
	return &AttachedFileResourceData{
		Key:  strings.TrimSpace(key),
		Kind: AttachedFileKindMissing,
	}
}

func (d *AttachedFileResourceData) Type() string {
	return AttachedResourceTypeFile
}

func (d *AttachedFileResourceData) Unmarshal(raw string) error {
	path := strings.TrimSpace(raw)
	if path == "" {
		return utils.Error("attached file path is empty")
	}
	d.Path = path
	d.MIMEType = attachedFileMIMEType(path)

	info, err := os.Stat(path)
	if err != nil {
		d.Kind = AttachedFileKindMissing
		d.StatError = err.Error()
		return nil
	}
	d.Size = info.Size()
	if info.IsDir() {
		d.Kind = AttachedFileKindDirectory
		return nil
	}
	switch {
	case IsImageContextAttachmentPath(path):
		d.Kind = AttachedFileKindImage
	case isAttachedTextFilePath(path, d.MIMEType):
		d.Kind = AttachedFileKindText
	default:
		d.Kind = AttachedFileKindBinary
	}
	return nil
}

func (d *AttachedFileResourceData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *AttachedFileResourceData) ToAttachData(reactloop ReActLoopIF) string {
	path := strings.TrimSpace(d.Path)
	if path == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Attached File\n\n")
	b.WriteString("- Resource Type: file\n")
	if d.Key != "" {
		b.WriteString(fmt.Sprintf("- Resource Key: %s\n", d.Key))
	}
	b.WriteString(fmt.Sprintf("- File: `%s`\n", path))
	b.WriteString(fmt.Sprintf("- File Kind: %s\n", d.Kind))
	if d.MIMEType != "" {
		b.WriteString(fmt.Sprintf("- MIME Type: %s\n", d.MIMEType))
	}
	if d.Size > 0 {
		b.WriteString(fmt.Sprintf("- Size: %d bytes\n", d.Size))
	}

	switch d.Kind {
	case AttachedFileKindMissing:
		if d.StatError != "" {
			b.WriteString(fmt.Sprintf("\n_Error: failed to stat attached file: %s_\n", d.StatError))
		}
	case AttachedFileKindDirectory:
		tree := truncateAttachedFilePreview(filesys.Glance(path))
		b.WriteString(fmt.Sprintf("\n### Directory Glance (first %d bytes)\n\n```\n", AttachedFilePreviewLimit))
		b.WriteString(tree)
		b.WriteString("\n```\n")
	case AttachedFileKindText:
		content, truncated, err := readAttachedFileTextPreview(path)
		if err != nil {
			b.WriteString(fmt.Sprintf("\n_Error: failed to read attached text file: %v_\n", err))
			break
		}
		b.WriteString(fmt.Sprintf("\n### File Content Preview (first %d bytes)\n\n```\n", AttachedFilePreviewLimit))
		b.WriteString(content)
		b.WriteString("\n```\n")
		if truncated {
			b.WriteString(fmt.Sprintf("\n\n_Content truncated to first %d bytes._", AttachedFilePreviewLimit))
		}
	case AttachedFileKindImage:
		b.WriteString("\nContent dump skipped for image file; registered loop file handlers may parse it and add vision results.\n")
	default:
		b.WriteString("\nContent dump skipped for non-text file; registered loop file handlers may parse it.\n")
	}
	return strings.TrimSpace(b.String())
}

func (d *AttachedFileResourceData) IsImage() bool {
	return d != nil && d.Kind == AttachedFileKindImage
}

func attachedFileMIMEType(path string) string {
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	return mimeType
}

func isAttachedTextFilePath(path string, mimeType string) bool {
	if mimeType != "" && isTextMimeType(mimeType) {
		return true
	}
	ext := filepath.Ext(path)
	if isTextFileExtension(ext) {
		return true
	}
	if ext != "" {
		return false
	}
	switch strings.ToLower(filepath.Base(path)) {
	case "makefile", "dockerfile", "vagrantfile", "gemfile", "rakefile", "procfile":
		return true
	default:
		return false
	}
}

func readAttachedFileTextPreview(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	limited := io.LimitReader(file, AttachedFilePreviewLimit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", false, err
	}
	truncated := len(data) > AttachedFilePreviewLimit
	if truncated {
		data = data[:AttachedFilePreviewLimit]
	}
	return string(data), truncated, nil
}

func truncateAttachedFilePreview(content string) string {
	content = strings.TrimSpace(content)
	if len(content) <= AttachedFilePreviewLimit {
		return content
	}
	return content[:AttachedFilePreviewLimit]
}

type AttachedKnowledgeBaseResourceData struct {
	Value string
}

func (d *AttachedKnowledgeBaseResourceData) Type() string {
	return AttachedResourceTypeKnowledgeBase
}

func (d *AttachedKnowledgeBaseResourceData) Unmarshal(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return utils.Error("attached knowledge base value is empty")
	}
	d.Value = value
	return nil
}

func (d *AttachedKnowledgeBaseResourceData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *AttachedKnowledgeBaseResourceData) ToAttachData(reactloop ReActLoopIF) string {
	return ""
}
