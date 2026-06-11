package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"
)

func init() {
	RegisterAttachedResourceDataFactory(
		AttachedResourceTypeSelected,
		func() AttachedResourceData { return &AttachedSelectedResourceData{} },
	)
}

type AttachedSelectedResourceData struct {
	Raw       string
	Selected  *AttachedCodeSelection
	PlainText string
}

func (d *AttachedSelectedResourceData) Type() string {
	return AttachedResourceTypeSelected
}

func (d *AttachedSelectedResourceData) Unmarshal(raw string) error {
	d.Raw = raw
	if sel, ok := parseAttachedCodeSelection(raw); ok {
		d.Selected = sel
		d.PlainText = sel.Content
		return nil
	}
	d.PlainText = raw
	return nil
}

func (d *AttachedSelectedResourceData) BindLoopData(reactloop ReActLoopIF) error {
	return nil
}

func (d *AttachedSelectedResourceData) ToAttachData(reactloop ReActLoopIF) string {
	var emitter *Emitter
	if reactloop != nil {
		emitter = reactloop.GetEmitter()
	}
	if d.Selected != nil {
		return FormatAttachedCodeSelection(d.Selected, emitter)
	}
	return FormatAttachedSelectedText(d.PlainText, emitter)
}

func parseAttachedCodeSelection(raw string) (*AttachedCodeSelection, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return nil, false
	}
	var sel AttachedCodeSelection
	if err := json.Unmarshal([]byte(raw), &sel); err != nil {
		return nil, false
	}
	if strings.TrimSpace(sel.Content) == "" {
		return nil, false
	}
	return &sel, true
}

func FormatAttachedSelectedText(content string, emitter *Emitter) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "## Attached Selected Text\n\n(empty selection)"
	}

	inline, spillNote := inlineOrSpillAttachedText("selected_text", content, AttachedSelectedTextInlineLimit, emitter)
	var b strings.Builder
	b.WriteString("## Attached Selected Text\n\n")
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

func FormatAttachedCodeSelection(sel *AttachedCodeSelection, emitter *Emitter) string {
	if sel == nil {
		return FormatAttachedSelectedText("", emitter)
	}
	content := strings.TrimSpace(sel.Content)
	if content == "" {
		return FormatAttachedSelectedText("", emitter)
	}

	inline, spillNote := inlineOrSpillAttachedText("selected_text", content, AttachedSelectedTextInlineLimit, emitter)
	lang := strings.TrimSpace(sel.Language)
	if lang == "" {
		lang = "text"
	}

	var b strings.Builder
	b.WriteString("## Attached Code Selection\n\n")
	if path := strings.TrimSpace(sel.Path); path != "" {
		b.WriteString(fmt.Sprintf("- File: `%s`\n", path))
	}
	if sel.StartLine > 0 && sel.EndLine > 0 {
		b.WriteString(fmt.Sprintf("- Lines: %d-%d\n", sel.StartLine, sel.EndLine))
	}
	if lang != "text" {
		b.WriteString(fmt.Sprintf("- Language: %s\n", lang))
	}
	b.WriteString("\n")
	if spillNote != "" {
		b.WriteString(spillNote)
		b.WriteString(fmt.Sprintf("\n\nInline preview:\n```%s\n", lang))
		b.WriteString(inline)
		b.WriteString("\n```\n")
	} else {
		b.WriteString(fmt.Sprintf("```%s\n", lang))
		b.WriteString(inline)
		b.WriteString("\n```\n")
	}
	return strings.TrimSpace(b.String())
}
