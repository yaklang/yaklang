package aicommon

import (
	"fmt"
	"os"
	"sync"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/consts"
)

// AttachedFileContentResourceData holds literal text, never a filesystem path.
// Large inputs retain their exact bytes in AI space for on-demand reading.
type AttachedFileContentResourceData struct {
	content  string
	once     sync.Once
	rendered string
	err      error
}

var _ AttachedResourceData = (*AttachedFileContentResourceData)(nil)

func (d *AttachedFileContentResourceData) Type() string { return AttachedResourceTypeFile }

func (d *AttachedFileContentResourceData) Unmarshal(raw string) error {
	d.content = raw
	d.once = sync.Once{}
	d.rendered, d.err = "", nil
	return nil
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

func (d *AttachedFileContentResourceData) render(emitter *Emitter) (string, error) {
	d.once.Do(func() {
		preview := d.content
		note := ""
		if len(preview) > AttachedDefaultResourceInlineLimit {
			file, err := consts.TempAIFile("attached-file-content-*.txt")
			if err != nil {
				d.err = err
				return
			}
			_, writeErr := file.WriteString(d.content)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				_ = os.Remove(file.Name())
				d.err = writeErr
				if d.err == nil {
					d.err = closeErr
				}
				return
			}
			end := AttachedDefaultResourceInlineLimit
			for end > 0 && !utf8.RuneStart(preview[end]) {
				end--
			}
			preview = preview[:end]
			note = fmt.Sprintf("Full content saved to file: %s\nUse file-reading tools to load the complete content.\n\nInline preview:\n", file.Name())
			if emitter != nil {
				_, _ = emitter.EmitPinFilename(file.Name())
			}
		}
		d.rendered = fmt.Sprintf("## Attached File Content\n\nFile Size: %d bytes\n\n%s--- File Content ---\n%s\n--- End of File Content ---\n", len(d.content), note, preview)
	})
	return d.rendered, d.err
}
