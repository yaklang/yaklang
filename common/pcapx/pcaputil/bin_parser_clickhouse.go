package pcaputil

import (
	"unicode/utf8"
)

const (
	clickHouseSupportedMajor    = uint64(23)
	clickHouseSupportedMinor    = uint64(8)
	clickHouseSupportedRevision = uint64(54401)
	clickHouseSupportedPatch    = uint64(1)
	clickHouseMaxHelloBytes     = 64
	clickHouseMaxStringBytes    = 48
	clickHouseMaxPasswordBytes  = 32
)

type clickHouseReader struct {
	wire      []byte
	offset    int
	needMore  bool
	malformed bool
}

func (r *clickHouseReader) varUInt() (uint64, bool) {
	var value uint64
	for i := 0; i < 10; i++ {
		if r.offset >= len(r.wire) {
			r.needMore = true
			return 0, false
		}
		b := r.wire[r.offset]
		r.offset++
		if i == 9 && b > 1 {
			r.malformed = true
			return 0, false
		}
		value |= uint64(b&0x7f) << (7 * i)
		if b&0x80 == 0 {
			if i > 0 && b == 0 {
				r.malformed = true
				return 0, false
			}
			return value, true
		}
	}
	r.malformed = true
	return 0, false
}

func (r *clickHouseReader) bytes(max int) ([]byte, bool) {
	n, ok := r.varUInt()
	if !ok {
		return nil, false
	}
	if n > uint64(max) {
		r.malformed = true
		return nil, false
	}
	end := uint64(r.offset) + n
	if end > uint64(len(r.wire)) {
		r.needMore = true
		return nil, false
	}
	b := r.wire[r.offset:int(end)]
	r.offset = int(end)
	return b, true
}

func (r *clickHouseReader) string(max int, allowEmpty bool) (string, bool) {
	b, ok := r.bytes(max)
	if !ok {
		return "", false
	}
	s := string(b)
	if (!allowEmpty && s == "") || !utf8.ValidString(s) {
		r.malformed = true
		return "", false
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			r.malformed = true
			return "", false
		}
	}
	return s, true
}

func (r *clickHouseReader) supportedVersion() (uint64, bool) {
	major, ok := r.varUInt()
	if !ok {
		return 0, false
	}
	minor, ok := r.varUInt()
	if !ok {
		return 0, false
	}
	revision, ok := r.varUInt()
	if !ok {
		return 0, false
	}
	if major != clickHouseSupportedMajor || minor != clickHouseSupportedMinor || revision != clickHouseSupportedRevision {
		r.malformed = true
		return 0, false
	}
	return revision, true
}

type clickHouseHello struct {
	fields   map[string]any
	revision uint64
	consumed int
}

func parseClickHouseClientHello(wire []byte) (clickHouseHello, bool, bool) {
	r := &clickHouseReader{wire: wire}
	packetType, ok := r.varUInt()
	if !ok || packetType != 0 {
		if ok {
			r.malformed = true
		}
		return clickHouseHello{}, r.needMore, false
	}
	name, ok := r.string(clickHouseMaxStringBytes, false)
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	revision, ok := r.supportedVersion()
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	database, ok := r.string(clickHouseMaxStringBytes, true)
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	user, ok := r.string(clickHouseMaxStringBytes, false)
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	password, ok := r.bytes(clickHouseMaxPasswordBytes)
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	return clickHouseHello{
		fields: map[string]any{
			"Packet Name": "Hello", "Role": "client", "Client Name": name,
			"Major Version": clickHouseSupportedMajor, "Minor Version": clickHouseSupportedMinor,
			"Revision": revision, "Database": database, "User": user,
			"Password Length": len(password), "Password": "[redacted]",
		},
		revision: revision,
		consumed: r.offset,
	}, false, true
}

