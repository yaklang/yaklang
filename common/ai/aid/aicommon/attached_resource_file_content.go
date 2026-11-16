package aicommon

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/consts"
)

// AttachedFileContentResourceData holds literal text, never a filesystem path.
// Large inputs retain their exact bytes in AI space for on-demand reading.
type AttachedFileContentResourceData struct {
	Key       string
	content   string
	savedPath string
	once      sync.Once
	rendered  string
	err       error
}

var _ AttachedResourceData = (*AttachedFileContentResourceData)(nil)

func (d *AttachedFileContentResourceData) Type() string { return AttachedResourceTypeFile }

func (d *AttachedFileContentResourceData) DedupKey() string {
	return attachedResourceDedupKey(d.Type(), d.Key, d.content)
}

func newAttachedFileContentResourceData(resource *AttachedResource) (AttachedResourceData, error) {
	return &AttachedFileContentResourceData{
		Key:     resource.Key,
		content: resource.Value,
	}, nil
}

func (d *AttachedFileContentResourceData) BindLoopData(loop ReActLoopIF) error {
	var emitter *Emitter
	if loop != nil {
		emitter = loop.GetEmitter()
	}
	_, err := d.render(emitter)
	return err
}

func (d *AttachedFileContentResourceData) ToAttachData(loop ReActLoopIF) string {
	var emitter *Emitter
	if loop != nil {
		emitter = loop.GetEmitter()
	}
	rendered, err := d.render(emitter)
	if err != nil {
		return fmt.Sprintf("[Error preparing attached file content: %v]", err)
	}
	return rendered
}

// UserPinNotice describes the user's attachment without copying file content
// into the compression-protected user-input journal.
func (d *AttachedFileContentResourceData) UserPinNotice() string {
	if d == nil {
		return ""
	}
	key := strings.TrimSpace(d.Key)
	if key == "" {
		key = CONTEXT_PROVIDER_KEY_FILE_CONTENT
	}
	notice := fmt.Sprintf("User pinned attached file content (`%s`, %d bytes).", key, len(d.content))
	if path := strings.TrimSpace(d.savedPath); path != "" {
		notice += fmt.Sprintf(" Full content is saved at `%s`.", path)
	}
	return notice
}

func (d *AttachedFileContentResourceData) render(emitter *Emitter) (string, error) {
	d.once.Do(func() {
		file, err := consts.TempAIFile("attached-file-content-*.txt")
		if err != nil {
			d.err = err
			return
		}
		path := file.Name()
		_, writeErr := file.WriteString(d.content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(path)
			d.err = writeErr
			if d.err == nil {
				d.err = closeErr
			}
			return
		}
		d.savedPath = path
		if emitter != nil {
			_, _ = emitter.EmitPinFilename(path)
		}

		preview := d.content
		previewNote := ""
		if len(preview) > AttachedDefaultResourceInlineLimit {
			end := AttachedDefaultResourceInlineLimit
			for end > 0 && !utf8.RuneStart(preview[end]) {
				end--
			}
			preview = preview[:end]
			previewNote = "\n... (inline preview truncated; use the saved file for the complete content) ..."
		}
		d.rendered = fmt.Sprintf(
			"## Attached File Content\n\n- Resource Type: file\n- Resource Key: %s\n- File Size: %d bytes\n- Full content saved to file: %s\n\nUse file-reading tools to load the complete content.\n\nInline preview:\n--- File Content ---\n%s%s\n--- End of File Content ---\n",
			d.Key,
			len(d.content),
			path,
			preview,
			previewNote,
		)
	})
	return d.rendered, d.err
}
