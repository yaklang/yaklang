package pcaputil

import (
	"encoding/binary"
	"fmt"
)

const (
	zookeeperMaxConnectPassword = 64
	zookeeperMaxPacketBytes     = 1 << 20
	zookeeperOpcodeGetData      = int32(4)
	zookeeperOpcodeGetChildren  = int32(8)
	zookeeperOpcodePing         = int32(11)
	zookeeperOpcodeCloseSession = int32(-11)
)

type zookeeperPending struct {
	opcode int32
	path   string
}

type binZooKeeper struct {
	clientDir           int
	connectRequestSeen  bool
	connectResponseSeen bool
	connected           bool
	pingOutstanding     bool
	sessionID           uint64
	pending             map[int32]zookeeperPending
	maxPending          int
}

func probeZooKeeper(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 4 {
		return probeNeed("zookeeper", "jute-connect-v0", len(w), 4)
	}
	// PostgreSQL's untyped startup, SSL, GSSENC, and CancelRequest packets
	// also begin with a four-byte length. Their following request code is
	// distinct from both ZooKeeper ConnectRequest (protocol version 0) and
	// ConnectResponse (a bounded session timeout).
	if len(w) >= 8 {
		switch binary.BigEndian.Uint32(w[4:8]) {
		case 196608, 80877102, 80877103, 80877104:
			return ProbeResult{Verdict: ProbeReject, Reason: "PostgreSQL startup packet is not ZooKeeper"}
		}
	}
	bodyLen := int(binary.BigEndian.Uint32(w[:4]))
	if bodyLen < 20 || bodyLen > zookeeperMaxConnectPassword+29 || bodyLen+4 > limit {
		return ProbeResult{Verdict: ProbeReject}
	}
	total := bodyLen + 4
	if len(w) < total {
		return probeNeed("zookeeper", "jute-connect-v0", len(w), total)
	}
	body := w[4:total]
	if _, ok := parseZooKeeperConnectRequest(body); ok {
		return probeAccept("zookeeper", "jute-connect-v0/ConnectRequest", 98)
	}
	if fields, ok := parseZooKeeperConnectResponse(body); ok && fields["Session ID"].(uint64) != 0 {
		return probeAccept("zookeeper", "jute-connect-v0/ConnectResponse", 98)
	}
	return ProbeResult{Verdict: ProbeReject, Reason: "not a supported ZooKeeper Connect handshake"}
}

func parseZooKeeperConnectRequest(body []byte) (map[string]any, bool) {
	if len(body) < 28 || int32(binary.BigEndian.Uint32(body[:4])) != 0 ||
		int64(binary.BigEndian.Uint64(body[16:24])) != 0 {
		return nil, false
	}
	timeout := binary.BigEndian.Uint32(body[12:16])
	passLen := int32(binary.BigEndian.Uint32(body[24:28]))
	if !validZooKeeperSessionTimeout(timeout) || passLen < 0 || passLen > zookeeperMaxConnectPassword {
		return nil, false
	}
	base := 28 + int(passLen)
	if len(body) != base && len(body) != base+1 {
		return nil, false
	}
	fields := map[string]any{
		"Packet Name":       "Connect Request",
		"Protocol Version":  int32(0),
		"Last ZXID Seen":    binary.BigEndian.Uint64(body[4:12]),
		"Timeout (ms)":      timeout,
		"Session ID":        uint64(0),
		"Password Length":   passLen,
		"Password":          "[redacted]",
		"Read Only Present": len(body) == base+1,
	}
	if len(body) == base+1 {
		if body[base] > 1 {
			return nil, false
		}
		fields["Read Only"] = body[base] == 1
	}
	return fields, true
}

func parseZooKeeperConnectResponse(body []byte) (map[string]any, bool) {
	if len(body) < 20 || int32(binary.BigEndian.Uint32(body[:4])) != 0 {
		return nil, false
	}
	timeout := binary.BigEndian.Uint32(body[4:8])
	sessionID := binary.BigEndian.Uint64(body[8:16])
	passLen := int32(binary.BigEndian.Uint32(body[16:20]))
	if !validZooKeeperSessionTimeout(timeout) || sessionID == 0 || passLen < 0 || passLen > zookeeperMaxConnectPassword {
		return nil, false
	}
	base := 20 + int(passLen)
	if len(body) != base && len(body) != base+1 {
		return nil, false
	}
	fields := map[string]any{
		"Packet Name":       "Connect Response",
		"Protocol Version":  int32(0),
		"Timeout (ms)":      timeout,
		"Session ID":        sessionID,
		"Password Length":   passLen,
		"Password":          "[redacted]",
		"Read Only Present": len(body) == base+1,
	}
	if len(body) == base+1 {
		if body[base] > 1 {
			return nil, false
		}
		fields["Read Only"] = body[base] == 1
	}
	return fields, true
}

