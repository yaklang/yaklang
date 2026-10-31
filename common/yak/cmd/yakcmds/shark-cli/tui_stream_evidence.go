package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Connection labels alone do not identify a prefix. Use only a complete,
// retained wire PDU in this physical TCP generation/direction/domain, whose
// entire bytes and source digest match the actual captured offset zero.
func capturedPrefixMessage(v *streamSnapshot, d int, data []byte) (*pcaputil.ProtocolEvent, map[string]any) {
	if v.nativeFlowID == 0 || len(data) == 0 {
		return nil, nil
	}
	for _, e := range v.events {
		if e == nil || e.ID == 0 || e.Transport != "tcp" || e.TCPFlowID != v.nativeFlowID || e.Domain != v.domain || e.Source != v.endpoints[d] || e.Destination != v.endpoints[1-d] || e.Offset != 0 || e.SourceBytes.ParentPDU != 0 || len(e.SourceBytes.ParentPDUs) != 0 || e.SourceBytes.CompletedIn == nil || e.SourceBytes.CompletedIn.Domain != v.domain {
			continue
		}
		if e.Status != "decoded" && e.Status != "deferred" || e.Error != "" || e.Length <= 0 || e.Length != len(e.Raw) || len(e.Raw) > len(data) || !bytes.Equal(e.Raw, data[:len(e.Raw)]) {
			continue
		}
		if e.SourceBytes.Kind != "captured" && e.SourceBytes.Kind != "reassembled" {
			continue
		}
		foreignReference := false
		for _, ref := range e.SourceBytes.PacketRefs {
			if ref.Domain != v.domain {
				foreignReference = true
				break
			}
		}
		if foreignReference {
			continue
		}
		digest := sha256.Sum256(e.Raw)
		if hex.EncodeToString(digest[:]) != e.SourceBytes.SHA256 {
			continue
		}
		fields, err := e.GetFields()
		complete := e.Completeness == "message" || e.Completeness == "record" || e.Completeness == "frame"
		tlsEnvelope := e.Completeness == "envelope" || e.Completeness == "encrypted"
		if err == nil && len(fields) > 0 && (complete || tlsEnvelope && capturedTLSRecordEnvelope(e, fields)) {
			return e, fields
		}
	}
	return nil, nil
}

func capturedTLSRecordEnvelope(e *pcaputil.ProtocolEvent, fields map[string]any) bool {
	w := e.Raw
	if len(w) < 5 || w[0] < 20 || w[0] > 24 {
		return false
	}
	version, length := int(binary.BigEndian.Uint16(w[1:3])), int(binary.BigEndian.Uint16(w[3:5]))
	if version < 0x300 || version > 0x304 || length+5 != len(w) {
		return false
	}
	for key, want := range map[string]int{"Record Type": int(w[0]), "Record Version": version, "Record Length": length} {
		got, ok := detailRangeOffset(fields[key])
		if !ok || got != want {
			return false
		}
	}
	// Use the admitted field query, never the editable event/connection label.
	owned := *e
	owned.Protocol, owned.Fields, owned.Session, owned.Structured = "tls", fields, fields, nil
	return credibleTLSIdentity(&owned) || strongTLSRecord(&owned)
}

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
	if v.messageBoundaries() {
		return datagramEvidenceRows(v)
	}
	var rows []streamLine
	add := func(text string, style ink) { rows = append(rows, streamLine{safeText(text), style}) }
	add("Connection identity records previously validated messages; it does not claim this packet contains those messages.", inkMuted)
	if len(v.protocolStack) > 0 {
		add("Observed connection: "+strings.Join(v.protocolStack, " › "), inkCyan)
	}
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
		} else if event, fields := capturedPrefixMessage(v, d, data); event != nil {
			label := ""
			tlsRecord := capturedTLSRecordEnvelope(event, fields)
			if smbDisplayOwner(fields, event.Raw) {
				label = "SMB1 "
				if fields["Header"].(map[string]any)["Protocol ID"] == uint32(0x424d53fe) {
					label = "SMB2 "
				}
			} else if tlsRecord {
				label = "TLS record "
			}
			name := textValue(fields["Packet Name"])
			if name == "" && tlsRecord {
				name = recordType(fields["Record Type"])
			}
			if name != "" {
				name = " · " + name
			}
			add(fmt.Sprintf("Validated %sPDU at captured offset 0: #%d · %d bytes%s", label, event.ID, len(event.Raw), name), style)
			add("Evidence: the complete retained wire PDU matches this direction's captured prefix and byte-source digest.", inkMuted)
			if tlsRecord {
				add("TLS scope: record framing and decoded record fields; session version and authentication are separate observations.", inkMuted)
			}
		} else {
			if name := sniffStreamPayload(data, v.ports[d], v.ports[1-d]); name != "" {
				add("Prefix signature: "+name, style)
			} else {
				add("Protocol at captured offset 0 is not identified.", inkMuted)
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
			w := terminalWidth.RuneWidth(r)
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
