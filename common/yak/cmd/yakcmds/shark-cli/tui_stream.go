package sharkcli

import (
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"
)

type streamLine struct {
	text  string
	style ink
}

func (u *tui) syncStream() {
	if u.pane != 3 {
		return
	}
	packet := u.inspectedPacket()
	if packet == nil {
		return
	}
	if u.streamPacket != packet.number {
		u.loadStream()
	} else if !u.streamPinned && time.Since(u.streamRefresh) >= time.Second {
		u.streamRefresh = time.Now()
		if u.stream == nil || u.streams.version(packet.streamID) != u.stream.version {
			u.stream = u.streams.snapshot(packet.streamID)
			u.streamRows = nil
		}
	}
}
func (u *tui) loadStream() {
	raw := u.inspectedPacket()
	u.stream, u.streamRows = nil, nil
	u.streamFields = packetDetail{}
	u.streamTop = 0
	u.streamPinned = false
	u.streamRefresh = time.Now()
	if raw == nil {
		return
	}
	if u.streamPacket != raw.number {
		u.streamBeginning = true
	}
	u.streamPacket = raw.number
	u.stream = u.streams.snapshot(raw.streamID)
	u.streamStatus = "h beginning · l recent · e evidence · v direction · t/x/d view · r resume"
}

func textPayload(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	n, printable := 0, 0
	for _, r := range string(data) {
		n++
		if unicode.IsPrint(r) || r == '\r' || r == '\n' || r == '\t' {
			printable++
		}
	}
	return n == 0 || printable*100 >= n*90
}

func (u *tui) streamLines() []streamLine {
	width := max(20, u.viewport(3).w-2)
	if u.streamRows != nil && u.streamWidth == width {
		return u.streamRows
	}
	u.streamWidth = width
	var rows []streamLine
	add := func(text string, style ink) { rows = append(rows, streamLine{safeText(text), style}) }
	v := u.stream
	if v == nil {
		add("Select a TCP packet to inspect its reassembled connection.", inkMuted)
		if raw := u.inspectedPacket(); raw != nil && raw.streamID != 0 {
			add("This stream was evicted from the bounded history; capture to a file to retain it.", inkGold)
		}
		u.streamRows = rows
		return rows
	}
	add(v.description(), inkTitle)
	add("A  "+v.endpoints[0]+"    ⇄    B  "+v.endpoints[1], inkCyan)
	state := v.closed
	if state == "" {
		state = "open"
	}
	add(fmt.Sprintf("%s · %d packets · A→B %s · B→A %s · retrans/overlap %d · out-of-order %d", state, v.packets, byteSize(float64(v.bytes[0])), byteSize(float64(v.bytes[1])), v.retransmits, v.outOfOrder), inkMuted)
	if v.omitted > 0 {
		add(fmt.Sprintf("[recent window: %s older bytes omitted; h shows the separately preserved beginning]", byteSize(float64(v.omitted))), inkGold)
	}
	if v.gaps > 0 {
		add(fmt.Sprintf("[incomplete: %d gap/start markers, %d known missing bytes; gaps are never joined]", v.gaps, v.missing), inkGold)
	}
	if v.protocol == "TLS" || v.protocol == "SSH" {
		add("Encrypted transport: reconstructed wire bytes; no decryption keys are loaded.", inkGold)
	}
	if v.protocol == "HTTP" && v.evidence != "port hint" {
		add(httpConnectionContext(v.prefix), inkGold)
	}
	if u.streamMode == 3 {
		rows = append(rows, u.streamEvidenceRows()...)
	} else if u.streamMode == 2 {
		if len(u.streamFields.fields) == 0 {
			add("Application fields are being decoded from contiguous prefixes…", inkMuted)
		}
		for _, f := range u.streamFields.fields {
			style := inkText
			if f.branch {
				style = inkCyan
			}
			add(f.text, style)
		}
	} else {
		if len(v.chunks) == 0 {
			add("No contiguous payload yet (ACK-only, or waiting up to 2s for earlier segments). r refresh", inkMuted)
		}
		chunks := v.chunks
		if u.streamBeginning {
			chunks = nil
			add("BEGINNING · preserved independently of recent bytes · max 16 KiB per direction · l recent / e evidence", inkCyan)
			for d, data := range v.prefix {
				if len(data) > 0 {
					chunks = append(chunks, streamChunk{direction: d, data: data})
				}
			}
		} else {
			add("RECENT WINDOW · this is not the beginning · h preserved beginning / e evidence", inkGold)
		}
		for _, chunk := range chunks {
			if u.streamDirection != 0 && chunk.direction != u.streamDirection-1 {
				continue
			}
			direction, style := "A → B", inkCyan
			if chunk.direction == 1 {
				direction, style = "B → A", inkGold
			}
			add(fmt.Sprintf("── %s · offset %d · %d bytes ──", direction, chunk.offset, len(chunk.data)), style)
			if u.streamBeginning {
				add(prefixOrigin(v, chunk.direction), style)
				if v.prefixGap[chunk.direction] {
					add("[Preserved prefix ends at a capture gap.]", inkGold)
				}
			}
			if chunk.gap < 0 {
				add("[capture started midstream: bytes before this point are unknown]", inkRed)
			} else if chunk.gap > 0 {
				add(fmt.Sprintf("[GAP: %d bytes missing before this point]", chunk.gap), inkRed)
			}
			appendStreamChunk(&rows, chunk, u.streamMode, width, style)
			if len(rows) > 100000 {
				add("[View row limit reached; use the saved capture for the full data.]", inkGold)
				break
			}
		}
	}
	u.streamRows = rows
	return rows
}
func (u *tui) drawStream(c *canvas, r rect) {
	rows := u.streamLines()
	u.setTop(3, u.streamTop)
	u.drawScrollbar(c, r, 3)
	for i := 0; i < r.h && u.streamTop+i < len(rows); i++ {
		row := rows[u.streamTop+i]
		c.text(r.x, r.y+i, r.w-2, row.text, row.style)
	}
}