func parseClickHouseServerHello(wire []byte) (clickHouseHello, bool, bool) {
	r := &clickHouseReader{wire: wire}
	packetType, ok := r.varUInt()
	if !ok || packetType != 0 {
		if ok {
			r.malformed = true
		}
		return clickHouseHello{}, r.needMore, false
	}
	name, ok := r.string(clickHouseMaxStringBytes, false)
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	revision, ok := r.supportedVersion()
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	timezone, ok := r.string(clickHouseMaxStringBytes, false)
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	displayName, ok := r.string(clickHouseMaxStringBytes, false)
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	patch, ok := r.varUInt()
	if !ok {
		return clickHouseHello{}, r.needMore, false
	}
	if patch != clickHouseSupportedPatch {
		return clickHouseHello{}, false, false
	}
	return clickHouseHello{
		fields: map[string]any{
			"Packet Name": "Hello", "Role": "server", "Server Name": name,
			"Major Version": clickHouseSupportedMajor, "Minor Version": clickHouseSupportedMinor,
			"Revision": revision, "Timezone": timezone, "Display Name": displayName,
			"Version Patch": patch,
		},
		revision: revision,
		consumed: r.offset,
	}, false, true
}

func probeClickHouse(wire []byte, limit int) ProbeResult {
	if len(wire) == 0 || wire[0] != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	// A one-byte zero prefix is too common to reserve for ClickHouse. Wait
	// until a plausible non-empty client-name length is present.
	if len(wire) < 2 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if wire[1] == 0 || wire[1] > clickHouseMaxStringBytes {
		return ProbeResult{Verdict: ProbeReject}
	}
	client, _, clientOK := parseClickHouseClientHello(wire)
	if clientOK {
		if client.consumed <= limit {
			return probeAccept("clickhouse", "native-23.8-r54401/client-hello", 97)
		}
		return ProbeResult{Verdict: ProbeReject, Reason: "ClickHouse Hello exceeds probe limit"}
	}
	return ProbeResult{Verdict: ProbeReject, Reason: "not a supported ClickHouse Native Hello"}
}

type binClickHouse struct {
	clientDir       int
	clientHelloSeen bool
	serverHelloSeen bool
	connected       bool
	pingOutstanding bool
	clientRevision  uint64
	serverRevision  uint64
}

func (f *binFlow) frameClickHouse(dir int, wire []byte) (int, *binSpec, error) {
	if len(wire) == 0 {
		return 0, nil, nil
	}
	s := f.clickhouse
	if dir == s.clientDir && !s.clientHelloSeen {
		_, need, ok := parseClickHouseClientHello(wire)
		if ok {
			parsed, _, _ := parseClickHouseClientHello(wire)
			return parsed.consumed, &binSpec{}, nil
		}
		if need {
			return 0, nil, nil
		}
		return 0, nil, protocolError(ErrUnsupportedVersion, "unsupported or malformed ClickHouse client Hello")
	}
	if dir != s.clientDir && !s.serverHelloSeen {
		_, need, ok := parseClickHouseServerHello(wire)
		if ok {
			parsed, _, _ := parseClickHouseServerHello(wire)
			return parsed.consumed, &binSpec{}, nil
		}
		if need {
			return 0, nil, nil
		}
		return 0, nil, protocolError(ErrMalformedMessage, "ClickHouse server response is not a valid native Hello")
	}
	if !s.connected {
		if dir == s.clientDir && s.clientHelloSeen {
			typ, end, need, ok := clickHousePacketType(wire)
			if need {
				return 0, nil, nil
			}
			if ok && typ == 4 {
				return end, &binSpec{}, nil
			}
		}
		return 0, nil, sessionContext("ClickHouse Ping observed before both Hello messages")
	}
	typ, end, need, ok := clickHousePacketType(wire)
	if need {
		return 0, nil, nil
	}
	if !ok || typ != 4 {
		return 0, nil, protocolError(ErrUnsupportedFeature, "ClickHouse packet is outside the supported Hello/Ping profile")
	}
	return end, &binSpec{}, nil
}