func (f *binFlow) frameZooKeeper(w []byte) (int, *binSpec, error) {
	if len(w) < 4 {
		return 0, nil, nil
	}
	bodyLen := int(binary.BigEndian.Uint32(w[:4]))
	if bodyLen < 8 {
		return 0, nil, protocolError(ErrMalformedMessage, "ZooKeeper Jute packet is shorter than a request/response header")
	}
	n := bodyLen + 4
	if n > zookeeperMaxPacketBytes || n > f.a.config.MaxMessageBytes {
		return n, nil, nil
	}
	if len(w) < n {
		return 0, nil, nil
	}
	return n, &binSpec{}, nil
}

func (s *binZooKeeper) consume(dir int, wire []byte, maxPending int) (map[string]any, error) {
	if len(wire) < 12 || int(binary.BigEndian.Uint32(wire[:4]))+4 != len(wire) {
		return nil, protocolError(ErrMalformedMessage, "ZooKeeper Jute frame length does not match the complete packet")
	}
	body := wire[4:]
	if dir == s.clientDir && !s.connectRequestSeen {
		fields, ok := parseZooKeeperConnectRequest(body)
		if !ok {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper ConnectRequest is outside the supported v0 profile")
		}
		s.connectRequestSeen = true
		return fields, nil
	}
	if dir != s.clientDir && !s.connectResponseSeen && !s.connected {
		fields, ok := parseZooKeeperConnectResponse(body)
		if !ok {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper ConnectResponse is outside the supported v0 profile")
		}
		s.connectResponseSeen = true
		s.connected = true
		s.sessionID = fields["Session ID"].(uint64)
		fields["Handshake Status"] = "connected"
		return fields, nil
	}
	if !s.connected {
		return nil, sessionContext("ZooKeeper operation observed before a validated ConnectResponse")
	}
	if s.pending == nil {
		s.pending = make(map[int32]zookeeperPending)
	}
	if maxPending <= 0 {
		maxPending = 4096
	}
	s.maxPending = maxPending
	if dir == s.clientDir {
		return s.consumeRequest(body)
	}
	return s.consumeResponse(body)
}

func (s *binZooKeeper) consumeRequest(body []byte) (map[string]any, error) {
	if len(body) < 8 {
		return nil, protocolError(ErrMalformedMessage, "ZooKeeper request header is truncated")
	}
	xid := int32(binary.BigEndian.Uint32(body[:4]))
	opcode := int32(binary.BigEndian.Uint32(body[4:8]))
	fields := map[string]any{"Packet Name": "Request", "XID": xid, "Opcode": opcode, "Session ID": s.sessionID}
	var path string
	switch opcode {
	case zookeeperOpcodePing:
		if xid != -2 || len(body) != 8 {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper Ping request must use XID -2 and have no body")
		}
		fields["Packet Name"], fields["Operation"] = "Ping", "Ping"
		s.pingOutstanding = true
	case zookeeperOpcodeCloseSession:
		if xid < 0 || len(body) != 8 {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper CloseSession request must have no body")
		}
		fields["Packet Name"], fields["Operation"] = "CloseSession", "CloseSession"
	case zookeeperOpcodeGetChildren, zookeeperOpcodeGetData:
		var ok bool
		var watch bool
		path, watch, ok = parseZooKeeperPathAndWatch(body[8:])
		if !ok || xid < 0 {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper path request has invalid Jute path/watch fields")
		}
		fields["Path"], fields["Watch"] = path, watch
		if opcode == zookeeperOpcodeGetChildren {
			fields["Packet Name"], fields["Operation"] = "GetChildren", "GetChildren"
		} else {
			fields["Packet Name"], fields["Operation"] = "GetData", "GetData"
		}
	default:
		return nil, protocolError(ErrUnsupportedFeature, fmt.Sprintf("ZooKeeper opcode %d is outside the supported profile", opcode))
	}
	if opcode != zookeeperOpcodePing {
		if _, exists := s.pending[xid]; exists {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper request reuses an outstanding XID")
		}
		if len(s.pending) >= s.maxPending {
			return nil, protocolError(ErrResourceExceeded, "ZooKeeper outstanding request limit reached")
		}
		s.pending[xid] = zookeeperPending{opcode: opcode, path: path}
	}
	return fields, nil
}

