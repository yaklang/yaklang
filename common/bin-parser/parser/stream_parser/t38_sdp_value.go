package stream_parser

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Factual SDP advertisement profile, not an IFP/UDPTL/RTP media decoder.
// ITU-T T.38 (11/2015) D.2.3.1, D.2.3.3 and V.3.3-4;
// RFC 8866 sections 5/9, RFC 3407 section 3, RFC 3525 section 7.1.8.
const t38SDPMaxBytes = 65536
const t38SDPMaxLines = 1024
const t38SDPMaxLine = 4096

type t38SDPField struct {
	Name, Type string
	Start, End int
	Info       map[string]any
	Children   []t38SDPField
}

func t38SDPLeaf(name, typ string, start, end int) t38SDPField {
	return t38SDPField{Name: name, Type: typ, Start: start, End: end}
}

func t38SDPDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, b := range []byte(s) {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

func t38SDPUint(s string, max uint64) (uint64, bool) {
	if !t38SDPDigits(s) {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil && n <= max
}

// RFC 8866 token; protocol is a slash-separated sequence of these tokens.
func t38SDPToken(s string) bool {
	if s == "" {
		return false
	}
	for _, b := range []byte(s) {
		if !(b == 0x21 || b >= 0x23 && b <= 0x27 || b >= 0x2a && b <= 0x2b || b >= 0x2d && b <= 0x2e || b >= 0x30 && b <= 0x39 || b >= 0x41 && b <= 0x5a || b >= 0x5e && b <= 0x7e) {
			return false
		}
	}
	return true
}

func t38SDPProto(s string) bool {
	for _, p := range strings.Split(s, "/") {
		if !t38SDPToken(p) {
			return false
		}
	}
	return true
}

func t38SDPWords(s string, start int, names []string) ([]t38SDPField, []string, error) {
	parts := strings.Split(s, " ")
	if len(parts) != len(names) {
		return nil, nil, fmt.Errorf("t38-sdp: wrong field count")
	}
	fields := []t38SDPField{}
	for i, part := range parts {
		if part == "" || strings.ContainsAny(part, "\t\r\n") {
			return nil, nil, fmt.Errorf("t38-sdp: empty or non-SP-delimited field")
		}
		if i > 0 {
			fields = append(fields, t38SDPLeaf("Separator", "raw", start-1, start))
		}
		fields = append(fields, t38SDPLeaf(names[i], "string", start, start+len(part)))
		start += len(part) + 1
	}
	return fields, parts, nil
}

func t38SDPT38Tuple(media, proto string, formats []string) bool {
	// T.38 V.3.4 documents both IANA UDPTL and ITU udptl spellings.
	if media != "image" || !(proto == "udptl" || proto == "UDPTL" || proto == "tcp" || proto == "TCP") {
		return false
	}
	for _, f := range formats {
		if f == "t38" {
			return true
		}
	}
	return false
}

type t38SDPMedia struct {
	media, proto string
	formats      []string
	connection   bool
	t38          bool
	attrs        map[string]bool
}

type t38SDPSession struct {
	field            t38SDPField
	seen             map[byte]bool
	rank, mediaRank  int
	media            []t38SDPMedia
	sqn, pendingCDSC bool
	attrs            map[string]bool
}

func decodeT38SDPAdvertisement(wire []byte, h248 bool) ([]t38SDPField, map[string]any, error) {
	fields, info, err := decodeT38SDPAdvertisementInner(wire, h248)
	if err != nil {
		return nil, nil, err
	}
	return fields, info, nil
}

func decodeT38SDPAdvertisementInner(wire []byte, h248 bool) ([]t38SDPField, map[string]any, error) {
	if len(wire) == 0 || len(wire) > t38SDPMaxBytes {
		return nil, nil, fmt.Errorf("t38-sdp: message must be 1..65536 bytes")
	}
	for _, b := range wire {
		if b == 0 || b < 32 && b != '\t' && b != '\r' && b != '\n' || b == 127 {
			return nil, nil, fmt.Errorf("t38-sdp: invalid text octet")
		}
	}
	start, end := 0, len(wire)
	fields := []t38SDPField{}
	if h248 {
		// These bytes belong to the Local/Remote descriptor, not an SDP line.
		for start < end && strings.ContainsRune(" \t\r\n", rune(wire[start])) {
			start++
		}
		for end > start && (wire[end-1] == ' ' || wire[end-1] == '\t') {
			end--
		}
		if start > 0 {
			fields = append(fields, t38SDPLeaf("Descriptor Leading Whitespace", "raw", 0, start))
		}
	}
	if start == end {
		return nil, nil, fmt.Errorf("t38-sdp: no SDP advertisement")
	}
	var session *t38SDPSession
	mediaAds, capabilityAds, sessions, lines, mediaCount := 0, 0, 0, 0, 0
	newSession := func(at int) {
		session = &t38SDPSession{field: t38SDPField{Name: fmt.Sprintf("SDP Session %d", sessions), Start: at}, seen: map[byte]bool{}, attrs: map[string]bool{}, rank: -1, mediaRank: -1}
		sessions++
	}
	finishSession := func(at int) error {
		if session == nil {
			return nil
		}
		if !h248 && (!session.seen['v'] || !session.seen['o'] || !session.seen['s'] || !session.seen['t']) {
			return fmt.Errorf("t38-sdp: full session requires v/o/s/t")
		}
		if len(session.media) == 0 {
			return fmt.Errorf("t38-sdp: session has no media description")
		}
		if session.pendingCDSC {
			return fmt.Errorf("t38-sdp: sqn requires immediately following cdsc")
		}
		for _, m := range session.media {
			if !session.seen['c'] && !m.connection {
				return fmt.Errorf("t38-sdp: media has no connection field")
			}
		}
		session.field.End = at
		session.field.Info = map[string]any{"Media Count": len(session.media), "Capability Set Sequence Present": session.sqn, "H248 Alternatives": h248}
		fields = append(fields, session.field)
		return nil
	}
	for at := start; at < end; {
		if lines >= t38SDPMaxLines {
			return nil, nil, fmt.Errorf("t38-sdp: 1024-line profile limit")
		}
		next := bytes.Index(wire[at:end], []byte("\r\n"))
		if next < 0 {
			return nil, nil, fmt.Errorf("t38-sdp: missing complete CRLF line")
		}
		stop := at + next
		if stop+2-at > t38SDPMaxLine {
			return nil, nil, fmt.Errorf("t38-sdp: 4096-byte line profile limit")
		}
		line := string(wire[at:stop])
		if len(line) < 3 || line[1] != '=' || strings.ContainsAny(line, "\r\n") {
			return nil, nil, fmt.Errorf("t38-sdp: malformed SDP line")
		}
		key, value, valueAt := line[0], line[2:], at+2
		if key == 'v' && session != nil {
			if !h248 {
				return nil, nil, fmt.Errorf("t38-sdp: multiple sessions require H248 entry")
			}
			if !session.seen['v'] {
				return nil, nil, fmt.Errorf("t38-sdp: H248 alternatives require v delimiters")
			}
			if err := finishSession(at); err != nil {
				return nil, nil, err
			}
			session = nil
		}
		if session == nil {
			if sessions >= 32 {
				return nil, nil, fmt.Errorf("t38-sdp: 32-alternative profile limit")
			}
			if !h248 && key != 'v' {
				return nil, nil, fmt.Errorf("t38-sdp: full session must start with v=0")
			}
			newSession(at)
		}
		f := t38SDPField{Name: fmt.Sprintf("SDP Line %d", lines), Start: at, End: stop + 2, Info: map[string]any{"Field": string(key), "Media Index": len(session.media) - 1}}
		f.Children = append(f.Children, t38SDPLeaf("Field Type", "string", at, at+1), t38SDPLeaf("Equals", "raw", at+1, at+2))
		if session.pendingCDSC && !(key == 'a' && strings.HasPrefix(value, "cdsc:")) {
			return nil, nil, fmt.Errorf("t38-sdp: sqn requires immediately following cdsc")
		}
		// Deliberately bounded observed line profile; unsupported SDP line kinds
		// are not silently accepted as a complete generic SDP implementation.
		if key == 'm' {
			if h248 && len(session.media) != 0 {
				return nil, nil, fmt.Errorf("t38-sdp: H248 session allows one media description")
			}
			if mediaCount >= 64 {
				return nil, nil, fmt.Errorf("t38-sdp: 64-media profile limit")
			}
			session.mediaRank = 0
		} else if len(session.media) == 0 {
			rank := strings.IndexByte("vosicbta", key)
			if rank < 0 {
				return nil, nil, fmt.Errorf("t38-sdp: unsupported session line %c", key)
			}
			if rank < session.rank || session.seen[key] && key != 'a' && key != 'b' && key != 't' {
				return nil, nil, fmt.Errorf("t38-sdp: duplicate or out-of-order session line")
			}
			session.rank = rank
			session.seen[key] = true
		} else {
			rank := strings.IndexByte("micba", key)
			if rank < 1 || rank < session.mediaRank {
				return nil, nil, fmt.Errorf("t38-sdp: unsupported or out-of-order media line")
			}
			if (key == 'i' || key == 'c') && session.mediaRank == rank {
				return nil, nil, fmt.Errorf("t38-sdp: repeated media information/connection unsupported")
			}
			session.mediaRank = rank
		}
		switch key {
		case 'v':
			if value != "0" {
				return nil, nil, fmt.Errorf("t38-sdp: unsupported SDP version")
			}
			f.Children = append(f.Children, t38SDPLeaf("SDP Version", "string", valueAt, stop))
		case 'o':
			children, parts, err := t38SDPWords(value, valueAt, []string{"Origin Username", "Session ID", "Session Version", "Origin Network Type", "Origin Address Type", "Origin Address"})
			if err != nil {
				return nil, nil, err
			}
			if !(t38SDPDigits(parts[1]) || h248 && parts[1] == "$") || !(t38SDPDigits(parts[2]) || h248 && parts[2] == "$") || !t38SDPToken(parts[3]) || !t38SDPToken(parts[4]) {
				return nil, nil, fmt.Errorf("t38-sdp: malformed origin fields")
			}
			f.Children = append(f.Children, children...)
			f.Info["Address Semantics Validated"] = false
		case 's', 'i':
			name := "Session Name"
			if key == 'i' {
				name = "Information"
			}
			f.Children = append(f.Children, t38SDPLeaf(name, "string", valueAt, stop))
		case 'c':
			children, parts, err := t38SDPWords(value, valueAt, []string{"Connection Network Type", "Connection Address Type", "Connection Address"})
			if err != nil {
				return nil, nil, err
			}
			if !t38SDPToken(parts[0]) || !t38SDPToken(parts[1]) || !h248 && parts[2] == "$" {
				return nil, nil, fmt.Errorf("t38-sdp: malformed connection fields or CHOOSE outside H248")
			}
			f.Children = append(f.Children, children...)
			f.Info["Address Semantics Validated"] = false
			f.Info["Address CHOOSE"] = parts[2] == "$"
			if len(session.media) > 0 {
				session.media[len(session.media)-1].connection = true
			}
		case 'b':
			colon := strings.IndexByte(value, ':')
			if colon < 1 || !t38SDPToken(value[:colon]) || !t38SDPDigits(value[colon+1:]) {
				return nil, nil, fmt.Errorf("t38-sdp: malformed bandwidth")
			}
			f.Children = append(f.Children, t38SDPLeaf("Bandwidth Type", "string", valueAt, valueAt+colon), t38SDPLeaf("Colon", "raw", valueAt+colon, valueAt+colon+1), t38SDPLeaf("Bandwidth", "string", valueAt+colon+1, stop))
		case 't':
			children, parts, err := t38SDPWords(value, valueAt, []string{"Start Time", "Stop Time"})
			if err != nil {
				return nil, nil, err
			}
			for _, p := range parts {
				if !(p == "0" || len(p) >= 10 && p[0] != '0' && t38SDPDigits(p) || h248 && p == "$") {
					return nil, nil, fmt.Errorf("t38-sdp: malformed SDP time")
				}
			}
			f.Children = append(f.Children, children...)
		case 'm':
			parts := strings.Split(value, " ")
			if len(parts) < 4 {
				return nil, nil, fmt.Errorf("t38-sdp: incomplete media description")
			}
			names := []string{"Media Type", "Media Port", "Media Transport"}
			for i := 3; i < len(parts); i++ {
				names = append(names, fmt.Sprintf("Media Format %d", i-3))
			}
			children, _, err := t38SDPWords(value, valueAt, names)
			if err != nil {
				return nil, nil, err
			}
			if !t38SDPToken(parts[0]) || !t38SDPProto(parts[2]) {
				return nil, nil, fmt.Errorf("t38-sdp: malformed media/transport token")
			}
			for _, p := range parts[3:] {
				if !t38SDPToken(p) {
					return nil, nil, fmt.Errorf("t38-sdp: malformed format token")
				}
			}
			portParts := strings.Split(parts[1], "/")
			port, valid := t38SDPUint(portParts[0], 65535)
			choose := h248 && parts[1] == "$"
			if (!valid && !choose) || len(portParts) > 2 {
				return nil, nil, fmt.Errorf("t38-sdp: invalid media port")
			}
			if len(portParts) == 2 {
				if n, ok := t38SDPUint(portParts[1], 65535); !ok || n == 0 {
					return nil, nil, fmt.Errorf("t38-sdp: invalid media port count")
				}
			}
			isT38 := t38SDPT38Tuple(parts[0], parts[2], parts[3:])
			if isT38 {
				mediaAds++
			}
			f.Info["T38 Media Advertisement"] = isT38
			f.Info["Port CHOOSE"], f.Info["Port Zero"] = choose, !choose && port == 0
			if !choose {
				f.Info["Port Number"] = port
			}
			f.Info["Media Index"] = len(session.media)
			f.Children = append(f.Children, children...)
			session.media = append(session.media, t38SDPMedia{media: parts[0], proto: parts[2], formats: parts[3:], t38: isT38, attrs: map[string]bool{}})
			mediaCount++
		case 'a':
			colon := strings.IndexByte(value, ':')
			name, av, avAt := value, "", stop
			if colon >= 0 {
				name, av, avAt = value[:colon], value[colon+1:], valueAt+colon+1
			}
			if !t38SDPToken(name) {
				return nil, nil, fmt.Errorf("t38-sdp: invalid attribute name")
			}
			f.Children = append(f.Children, t38SDPLeaf("Attribute Name", "string", valueAt, valueAt+len(name)))
			if colon >= 0 {
				f.Children = append(f.Children, t38SDPLeaf("Colon", "raw", avAt-1, avAt))
			}
			f.Info["Attribute Semantics Decoded"] = false
			if name == "sqn" || name == "cdsc" {
				trimmed := strings.TrimLeft(av, " ")
				lead := len(av) - len(trimmed)
				if lead > 0 {
					f.Children = append(f.Children, t38SDPLeaf("Attribute Whitespace", "raw", avAt, avAt+lead))
				}
				avAt += lead
				av = trimmed
				if name == "sqn" {
					n, ok := t38SDPUint(av, 255)
					if !ok || session.sqn {
						return nil, nil, fmt.Errorf("t38-sdp: invalid or duplicate capability sequence")
					}
					session.sqn, session.pendingCDSC = true, true
					f.Children = append(f.Children, t38SDPLeaf("Capability Sequence", "string", avAt, stop))
					f.Info["Sequence Number"] = n
				} else {
					parts := strings.Split(av, " ")
					if !session.sqn || len(parts) < 4 {
						return nil, nil, fmt.Errorf("t38-sdp: capability requires sequence and complete tuple")
					}
					n, ok := t38SDPUint(parts[0], 255)
					if !ok || n == 0 || !t38SDPToken(parts[1]) || !t38SDPProto(parts[2]) {
						return nil, nil, fmt.Errorf("t38-sdp: invalid capability tuple")
					}
					names := []string{"Capability Number", "Capability Media", "Capability Transport"}
					for i, p := range parts[3:] {
						if !t38SDPToken(p) {
							return nil, nil, fmt.Errorf("t38-sdp: invalid capability format")
						}
						names = append(names, fmt.Sprintf("Capability Format %d", i))
					}
					children, _, err := t38SDPWords(av, avAt, names)
					if err != nil {
						return nil, nil, err
					}
					f.Children = append(f.Children, children...)
					isT38 := t38SDPT38Tuple(parts[1], parts[2], parts[3:])
					if isT38 {
						capabilityAds++
					}
					f.Info["T38 Capability Advertisement"] = isT38
					f.Info["Capability Number"] = n
					session.pendingCDSC = false
				}
				f.Info["Attribute Semantics Decoded"] = true
			} else {
				known, info, err := t38SDPAttribute(name, av, colon >= 0)
				if err != nil {
					return nil, nil, err
				}
				if !known && colon >= 0 && av == "" {
					return nil, nil, fmt.Errorf("t38-sdp: empty attribute value")
				}
				if known {
					attrs := session.attrs
					if len(session.media) > 0 {
						attrs = session.media[len(session.media)-1].attrs
					}
					canonical := name
					if name == "T38maxBitRate" {
						canonical = "T38MaxBitRate"
					}
					if attrs[canonical] {
						return nil, nil, fmt.Errorf("t38-sdp: duplicate T38 attribute unsupported")
					}
					attrs[canonical] = true
					for k, v := range info {
						f.Info[k] = v
					}
					f.Info["Attribute Semantics Decoded"] = true
				}
				if colon >= 0 {
					f.Children = append(f.Children, t38SDPLeaf("Attribute Value", "string", avAt, stop))
				}
			}
		}
		f.Children = append(f.Children, t38SDPLeaf("Line Ending", "raw", stop, stop+2))
		session.field.Children = append(session.field.Children, f)
		lines++
		at = stop + 2
	}
	if err := finishSession(end); err != nil {
		return nil, nil, err
	}
	if mediaAds+capabilityAds == 0 {
		return nil, nil, fmt.Errorf("t38-sdp: no image/t38 advertisement in supported transport profile")
	}
	if end < len(wire) {
		fields = append(fields, t38SDPLeaf("Descriptor Trailing Whitespace", "raw", end, len(wire)))
	}
	return fields, map[string]any{"SDP Session Count": sessions, "SDP Media Count": mediaCount, "SDP Line Count": lines, "T38 Media Advertisement Count": mediaAds, "T38 Capability Advertisement Count": capabilityAds, "H248 Descriptor Syntax": h248, "Media Payload Decoded": false, "Negotiation Outcome Validated": false, "Endpoint Address Validated": false, "Capability Set Completeness Validated": false, "T38 Required Parameter Set Validated": false, "H248 Direction Validated": false}, nil
}

func t38SDPAttribute(name, value string, hasValue bool) (bool, map[string]any, error) {
	info := map[string]any{}
	bad := func() (bool, map[string]any, error) {
		return true, nil, fmt.Errorf("t38-sdp: malformed %s attribute", name)
	}
	switch name {
	case "T38FaxVersion", "T38MaxBitRate", "T38maxBitRate", "T38FaxMaxBuffer", "T38FaxMaxDatagram", "T38FaxMaxIFP", "T38FaxUdpFECMaxSpan":
		if !hasValue || !t38SDPDigits(value) {
			return bad()
		}
		info["Decimal Declaration"] = value
	case "T38FaxRateManagement":
		if !hasValue || (value != "localTCF" && value != "transferredTCF") {
			return bad()
		}
	case "T38FaxUdpEC":
		if !hasValue || (value != "t38UDPNoEC" && value != "t38UDPFEC" && value != "t38UDPRedundancy") {
			return bad()
		}
	case "T38FaxFillBitRemoval", "T38FaxTranscodingMMR", "T38FaxTranscodingJBIG":
		// T.38 V.3.3 says receivers treat presence as true even when older
		// equipment incorrectly appends a colon/value. Preserve that value.
		info["Boolean Present"] = true
		info["Noncanonical Boolean Value Present"] = hasValue
	case "T38FaxUdpECDepth":
		p := strings.Split(value, " ")
		if !hasValue || len(p) > 2 {
			return bad()
		}
		for _, s := range p {
			if !t38SDPDigits(s) {
				return bad()
			}
		}
	case "T38VendorInfo":
		p := strings.Split(value, " ")
		if !hasValue || len(p) != 3 {
			return bad()
		}
		for _, s := range p[:2] {
			if _, ok := t38SDPUint(s, 255); !ok {
				return bad()
			}
		}
		if !t38SDPDigits(p[2]) {
			return bad()
		}
	default:
		return false, nil, nil
	}
	return true, info, nil
}