func clickHousePacketType(wire []byte) (uint64, int, bool, bool) {
	r := &clickHouseReader{wire: wire}
	typ, ok := r.varUInt()
	return typ, r.offset, r.needMore, ok
}

func (s *binClickHouse) consume(dir int, wire []byte) (map[string]any, error) {
	if dir == s.clientDir && !s.clientHelloSeen {
		hello, need, ok := parseClickHouseClientHello(wire)
		if !ok {
			if need {
				return nil, protocolError(ErrNeedMore, "incomplete ClickHouse client Hello")
			}
			return nil, protocolError(ErrUnsupportedVersion, "unsupported ClickHouse client Hello")
		}
		s.clientHelloSeen, s.clientRevision = true, hello.revision
		if s.serverHelloSeen {
			if s.serverRevision != hello.revision {
				return nil, protocolError(ErrMalformedMessage, "ClickHouse client and server revisions do not match")
			}
			s.connected = true
			hello.fields["Handshake Status"] = "connected"
		}
		return hello.fields, nil
	}
	if dir != s.clientDir && !s.serverHelloSeen {
		hello, need, ok := parseClickHouseServerHello(wire)
		if !ok {
			if need {
				return nil, protocolError(ErrNeedMore, "incomplete ClickHouse server Hello")
			}
			return nil, protocolError(ErrUnsupportedVersion, "unsupported ClickHouse server Hello")
		}
		if s.clientHelloSeen && s.clientRevision != hello.revision {
			return nil, protocolError(ErrMalformedMessage, "ClickHouse client and server revisions do not match")
		}
		s.serverHelloSeen, s.serverRevision = true, hello.revision
		if s.clientHelloSeen {
			s.connected = true
			hello.fields["Handshake Status"] = "connected"
		}
		return hello.fields, nil
	}
	if !s.connected {
		if dir == s.clientDir && s.clientHelloSeen {
			typ, end, need, ok := clickHousePacketType(wire)
			if need {
				return nil, protocolError(ErrNeedMore, "incomplete ClickHouse Ping")
			}
			if !ok || end != len(wire) || typ != 4 {
				return nil, protocolError(ErrUnsupportedFeature, "ClickHouse packet is outside the supported Hello/Ping profile")
			}
			if s.pingOutstanding {
				return nil, protocolError(ErrMalformedMessage, "ClickHouse client sent another Ping before Pong")
			}
			s.pingOutstanding = true
			return map[string]any{"Packet Name": "Ping", "Request Association": "request"}, nil
		}
		if dir == s.clientDir && !s.clientHelloSeen {
			return nil, sessionContext("ClickHouse client Hello has not been observed")
		}
		if dir != s.clientDir && !s.serverHelloSeen {
			return nil, sessionContext("ClickHouse server Hello has not been observed")
		}
		return nil, sessionContext("ClickHouse Ping observed before both Hello messages")
	}
	typ, end, need, ok := clickHousePacketType(wire)
	if need {
		return nil, protocolError(ErrNeedMore, "incomplete ClickHouse packet type")
	}
	if !ok || end != len(wire) || typ != 4 {
		return nil, protocolError(ErrUnsupportedFeature, "ClickHouse packet is outside the supported Hello/Ping profile")
	}
	if dir == s.clientDir {
		if s.pingOutstanding {
			return nil, protocolError(ErrMalformedMessage, "ClickHouse client sent another Ping before Pong")
		}
		s.pingOutstanding = true
		return map[string]any{"Packet Name": "Ping", "Request Association": "request"}, nil
	}
	if !s.pingOutstanding {
		return nil, protocolError(ErrMalformedMessage, "ClickHouse Pong has no observed Ping")
	}
	s.pingOutstanding = false
	return map[string]any{"Packet Name": "Pong", "Request Association": "matched"}, nil
}