func (s *binZooKeeper) consumeResponse(body []byte) (map[string]any, error) {
	if len(body) < 16 {
		return nil, protocolError(ErrMalformedMessage, "ZooKeeper response header is truncated")
	}
	xid := int32(binary.BigEndian.Uint32(body[:4]))
	zxid := binary.BigEndian.Uint64(body[4:12])
	errCode := int32(binary.BigEndian.Uint32(body[12:16]))
	fields := map[string]any{"Packet Name": "Response", "XID": xid, "ZXID": zxid, "Error Code": errCode, "Session ID": s.sessionID}
	if errCode != 0 {
		if len(body) != 16 {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper error response carries an unsupported payload")
		}
		if pending, ok := s.pending[xid]; ok {
			fields["Operation"] = zookeeperOperationName(pending.opcode)
			fields["Path"] = pending.path
			delete(s.pending, xid)
		}
		return fields, nil
	}
	pending, matched := s.pending[xid]
	if matched {
		fields["Operation"], fields["Path"], fields["Request Association"] = zookeeperOperationName(pending.opcode), pending.path, "matched"
		delete(s.pending, xid)
	} else {
		fields["Request Association"] = "unmatched"
	}
	switch {
	case matched && pending.opcode == zookeeperOpcodeGetChildren:
		children, ok := parseZooKeeperChildren(body[16:])
		if !ok {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper GetChildren response has invalid Jute vector")
		}
		fields["Packet Name"], fields["Children"] = "GetChildren Response", children
	case matched && pending.opcode == zookeeperOpcodeGetData:
		value, stat, ok := parseZooKeeperData(body[16:])
		if !ok {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper GetData response has invalid buffer/stat fields")
		}
		fields["Packet Name"], fields["Value"], fields["Value Length"] = "GetData Response", value, len(value)
		fields["Stat Data Length"] = stat
	case matched && pending.opcode == zookeeperOpcodeCloseSession:
		if len(body) != 16 {
			return nil, protocolError(ErrMalformedMessage, "ZooKeeper CloseSession response has an unexpected payload")
		}
		fields["Packet Name"] = "CloseSession Response"
	case !matched && s.pingOutstanding && len(body) == 16 && (xid == 0 || xid == -2):
		// ZooKeeper Ping responses do not carry the request XID in the same
		// way as ordinary operations. Keep this as a response observation
		// without claiming a transaction match.
		fields["Packet Name"], fields["Operation"], fields["Request Association"] = "Ping Response", "Ping", "observed"
		s.pingOutstanding = false
	default:
		return nil, protocolError(ErrUnsupportedFeature, "ZooKeeper response is not associated with a supported request")
	}
	return fields, nil
}

func parseZooKeeperPathAndWatch(body []byte) (string, bool, bool) {
	if len(body) < 5 {
		return "", false, false
	}
	n := int32(binary.BigEndian.Uint32(body[:4]))
	if n <= 0 || n > 4096 || int(n)+5 != len(body) {
		return "", false, false
	}
	path := body[4 : 4+int(n)]
	watch := body[4+int(n)]
	if len(path) == 0 || path[0] != '/' || watch > 1 {
		return "", false, false
	}
	for _, c := range path {
		if c == 0 {
			return "", false, false
		}
	}
	return string(path), watch == 1, true
}

func parseZooKeeperChildren(body []byte) ([]string, bool) {
	if len(body) < 4 {
		return nil, false
	}
	count := int32(binary.BigEndian.Uint32(body[:4]))
	if count < 0 || count > 4096 {
		return nil, false
	}
	children := make([]string, 0, count)
	offset := 4
	for i := int32(0); i < count; i++ {
		if offset+4 > len(body) {
			return nil, false
		}
		n := int32(binary.BigEndian.Uint32(body[offset : offset+4]))
		offset += 4
		if n < 0 || n > 4096 || offset+int(n) > len(body) {
			return nil, false
		}
		children = append(children, string(body[offset:offset+int(n)]))
		offset += int(n)
	}
	return children, offset == len(body)
}

func parseZooKeeperData(body []byte) (string, int32, bool) {
	if len(body) < 4 {
		return "", 0, false
	}
	n := int32(binary.BigEndian.Uint32(body[:4]))
	if n < 0 || n > 1<<20 || 4+int(n)+68 != len(body) {
		return "", 0, false
	}
	stat := body[4+int(n):]
	dataLength := int32(binary.BigEndian.Uint32(stat[52:56]))
	if dataLength != n {
		return "", 0, false
	}
	return string(body[4 : 4+int(n)]), dataLength, true
}

func zookeeperOperationName(opcode int32) string {
	switch opcode {
	case zookeeperOpcodeGetData:
		return "GetData"
	case zookeeperOpcodeGetChildren:
		return "GetChildren"
	case zookeeperOpcodePing:
		return "Ping"
	case zookeeperOpcodeCloseSession:
		return "CloseSession"
	default:
		return "Unsupported"
	}
}
