package aicommon

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeCode,
		func() AttachedResourceData { return NewAttachedCodeResourceData("") },
	)
}

// AttachedCodeResourceData is Type=code: a writable editor/script delivery path.
// It must NOT be treated as read-only Type=file (@mention / reference) context.
// ToAttachData is intentionally empty — focus loops (e.g. write_yaklang_code) bind
// delivery via YaklangEditorContext / yaklang_editor_context timeline instead.
type AttachedCodeResourceData struct {
	Key  string
	Path string
}

func NewAttachedCodeResourceData(key string) *AttachedCodeResourceData {
	return &AttachedCodeResourceData{Key: strings.TrimSpace(key)}
}

func (d *AttachedCodeResourceData) Type() string {
	return AttachedResourceTypeCode
}

func (d *AttachedCodeResourceData) Unmarshal(raw string) error {
	path := strings.TrimSpace(raw)
	if path == "" {
		return utils.Error("attached code path is empty")
	}
	d.Path = filepath.Clean(path)
	if d.Key == "" {
		d.Key = CONTEXT_PROVIDER_KEY_FILE_PATH
	}
	return nil
}

func (d *AttachedCodeResourceData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *AttachedCodeResourceData) ToAttachData(reactloop ReActLoopIF) string {
	// Delivery targets are not dumped into attached_code timeline as reference files.
	// write_yaklang_code records them under yaklang_editor_context instead.
	_ = reactloop
	return ""
}

// FormatAttachedCodeDeliveryHint is available for loops that want a one-line delivery note.
func FormatAttachedCodeDeliveryHint(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return fmt.Sprintf("Code delivery target (writable): `%s`", path)
}
