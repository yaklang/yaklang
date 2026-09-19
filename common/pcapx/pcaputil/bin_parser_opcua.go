package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type uaKey struct {
	dir              int
	channel, request uint32
}
type uaChunk struct {
	kind  string
	token uint32
	data  []byte
	count int
}
type uaPeer struct {
	seen                           bool
	receive, send, message, chunks uint32
}
type binOPCUA struct {
	peers   [2]uaPeer
	plain   map[uint32]bool
	tokens  map[uint32]uint32
	seq     [2]map[uint32]uint32
	chunks  map[uaKey]*uaChunk
	pending map[uaKey]uint32
}

func probeOPCUA(w []byte, _ int) ProbeResult {
	if len(w) < 3 {
		return ProbeResult{Verdict: ProbeReject}
	}
	t := string(w[:3])
	switch t {
	case "HEL", "ACK", "ERR", "RHE", "OPN":
	default:
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 8 {
		return probeNeed("opcua", "tcp", len(w), 8)
	}
	if w[3] != 'F' || binary.LittleEndian.Uint32(w[4:]) < 8 {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("opcua", "tcp", 99)
}
func (f *binFlow) frameOPCUA(w []byte) (int, *binSpec, error) {
	s := f.opcua
	memory := int64(512 + (len(s.plain)+len(s.tokens)+len(s.pending)+len(s.seq[0])+len(s.seq[1]))*128 + 2*len(w))
	for _, c := range s.chunks {
		memory += 2*int64(cap(c.data)) + 128
	}
	if err := f.reserveSession(memory); err != nil {
		return 0, nil, err
	}
	if len(w) < 8 {
		return 0, nil, nil
	}
	n := uint64(binary.LittleEndian.Uint32(w[4:]))
	if n < 8 {
		return 0, nil, fmt.Errorf("opcua: message size")
	}
	if n > uint64(f.a.budget.MaxMessageBytes) {
		return 0, nil, protocolError(ErrResourceExceeded, "OPC UA message size")
	}
	return int(n), f.a.specs["session_envelopes/UATCP"], nil
}
func uaBlob(w []byte) ([]byte, []byte, error) {
	if len(w) < 4 {
		return nil, nil, fmt.Errorf("opcua: string/bytes length")
	}
	n := int32(binary.LittleEndian.Uint32(w))
	if n == -1 {
		return nil, w[4:], nil
	}
	if n < 0 || int64(n) > int64(len(w)-4) {
		return nil, nil, fmt.Errorf("opcua: string/bytes boundary")
	}
	return w[4 : 4+int(n)], w[4+int(n):], nil
}
func uaService(w []byte) (uint32, int, error) {
	if len(w) < 2 {
		return 0, 0, fmt.Errorf("opcua: service type NodeId")
	}
	switch w[0] {
	case 0:
		return uint32(w[1]), 2, nil
	case 1:
		if len(w) < 4 {
			return 0, 0, fmt.Errorf("opcua: four-byte NodeId")
		}
		if w[1] != 0 {
			return 0, 4, protocolError(ErrUnsupportedFeature, "OPC UA custom namespace service")
		}
		return uint32(binary.LittleEndian.Uint16(w[2:])), 4, nil
	case 2:
		if len(w) < 7 {
			return 0, 0, fmt.Errorf("opcua: numeric NodeId")
		}
		if binary.LittleEndian.Uint16(w[1:]) != 0 {
			return 0, 7, protocolError(ErrUnsupportedFeature, "OPC UA custom namespace service")
		}
		return binary.LittleEndian.Uint32(w[3:]), 7, nil
	}
	return 0, 0, protocolError(ErrUnsupportedFeature, "OPC UA nonnumeric service NodeId")
}
func (s *binOPCUA) consume(dir int, w []byte, max, maxBytes int) (map[string]any, error) {
	if len(w) < 8 || int(binary.LittleEndian.Uint32(w[4:])) != len(w) {
		return nil, fmt.Errorf("opcua: header")
	}
	kind := string(w[:3])
	chunk := w[3]
	b := w[8:]
	out := map[string]any{"Message Type": kind, "Chunk Type": string(chunk), "Message Size": len(w)}
	if s.plain == nil {
		s.plain = map[uint32]bool{}
		s.tokens = map[uint32]uint32{}
		s.chunks = map[uaKey]*uaChunk{}
		s.pending = map[uaKey]uint32{}
		s.seq = [2]map[uint32]uint32{{}, {}}
	}
	if kind == "HEL" || kind == "ACK" {
		if chunk != 'F' || len(b) < 20 {
			return nil, fmt.Errorf("opcua: HEL/ACK")
		}
		p := uaPeer{true, binary.LittleEndian.Uint32(b[4:]), binary.LittleEndian.Uint32(b[8:]), binary.LittleEndian.Uint32(b[12:]), binary.LittleEndian.Uint32(b[16:])}
		if p.receive < 8192 || p.send < 8192 {
			return nil, fmt.Errorf("opcua: buffers below 8192")
		}
		out["Protocol Version"] = binary.LittleEndian.Uint32(b)
		out["Receive Buffer Size"] = p.receive
		out["Send Buffer Size"] = p.send
		out["Max Message Size"] = p.message
		out["Max Chunk Count"] = p.chunks
		if kind == "HEL" {
			endpoint, rest, err := uaBlob(b[20:])
			if err != nil {
				return nil, err
			}
			if len(rest) != 0 {
				return nil, fmt.Errorf("opcua: HEL trailing bytes")
			}
			out["Endpoint URL"] = string(endpoint)
		} else if len(b) != 20 {
			return nil, fmt.Errorf("opcua: ACK trailing bytes")
		}
		s.peers[dir] = p
		return out, nil
	}
	if kind == "ERR" {
		if chunk != 'F' || len(b) < 8 {
			return nil, fmt.Errorf("opcua: ERR")
		}
		out["Status Code"] = binary.LittleEndian.Uint32(b)
		reason, rest, err := uaBlob(b[4:])
		if err != nil {
			return nil, err
		}
		if len(rest) != 0 {
			return nil, fmt.Errorf("opcua: ERR trailing bytes")
		}
		out["Reason"] = string(reason)
		return out, nil
	}
	if kind == "RHE" {
		if chunk != 'F' {
			return nil, fmt.Errorf("opcua: RHE chunk")
		}
		uri, rest, err := uaBlob(b)
		if err != nil {
			return nil, err
		}
		url, rest, err := uaBlob(rest)
		if err != nil {
			return nil, err
		}
		if len(rest) != 0 {
			return nil, fmt.Errorf("opcua: RHE trailing bytes")
		}
		out["Server URI"] = string(uri)
		out["Endpoint URL"] = string(url)
		return out, nil
	}
	if kind != "OPN" && kind != "MSG" && kind != "CLO" {
		return nil, protocolError(ErrUnsupportedFeature, "OPC UA message type")
	}
	if chunk != 'F' && chunk != 'C' && chunk != 'A' {
		return nil, fmt.Errorf("opcua: chunk type")
	}
	if len(b) < 4 {
		return nil, fmt.Errorf("opcua: channel")
	}
	channel := binary.LittleEndian.Uint32(b)
	b = b[4:]
	out["Secure Channel ID"] = channel
	peer := s.peers[1-dir]
	if peer.seen && uint32(len(w)) > peer.receive {
		return nil, protocolError(ErrResourceExceeded, "OPC UA negotiated receive buffer")
	}
	token := uint32(0)
	plain := s.plain[channel]
	if kind == "OPN" {
		if chunk != 'F' {
			return nil, protocolError(ErrUnsupportedFeature, "OPC UA chunked OPN")
		}
		policy, rest, err := uaBlob(b)
		if err != nil {
			return nil, err
		}
		cert, rest, err := uaBlob(rest)
		if err != nil {
			return nil, err
		}
		thumb, rest, err := uaBlob(rest)
		if err != nil {
			return nil, err
		}
		out["Security Policy URI"] = string(policy)
		out["Sender Certificate Bytes"] = len(cert)
		out["Receiver Thumbprint Bytes"] = len(thumb)
		plain = bytes.Equal(policy, []byte("http://opcfoundation.org/UA/SecurityPolicy#None"))
		if plain && (len(cert) != 0 || len(thumb) != 0) {
			return nil, fmt.Errorf("opcua: certificates with None policy")
		}
		if _, ok := s.plain[channel]; !ok && len(s.plain) >= max {
			return nil, protocolError(ErrResourceExceeded, "OPC UA channels")
		}
		s.plain[channel] = plain
		b = rest
	} else {
		if len(b) < 4 {
			return nil, fmt.Errorf("opcua: token")
		}
		token = binary.LittleEndian.Uint32(b)
		out["Token ID"] = token
		b = b[4:]
	}
	if !plain {
		out["Semantic Status"] = "security-context-required"
		out["Protected Bytes"] = len(b)
		out["Content Decoded"] = false
		return out, nil
	}
	if len(b) < 8 {
		return nil, fmt.Errorf("opcua: sequence header")
	}
	sequence, request := binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[4:])
	b = b[8:]
	out["Sequence Number"] = sequence
	out["Request ID"] = request
	previous, seen := s.seq[dir][channel]
	if seen && sequence != previous+1 && !(previous > 4294966271 && sequence < 1024) {
		out["Sequence Gap"] = true
	}
	if !seen && len(s.seq[dir]) >= max {
		return nil, protocolError(ErrResourceExceeded, "OPC UA sequence channels")
	}
	s.seq[dir][channel] = sequence
	key := uaKey{dir, channel, request}
	if chunk == 'A' {
		delete(s.chunks, key)
		if len(b) < 8 {
			return nil, fmt.Errorf("opcua: abort")
		}
		out["Abort Status"] = binary.LittleEndian.Uint32(b)
		reason, rest, err := uaBlob(b[4:])
		if err != nil {
			return nil, err
		}
		if len(rest) != 0 {
			return nil, fmt.Errorf("opcua: abort trailing bytes")
		}
		out["Reason"] = string(reason)
		return out, nil
	}
	assembled := s.chunks[key]
	total, count := len(b), 1
	if assembled != nil {
		if assembled.kind != kind || assembled.token != token {
			return nil, protocolError(ErrDesynchronized, "OPC UA chunk identity")
		}
		total += len(assembled.data)
		count += assembled.count
	}
	if total > maxBytes || count > max || (peer.message != 0 && uint64(total) > uint64(peer.message)) || (peer.chunks != 0 && uint64(count) > uint64(peer.chunks)) {
		return nil, protocolError(ErrResourceExceeded, "OPC UA assembled message/chunks")
	}
	if chunk == 'C' {
		if assembled == nil {
			if len(s.chunks) >= max {
				return nil, protocolError(ErrResourceExceeded, "OPC UA chunk streams")
			}
			assembled = &uaChunk{kind: kind, token: token}
			s.chunks[key] = assembled
		}
		assembled.data = append(assembled.data, b...)
		assembled.count = count
		out["Buffered Bytes"] = total
		return out, nil
	}
	if assembled != nil {
		b = append(assembled.data, b...)
		delete(s.chunks, key)
		out["Reassembled"] = true
	}
	service, _, err := uaService(b)
	if err != nil {
		return nil, err
	}
	out["Service Type ID"] = service
	out["Service Payload Bytes"] = len(b)
	out["Semantic Status"] = "service-envelope-only"
	out["Service Name"] = map[uint32]string{446: "OpenSecureChannelRequest", 449: "OpenSecureChannelResponse", 452: "CloseSecureChannelRequest", 461: "CreateSessionRequest", 464: "CreateSessionResponse", 467: "ActivateSessionRequest", 470: "ActivateSessionResponse", 473: "CloseSessionRequest", 476: "CloseSessionResponse", 527: "BrowseRequest", 530: "BrowseResponse", 631: "ReadRequest", 634: "ReadResponse", 673: "WriteRequest", 676: "WriteResponse"}[service]
	// Request ID is scoped by secure channel and direction. Service bodies remain opaque.
	responseTo := map[uint32]uint32{449: 446, 464: 461, 470: 467, 476: 473, 530: 527, 634: 631, 676: 673}
	if expected, ok := responseTo[service]; ok {
		key.dir = 1 - dir
		req, match := s.pending[key]
		if !match && kind == "OPN" {
			key.channel = 0
			req, match = s.pending[key]
		}
		out["Matched"] = match && req == expected
		if match {
			if req != expected {
				return nil, protocolError(ErrDesynchronized, "OPC UA response service")
			}
			delete(s.pending, key)
		}
	} else if _, ok := map[uint32]bool{446: true, 461: true, 467: true, 473: true, 527: true, 631: true, 673: true}[service]; ok {
		if _, exists := s.pending[key]; exists {
			return nil, protocolError(ErrDesynchronized, "OPC UA request ID reused")
		}
		if len(s.pending) >= max {
			return nil, protocolError(ErrResourceExceeded, "OPC UA pending requests")
		}
		s.pending[key] = service
	}
	if kind == "CLO" {
		delete(s.plain, channel)
		delete(s.tokens, channel)
		for k := range s.pending {
			if k.channel == channel {
				delete(s.pending, k)
			}
		}
		for k := range s.chunks {
			if k.channel == channel {
				delete(s.chunks, k)
			}
		}
		delete(s.seq[0], channel)
		delete(s.seq[1], channel)
	}
	return out, nil
}
