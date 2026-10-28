package pcaputil

import (
	"bytes"
	"encoding/hex"
	"strings"
)

// Type5 association is deliberately local to one complete UDP datagram. No
// sequence label establishes a persistent generation, sender identity or voyage.
func nmeaAISCompleteType5(w []byte, maxElements, maxDepth int) (map[string]any, error) {
	if maxElements < 2 || maxDepth < 2 {
		return nil, protocolError(ErrResourceExceeded, "AIS Type5 two-fragment projection exceeds budget")
	}
	var fragments [2][]string
	var checks [2]int
	for i := 0; i < 2; i++ {
		end := bytes.IndexByte(w, '\n')
		if end < 0 {
			return nil, protocolError(ErrMalformedMessage, "AIS datagram ends within a required fragment")
		}
		raw := w[:end+1]
		w = w[end+1:]
		if len(raw) < 10 || len(raw) > 82 || raw[0] != '!' || raw[len(raw)-2] != '\r' {
			return nil, protocolError(ErrMalformedMessage, "AIS fragment framing is invalid")
		}
		body := raw[1 : len(raw)-2]
		star := len(body) - 3
		if star < 1 || body[star] != '*' || bytes.IndexByte(body[:star], '*') >= 0 {
			return nil, protocolError(ErrMalformedMessage, "AIS checksum suffix is invalid")
		}
		for _, c := range body {
			if c < 32 || c > 126 {
				return nil, protocolError(ErrMalformedMessage, "AIS fragment is not printable ASCII")
			}
		}
		hi, lo := nmeaHex(body[star+1]), nmeaHex(body[star+2])
		if hi < 0 || lo < 0 {
			return nil, protocolError(ErrMalformedMessage, "AIS checksum is not hexadecimal")
		}
		var sum byte
		for _, c := range body[:star] {
			sum ^= c
		}
		if sum != byte(hi*16+lo) {
			return nil, protocolError(ErrMalformedMessage, "AIS XOR checksum mismatch")
		}
		p := strings.Split(string(body[:star]), ",")
		if err := nmeaAISEnvelope(p); err != nil {
			return nil, err
		}
		if len(p) != 7 || p[0] != "AIVDM" && p[0] != "AIVDO" || p[1] != "2" {
			return nil, protocolError(ErrUnsupportedFeature, "AIS Type5 requires two AIVDM/AIVDO fragments")
		}
		if p[2] != string(byte('1'+i)) {
			return nil, protocolError(ErrContextRequired, "AIS fragments must be ordered one then two")
		}
		fragments[i], checks[i] = p, int(sum)
		if i == 0 && len(w) == 0 {
			return nil, protocolError(ErrContextRequired, "AIS fragment has no partner in this datagram")
		}
	}
	if len(w) != 0 {
		return nil, protocolError(ErrUnsupportedFeature, "AIS Type5 datagram must contain exactly its two fragments")
	}
	a, b := fragments[0], fragments[1]
	if a[0] != b[0] || a[3] != b[3] || a[4] != b[4] {
		return nil, protocolError(ErrContextRequired, "AIS fragment labels do not establish one report")
	}
	if a[6] != "0" || b[6] != "2" {
		return nil, protocolError(ErrMalformedMessage, "AIS Type5 requires zero nonfinal and two final fill bits")
	}
	armored := a[5] + b[5]
	if len(armored) != 71 {
		return nil, protocolError(ErrMalformedMessage, "AIS Type5 requires 424 payload bits")
	}
	var bits [53]byte
	for i, c := range []byte(armored) {
		v := c - 48
		if v > 40 {
			v -= 8
		}
		for j := 0; j < 6 && i*6+j < 424; j++ {
			at := i*6 + j
			bits[at/8] |= ((v >> uint(5-j)) & 1) << uint(7-at%8)
		}
	}
	u := func(start, width int) int {
		v := 0
		for i := start; i < start+width; i++ {
			v = v<<1 | int((bits[i/8]>>uint(7-i%8))&1)
		}
		return v
	}
	if u(0, 6) != 5 {
		return nil, protocolError(ErrUnsupportedFeature, "AIS two-fragment payload is outside Type5")
	}
	month, day, hour, minute := u(274, 4), u(278, 5), u(283, 5), u(288, 6)
	if month > 12 || hour > 24 || minute > 60 {
		return nil, protocolError(ErrMalformedMessage, "AIS Type5 ETA contains reserved values")
	}
	text := func(at, n int) string {
		const alphabet = "@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_ !\"#$%&'()*+,-./0123456789:;<=>?"
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(alphabet[u(at+i*6, 6)])
		}
		return b.String()
	}
	fillValue := armored[70] - 48
	if fillValue > 40 {
		fillValue -= 8
	}
	context := "received"
	if a[0] == "AIVDO" {
		context = "own-ship-reported"
	}
	out := map[string]any{"Packet Name": a[0][2:], "Sentence ID": a[0], "Talker": "AI", "Context Level": "observed", "Report Context": context, "Checksum Valid": true,
		"Message Count": 1, "Fragment Count": 2, "Assembly Scope": "single-UDP-datagram", "Sequence ID Present": a[3] != "", "Sequence ID Wire": a[3], "Radio Channel Present": a[4] != "", "Radio Channel Wire": a[4],
		"Armored Payload Wire": armored, "Fill Bits": 2, "Armoring Fill Value": int(fillValue & 3), "Payload Bit Count": 424, "Payload Hex": hex.EncodeToString(bits[:]),
		"Fragments":    []map[string]any{{"Fragment Number": 1, "Armored Payload Wire": a[5], "Fill Bits": 0, "Checksum": checks[0]}, {"Fragment Number": 2, "Armored Payload Wire": b[5], "Fill Bits": 2, "Checksum": checks[1]}},
		"Message Type": 5, "Repeat Indicator": u(6, 2), "MMSI": u(8, 30), "AIS Version Raw": u(38, 2), "IMO Number Raw": u(40, 30), "IMO Number Available": u(40, 30) != 0,
		"Ship Type Raw": u(232, 8), "Fix Type Raw": u(270, 4),
		"ETA Month Raw": month, "ETA Day Raw": day, "ETA Hour Raw": hour, "ETA Minute Raw": minute, "ETA Month Available": month != 0, "ETA Day Available": day != 0, "ETA Hour Available": hour != 24, "ETA Minute Available": minute != 60,
		"Draught Raw": u(294, 8), "Draught Available": u(294, 8) != 0, "Draught Lower Bound": u(294, 8) == 255, "DTE Raw": u(422, 1), "DTE Ready": u(422, 1) == 0, "Spare Bit": u(423, 1)}
	for _, dim := range []struct {
		name      string
		at, width int
	}{{"Bow", 240, 9}, {"Stern", 249, 9}, {"Port", 258, 6}, {"Starboard", 264, 6}} {
		v := u(dim.at, dim.width)
		name := "Dimension To " + dim.name
		out[name+" Raw"] = v
		out[name+" Available"] = v != 0
		out[name+" Lower Bound"] = v == (1<<uint(dim.width))-1
		if v != 0 {
			out[name+" Meters"] = v
		}
	}
	if u(294, 8) != 0 {
		out["Draught Meters"] = float64(u(294, 8)) / 10
	}
	for _, f := range []struct {
		name  string
		at, n int
	}{{"Call Sign", 70, 7}, {"Ship Name", 112, 20}, {"Destination", 302, 20}} {
		v := text(f.at, f.n)
		out[f.name+" Wire"] = v
		normalized, _, _ := strings.Cut(v, "@")
		normalized = strings.TrimRight(normalized, " ")
		out[f.name] = normalized
		out[f.name+" Available"] = normalized != ""
	}
	return out, nil
}

func nmeaAIS5Candidate(w []byte) bool {
	if !bytes.HasPrefix(w, []byte("!AIVDM,2,")) && !bytes.HasPrefix(w, []byte("!AIVDO,2,")) {
		return false
	}
	end := bytes.IndexByte(w, '\n')
	if end < 0 || end >= 82 {
		return false
	}
	p := strings.Split(string(w[1:end]), ",")
	return len(p) == 7 && len(p[5]) > 0 && p[5][0] == '5'
}
