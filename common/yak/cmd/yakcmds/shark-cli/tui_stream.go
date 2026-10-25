package sharkcli

import (
	"fmt"
	"strings"
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
		if packet.conversationID != 0 && packet.streamID == 0 {
			u.stream = datagramConversation(packet)
			u.streamRows = nil
		} else if u.stream == nil || u.streams.version(packet.streamID) != u.stream.version {
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
	if raw.streamID == 0 && raw.conversationID != 0 {
		u.stream = datagramConversation(raw)
		u.streamBeginning = false
	}
	u.streamApplication = u.stream != nil && len(u.stream.events) > 0
	u.streamStatus = "a messages/wire · h beginning · l recent · e evidence · v direction · t/x/d view · r resume"
	if u.stream != nil && u.stream.messageBoundaries() {
		u.streamStatus = "a messages/wire · e association evidence · v direction · t/x/d view · r resume"
	}
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
		add("Select a TCP packet or an observed protocol association to inspect its retained messages.", inkMuted)
		if raw := u.inspectedPacket(); raw != nil && raw.streamID != 0 {
			add("This stream was evicted from the bounded history; capture to a file to retain it.", inkGold)
		}
		u.streamRows = rows
		return rows
	}
	add(v.description(), inkTitle)
	if v.retentionNotice != "" {
		add(v.retentionNotice, inkGold)
	}
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
	if v.protocol == "TLS" {
		verified := false
		var status string
		failures := 0
		for _, e := range v.events {
			if e == nil || e.Protocol != "tls" {
				continue
			}
			verified = verified || eventValues(e)["Authentication Verified"] == true
			if e.Error != "" || e.ExpertCode != "" {
				failures++
			}
			if value := tlsKeyStatus(eventValues(e)); value != "" {
				status = value
			}
		}
		if verified {
			add("TLS record AEAD verified · certificate trust not evaluated · a switches messages / wire", inkGreen)
		} else if status != "" {
			add("TLS · "+status+" · opaque records remain encrypted", inkGold)
		} else if u.tlsKeysConfigured {
			add("TLS keys configured · no authenticated plaintext in this retained view", inkGold)
		} else {
			add("TLS wire view · decryption keys not configured · a shows decoded record metadata", inkGold)
		}
		if failures > 0 {
			add(fmt.Sprintf("TLS diagnostics: %d retained record(s) failed or limited; inspect their messages/fields", failures), inkRed)
		}
	}
	if v.protocol == "SSH" {
		add("SSH · reconstructed wire bytes · payload is encrypted after key exchange", inkGold)
	}
	if v.transport == "udp" {
		add("UDP conversation · retained datagrams only · CID/session association does not verify peer identity", inkGold)
	}
	if v.transport == "network" {
		add("Observed IP association · receiver/session context only · peer/control authentication not verified", inkGold)
	}
	if v.protocol == "HTTP" && v.evidence != "port hint" {
		add(httpConnectionContext(v.prefix), inkGold)
	}
	if u.streamMode == 3 {
		rows = append(rows, u.streamEvidenceRows()...)
	} else if u.streamMode == 2 {
		if len(u.streamFields.fields) == 0 {
			if v.messageBoundaries() {
				add("Retained message fields are unavailable in this bounded view. r refresh", inkMuted)
			} else {
				add("Application fields are being decoded from contiguous prefixes…", inkMuted)
			}
		}
		for _, f := range u.streamFields.fields {
			style := inkText
			if f.branch {
				style = inkCyan
			}
			add(f.text, style)
		}
	} else if u.streamApplication {
		rows = append(rows, applicationMessageRows(v, u.streamDirection, u.streamMode, width)...)
	} else {
		if len(v.chunks) == 0 {
			if v.messageBoundaries() {
				add("No retained message bytes are available. r refresh", inkMuted)
			} else {
				add("No contiguous payload yet (ACK-only, or waiting up to 2s for earlier segments). r refresh", inkMuted)
			}
		}
		chunks := v.chunks
		if u.streamBeginning && !v.messageBoundaries() {
			chunks = nil
			add("BEGINNING · preserved independently of recent bytes · max 16 KiB per direction · l recent / e evidence", inkCyan)
			for d, data := range v.prefix {
				if len(data) > 0 {
					chunks = append(chunks, streamChunk{direction: d, data: data})
				}
			}
		} else {
			if v.transport == "udp" {
				add("RETAINED UDP DATAGRAMS · each PDU keeps its wire boundary · e association evidence", inkGold)
			} else if v.transport == "network" {
				add("RETAINED IP MESSAGES · each PDU keeps its wire boundary · e association evidence", inkGold)
			} else {
				add("RECENT WINDOW · this is not the beginning · h preserved beginning / e evidence", inkGold)
			}
		}
		for _, chunk := range chunks {
			if u.streamDirection != 0 && chunk.direction != u.streamDirection-1 {
				continue
			}
			direction, style := "A → B", inkCyan
			if chunk.direction == 1 {
				direction, style = "B → A", inkGold
			}
			if v.transport == "udp" {
				add(fmt.Sprintf("── %s · datagram PDU #%d · %d bytes ──", direction, chunk.pdu, len(chunk.data)), style)
			} else if v.transport == "network" {
				add(fmt.Sprintf("── %s · IP PDU #%d · %d bytes ──", direction, chunk.pdu, len(chunk.data)), style)
			} else {
				add(fmt.Sprintf("── %s · offset %d · %d bytes ──", direction, chunk.offset, len(chunk.data)), style)
			}
			if u.streamBeginning && !v.messageBoundaries() {
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
			if v.messageBoundaries() {
				appendStreamData(&rows, chunk.data, 0, u.streamMode, width, style)
			} else {
				appendStreamChunk(&rows, chunk, u.streamMode, width, style)
			}
			if len(rows) > 100000 {
				add("[View row limit reached; use the saved capture for the full data.]", inkGold)
				break
			}
		}
	}
	u.streamRows = rows
	return rows
}

func applicationMessageRows(v *streamSnapshot, direction, mode, width int) []streamLine {
	var rows []streamLine
	add := func(text string, style ink) { rows = append(rows, streamLine{safeText(text), style}) }
	presentation := presentEvents(v.events)
	title := "DECODED MESSAGES"
	if v.messageBoundaries() {
		title = "PROTOCOL MESSAGES"
	}
	add(title+" · "+strings.Join(presentation.stack, " › ")+" · a original wire", inkCyan)
	hasApplication := false
	for _, e := range v.events {
		if e != nil && e.Protocol != "tls" && e.Protocol != "quic" && (e.Status == "decoded" || e.Status == "deferred") {
			hasApplication = true
		}
	}
	for _, e := range v.events {
		if e == nil {
			continue
		}
		if hasApplication && (e.Protocol == "tls" || e.Protocol == "quic") && e.Error == "" && e.ExpertCode == "" {
			continue
		}
		d := 0
		if e.Source == v.endpoints[1] {
			d = 1
		}
		if v.messageBoundaries() {
			d = clamp(e.Direction, 0, 1)
		}
		if direction != 0 && direction != d+1 {
			continue
		}
		style, label := inkCyan, "A → B"
		if d == 1 {
			style, label = inkGold, "B → A"
		}
		add(fmt.Sprintf("── %s · %s · PDU #%d · %s ──", eventProtocolName(e), label, e.ID, e.SourceBytes.Kind), style)
		add(applicationEventSummary(e), inkText)
		if e.Error != "" || e.ExpertCode != "" {
			add(e.Status+": "+e.ExpertCode+" · "+e.Error, inkRed)
			continue
		}
		for _, header := range mapList(eventValues(e)["Headers"]) {
			add(textValue(header["Name"])+": "+textValue(header["Value"]), inkText)
		}
		messages := mapList(eventValues(e)["GRPC Messages"])
		for i, message := range messages {
			data, kind := grpcMessageData(message)
			add(fmt.Sprintf("gRPC message %d · %d bytes · %s · protobuf schema not supplied", i+1, len(data), kind), style)
			appendStreamData(&rows, data[:min(len(data), 4096)], 0, mode, width, style)
			if len(data) > 4096 {
				add("[Message preview limited to 4096 bytes; select its field for retained message bytes.]", inkGold)
			}
		}
		if len(messages) == 0 && e.Protocol != "tls" && e.Protocol != "http2" && e.Protocol != "grpc" && e.Protocol != "quic" && e.Protocol != "http3" && e.Protocol != "doq" && eventValues(e)["Encrypted"] != true && len(e.Raw) > 0 {
			appendStreamData(&rows, e.Raw[:min(len(e.Raw), 4096)], 0, mode, width, style)
		}
		if len(rows) > 100000 {
			add("[View row limit reached.]", inkGold)
			break
		}
	}
	if len(v.events) == 0 {
		add("No retained decoded messages. a switches to the original wire stream.", inkMuted)
	}
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
