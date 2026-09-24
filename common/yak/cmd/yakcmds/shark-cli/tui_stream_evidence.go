package sharkcli

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

func prefixOrigin(v *streamSnapshot, direction int) string {
	if len(v.prefix[direction]) == 0 {
		return "no contiguous beginning captured yet"
	}
	if v.startKnown[direction] {
		return "TCP payload beginning (SYN observed); offset 0"
	}
	return "first CAPTURED bytes; actual TCP beginning unknown (SYN missing or gap)"
}

func (u *tui) streamEvidenceRows() []streamLine {
	v := u.stream
	var rows []streamLine
	add := func(text string, style ink) { rows = append(rows, streamLine{safeText(text), style}) }
	add("Evidence scope: TCP/HTTP means HTTP was observed in the connection prefix, not in this packet.", inkMuted)
	for d, data := range v.prefix {
		if u.streamDirection != 0 && d != u.streamDirection-1 {
			continue
		}
		direction, style := "A → B", inkCyan
		if d == 1 {
			direction, style = "B → A", inkGold
		}
		add("── "+direction+" · "+prefixOrigin(v, d), style)
		if len(data) == 0 {
			continue
		}
		add(fmt.Sprintf("Preserved %d / %d prefix bytes · first bytes observed at %s", len(data), streamPrefixLimit, v.prefixTime[d].Format("15:04:05.000000")), inkMuted)
		if h := inspectHTTP(data); h != nil {
			add("Verified HTTP at captured offset 0: "+strconv.QuoteToASCII(h.line), style)
			add(fmt.Sprintf("Header block: %d bytes; body/tunnel begins at offset %d", h.length, h.length), inkText)
			for _, key := range []string{"Host", "Content-Type", "Content-Encoding", "Content-Length", "Transfer-Encoding", "Connection", "Upgrade"} {
				if value := h.header.Get(key); value != "" {
					add(key+": "+value, inkText)
				}
			}
			add("Validation: complete start line + syntactically valid HTTP headers; body bytes are not proof of HTTP.", inkMuted)
		} else {
			add("No complete, valid HTTP header at this captured prefix.", inkMuted)
			if name := sniffStreamPayload(data, v.ports[d], v.ports[1-d]); name != "" {
				add("Prefix signature: "+name, style)
			}
		}
		add("Raw first 32 bytes (hex):", inkMuted)
		for offset := 0; offset < min(32, len(data)); offset += 16 {
			add(fmt.Sprintf("%08x  % x", offset, data[offset:min(offset+16, len(data))]), style)
		}
		add("Escaped bytes: "+strconv.QuoteToASCII(string(data[:min(48, len(data))])), style)
		if v.prefixGap[d] {
			add("[Prefix stops before a capture gap; missing bytes were not joined.]", inkRed)
		}
	}
	return rows
}

func appendStreamData(rows *[]streamLine, data []byte, base uint64, mode, width int, style ink) {
	add := func(text string) { *rows = append(*rows, streamLine{safeText(text), style}) }
	if mode == 1 || !textPayload(data) {
		perRow := 16
		if width < 78 {
			perRow = 8
		}
		for offset := 0; offset < len(data); offset += perRow {
			part := data[offset:min(len(data), offset+perRow)]
			var ascii, spaced strings.Builder
			for _, b := range part {
				if b >= 32 && b < 127 {
					ascii.WriteByte(b)
				} else {
					ascii.WriteByte('.')
				}
			}
			encoded := hex.EncodeToString(part)
			for i := 0; i < len(encoded); i += 2 {
				spaced.WriteString(encoded[i : i+2])
				spaced.WriteByte(' ')
			}
			add(fmt.Sprintf("%08x  %-*s %s", base+uint64(offset), perRow*3, spaced.String(), ascii.String()))
		}
		return
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = safeText(strings.ReplaceAll(line, "\t", "    "))
		var b strings.Builder
		columns := 0
		for _, r := range line {
			w := runewidth.RuneWidth(r)
			if columns+w > width {
				add(b.String())
				b.Reset()
				columns = 0
			}
			b.WriteRune(r)
			columns += w
		}
		add(b.String())
	}
}

func appendStreamChunk(rows *[]streamLine, chunk streamChunk, mode, width int, style ink) {
	data, offset := chunk.data, chunk.offset
	// Do not make the real opening HTTP headers disappear into a hex dump just
	// because the following body/tunnel contains binary bytes in the same chunk.
	if mode != 1 {
		if h := inspectHTTP(data); h != nil {
			appendStreamData(rows, data[:h.length], offset, 0, width, style)
			data = data[h.length:]
			offset += uint64(h.length)
			if len(data) > 0 {
				*rows = append(*rows, streamLine{fmt.Sprintf("── after HTTP headers · offset %d · body/tunnel bytes, not separately identified ──", offset), inkMuted})
			}
		}
	}
	if len(data) > 0 {
		appendStreamData(rows, data, offset, mode, width, style)
	}
}
