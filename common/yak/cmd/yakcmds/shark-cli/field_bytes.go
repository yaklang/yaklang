package sharkcli

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func formatFieldValue(name string, value any) string {
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		binary := !utf8.ValidString(v)
		for _, r := range v {
			if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' || unicode.In(r, unicode.Cf) {
				binary = true
				break
			}
		}
		lower := strings.ToLower(name)
		if !binary && lower != "data" && lower != "payload" && !strings.HasSuffix(lower, "protocol data") {
			return safeText(v)
		}
		data = []byte(v)
	default:
		return safeText(fmt.Sprint(value))
	}
	preview := data[:min(32, len(data))]
	ascii := make([]byte, len(preview))
	for i, b := range preview {
		ascii[i] = '.'
		if b >= 32 && b < 127 {
			ascii[i] = b
		}
	}
	suffix := ""
	if len(preview) < len(data) {
		suffix = " … (select for full bytes)"
	}
	return fmt.Sprintf("%d bytes · HEX % x |%s|%s", len(data), preview, ascii, suffix)
}

func nodeField(n *base.NodeValue, text string, branch bool) field {
	f := field{text: text, branch: branch}
	// Parser positions are bit offsets, including packed fields. Highlight the
	// containing bytes and preserve exact positions even when values repeat.
	visited := 0
	var visit func(*base.Node)
	visit = func(node *base.Node) {
		if node == nil || node.Cfg == nil || visited >= 8192 {
			return
		}
		visited++
		if stream_parser.NodeHasResult(node) {
			pos := stream_parser.GetNodeResultPos(node)
			if pos[1] > pos[0] && pos[1] <= 262144*8 {
				start, end := int(pos[0]/8), int((pos[1]+7)/8)
				if !f.hasRange {
					f.start, f.end, f.hasRange = start, end, true
				} else {
					f.start, f.end = min(f.start, start), max(f.end, end)
				}
			}
			return
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(n.Origin)
	return f
}

func (u *tui) selectFieldBytes() bool {
	u.hexStart, u.hexEnd = 0, 0
	indices := u.fieldIndices()
	if u.fieldSelection < 0 || u.fieldSelection >= len(indices) {
		return false
	}
	f := u.fields.fields[indices[u.fieldSelection]]
	raw := u.inspectedPacket()
	if !f.hasRange || raw == nil || f.source < 0 || f.source > len(u.fields.sources) {
		return false
	}
	data := raw.data
	if f.source > 0 {
		data = u.fields.sources[f.source-1].Data
	}
	if f.start < 0 || f.end > len(data) || f.end <= f.start {
		return false
	}
	u.bytesSource = f.source
	u.hexStart, u.hexEnd = f.start, f.end
	perRow := bytesPerRow(u.viewport(2).w - 2)
	u.setTop(2, f.start/perRow)
	return true
}

func (u *tui) selectByteAt(x, y int) {
	r := u.viewport(2)
	perRow := bytesPerRow(r.w - 2)
	column := -1
	asciiX := r.x + 9 + perRow*3 + (perRow-1)/8
	for i := 0; i < perRow; i++ {
		hexX := r.x + 8 + i*3 + i/8
		if x >= hexX && x < hexX+2 || x == asciiX+i {
			column = i
			break
		}
	}
	data := u.byteData()
	offset := (u.hexTop+y-r.y)*perRow + column
	if column < 0 || offset < 0 || offset >= len(data) {
		return
	}
	u.touchFields()
	u.hexStart, u.hexEnd = offset, offset+1
	best, span := -1, len(data)+1
	for i, f := range u.fields.fields {
		if f.hasRange && f.source == u.bytesSource && offset >= f.start && offset < f.end && f.end-f.start < span {
			best, span = i, f.end-f.start
		}
	}
	if best >= 0 {
		u.fieldSelection = best
		f := u.fields.fields[best]
		u.hexStart, u.hexEnd = f.start, f.end
		u.ensureSelectionVisible(1)
	}
}

func (u *tui) bytesLabel() string {
	label := "Captured frame"
	if u.bytesSource > 0 && u.bytesSource <= len(u.fields.sources) {
		label = u.fields.sources[u.bytesSource-1].Name
	}
	if u.hexEnd > u.hexStart {
		return fmt.Sprintf("%s · 0x%x–0x%x · %d bytes", label, u.hexStart, u.hexEnd-1, u.hexEnd-u.hexStart)
	}
	return label + " · b source · click bytes"
}

func (u *tui) byteData() []byte {
	if u.bytesSource > 0 && u.bytesSource <= len(u.fields.sources) {
		return u.fields.sources[u.bytesSource-1].Data
	}
	if raw := u.inspectedPacket(); raw != nil {
		return raw.data
	}
	return nil
}
