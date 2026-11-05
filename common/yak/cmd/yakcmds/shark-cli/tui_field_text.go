package sharkcli

import "strings"

func fieldDisplayText(f field) string {
	prefix := "  "
	if f.branch {
		prefix = "── "
	} else if f.depth > 0 {
		prefix = ""
	}
	return safeText(strings.Repeat("  ", min(max(0, f.depth), 16)) + prefix + f.text)
}

// Columns use the same locale-independent widths as the canvas. If the left
// edge intersects a wide rune, leave its remaining cell blank instead of
// moving following characters or emitting half a glyph.
func fieldTextAfterColumns(text string, columns int) string {
	if columns <= 0 {
		return text
	}
	used := 0
	for at, r := range text {
		width := terminalWidth.RuneWidth(r)
		if used >= columns {
			return "‹" + text[at:]
		}
		used += width
		if used > columns {
			return "‹" + strings.Repeat(" ", used-columns) + text[at+len(string(r)):]
		}
	}
	return "‹"
}

func (u *tui) fieldTextLimit() int {
	indices := u.fieldIndices()
	if u.fieldSelection < 0 || u.fieldSelection >= len(indices) {
		return 0
	}
	width := max(1, u.viewport(1).w-2)
	length := terminalWidth.StringWidth(fieldDisplayText(u.fields.fields[indices[u.fieldSelection]]))
	if length <= width {
		return 0
	}
	return length - width + 1 // one cell identifies the hidden left prefix
}

func (u *tui) scrollFieldText(delta int) {
	u.touchFields()
	u.fieldLeft = clamp(u.fieldLeft+delta, 0, u.fieldTextLimit())
}
