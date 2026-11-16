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
		newAttachedFileResourceData,
		CONTEXT_PROVIDER_KEY_FILE_PATH,
		"filepath",
		"file-path",
	)
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeKnowledgeBase,
		newAttachedKnowledgeBaseResourceData,
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

func (d *DefaultAttachedResourceData) DedupKey() string {
	return attachedResourceDedupKey(d.Type(), d.Key, d.Value)
}

func newDefaultAttachedResourceData(resource *AttachedResource) (AttachedResourceData, error) {
	return &DefaultAttachedResourceData{
		ResourceType: resource.Type,
		Key:          strings.TrimSpace(resource.Key),
		Value:        strings.TrimSpace(resource.Value),
	}, nil
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

func (d *AttachedFileResourceData) DedupKey() string {
	return attachedResourceDedupKey(d.Type(), d.Key, d.Path)
}

func newAttachedFileResourceData(resource *AttachedResource) (AttachedResourceData, error) {
	path := strings.TrimSpace(resource.Value)
	if path == "" {
		return nil, utils.Error("attached file path is empty")
	}
	d := NewAttachedFileResourceData(resource.Key)
	d.Path = path
	d.MIMEType = attachedFileMIMEType(path)

	info, err := os.Stat(path)
	if err != nil {
		d.Kind = AttachedFileKindMissing
		d.StatError = err.Error()
		return d, nil
	}
	d.Size = info.Size()
	if info.IsDir() {
		d.Kind = AttachedFileKindDirectory
		return d, nil
	}
	switch {
	case IsImageContextAttachmentPath(path):
		d.Kind = AttachedFileKindImage
	case isAttachedTextFilePath(path, d.MIMEType):
		d.Kind = AttachedFileKindText
	default:
		d.Kind = AttachedFileKindBinary
	}
	return d, nil
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

// UserPinNotice describes a path attachment without repeating its contents.
func (d *AttachedFileResourceData) UserPinNotice() string {
	if d == nil {
		return ""
	}
	path := strings.TrimSpace(d.Path)
	if path == "" {
		path = strings.TrimSpace(d.Key)
	}
	if path == "" {
		return ""
	}
	switch d.Kind {
	case AttachedFileKindDirectory:
		return fmt.Sprintf("User pinned attached directory `%s`.", path)
	case AttachedFileKindImage:
		return fmt.Sprintf("User pinned attached image `%s`.", path)
	default:
		return fmt.Sprintf("User pinned attached file `%s`.", path)
	}
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
	Key     string
	Value   string
	content AttachedFileContentResourceData
}

func (d *AttachedKnowledgeBaseResourceData) Type() string {
	return AttachedResourceTypeKnowledgeBase
}

func (d *AttachedKnowledgeBaseResourceData) DedupKey() string {
	return attachedResourceDedupKey(d.Type(), d.Key, d.Value)
}

func newAttachedKnowledgeBaseResourceData(resource *AttachedResource) (AttachedResourceData, error) {
	value := strings.TrimSpace(resource.Value)
	if value == "" {
		return nil, utils.Error("attached knowledge base value is empty")
	}
	return &AttachedKnowledgeBaseResourceData{
		Key:   resource.Key,
		Value: resource.Value,
		content: AttachedFileContentResourceData{
			Key:     resource.Key,
			content: resource.Value,
		},
	}, nil
}

func (d *AttachedKnowledgeBaseResourceData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *AttachedKnowledgeBaseResourceData) ToAttachData(reactloop ReActLoopIF) string {
	raw := "```\n" + d.Value + "\n```"
	if len(d.Value) > AttachedDefaultResourceInlineLimit {
		// Reuse the literal attachment renderer to preserve the full byte stream
		// once, with a bounded Unicode-safe preview and a readable file reference.
		raw = d.content.ToAttachData(reactloop)
	}
	preference := "Prefer the user's attached knowledge base name or selection notes when choosing explicit query names. This attachment does not execute a search."
	if d.Key == CONTEXT_PROVIDER_KEY_SYSTEM_FLAG {
		switch {
		case strings.TrimSpace(d.Value) == CONTEXT_PROVIDER_VALUE_ALL_KNOWLEDGE_BASE:
			preference = "The user explicitly selected all knowledge bases; use all=true if querying all bases is needed."
		case strings.HasPrefix(strings.TrimSpace(d.Value), CONTEXT_PROVIDER_VALUE_AUTO_SELECT_KNOWLEDGE_BASE):
			preference = "The user requested automatic selection: call manage_knowledge_base op=list, then choose explicit name or names using the question and attached notes. Do not substitute all=true."
		}
	}
	return fmt.Sprintf("## User-attached Knowledge Base Preference\n\n- Resource Key: %s\n- Selection: %s\n\n### Raw Value\n%s", d.Key, preference, raw)
}
