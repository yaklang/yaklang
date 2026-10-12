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
	kind    string
	token   uint32
	data    []byte
	count   int
	refs    []PacketReference
	parents []uint64
}
type uaPeer struct {
	seen, hello                    bool
	version                        uint32
	receive, send, message, chunks uint32
}
type uaSelectedPending struct {
	service, handle uint32
	count           int
	maxRefs         uint32
	release         bool
	argCounts       []int
}
type binOPCUA struct {
	currentRefs, completedRefs      []PacketReference
	currentID                       uint64
	completedParents                []uint64
	read, browse, write, next, call bool
	arrays, structured              bool
	maxDepth                        int
	appPending                      map[uaKey]uaSelectedPending
	appReserve                      func(int64) error
	transportSeen                   bool
	reverseSeen                     bool
	reverseDir                      int
	reverseEndpoint                 string
	peers                           [2]uaPeer
	plain                           map[uint32]bool
	tokens                          map[uint32]uint32
	seq                             [2]map[uint32]uint32
	chunks                          map[uaKey]*uaChunk
	pending                         map[uaKey]uint32
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
	memory := int64(512 + len(s.reverseEndpoint) + (len(s.plain)+len(s.tokens)+len(s.pending)+len(s.appPending)+len(s.seq[0])+len(s.seq[1]))*128 + 2*len(w))
	for _, p := range s.appPending {
		memory += int64(cap(p.argCounts)) * 8
	}
	for _, c := range s.chunks {
		memory += 2*int64(cap(c.data)) + int64(cap(c.refs))*80 + int64(cap(c.parents))*8 + 128
	}
	s.maxDepth = f.a.budget.MaxRecursionDepth
	s.appReserve = func(extra int64) error { return f.reserveSession(memory + 3*extra) }
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
func (s *binOPCUA) consume(dir int, w []byte, max, maxBytes int) (result map[string]any, err error) {
	if len(w) < 8 || int(binary.LittleEndian.Uint32(w[4:])) != len(w) {
		return nil, fmt.Errorf("opcua: header")
	}
	s.completedRefs, s.completedParents = nil, nil
	kind := string(w[:3])
	defer func() {
		if err == nil && kind != "RHE" {
			s.transportSeen = true
		}
	}()
	chunk := w[3]
	b := w[8:]
	out := map[string]any{"Message Type": kind, "Chunk Type": string(chunk), "Message Size": len(w)}
	if s.plain == nil {
		s.plain = map[uint32]bool{}
		s.tokens = map[uint32]uint32{}
		s.chunks = map[uaKey]*uaChunk{}
		s.pending = map[uaKey]uint32{}
		s.appPending = map[uaKey]uaSelectedPending{}
		s.seq = [2]map[uint32]uint32{{}, {}}
	}
	if kind == "HEL" || kind == "ACK" {
		if chunk != 'F' || len(b) < 20 {
			return nil, fmt.Errorf("opcua: HEL/ACK")
		}
		// Part 6 7.1.3 permits one HEL and one ACK per transport. Check
		// observed messages only: ACK can be the first packet in a capture.
		for _, peer := range s.peers {
			if peer.seen && peer.hello == (kind == "HEL") {
				return nil, fmt.Errorf("opcua: duplicate %s", kind)
			}
		}
		if s.peers[dir].seen {
			return nil, fmt.Errorf("opcua: HEL/ACK sender role changed")
		}
		// RHE identifies the server independently of TCP initiation or ports.
		// Enforce roles only after observing that message in this connection.
		if s.reverseSeen && (kind == "HEL" && dir == s.reverseDir || kind == "ACK" && dir != s.reverseDir) {
			return nil, fmt.Errorf("opcua: reverse handshake direction")
		}
		p := uaPeer{seen: true, hello: kind == "HEL", version: binary.LittleEndian.Uint32(b), receive: binary.LittleEndian.Uint32(b[4:]), send: binary.LittleEndian.Uint32(b[8:]), message: binary.LittleEndian.Uint32(b[12:]), chunks: binary.LittleEndian.Uint32(b[16:])}
		// Part 6 permits 1024-byte buffers for ECC. HEL/ACK precede OPN,
		// so the security policy is not yet known at this transport boundary.
		if p.receive < 1024 || p.send < 1024 {
			return nil, fmt.Errorf("opcua: buffers below 1024")
		}
		if peer := s.peers[1-dir]; kind == "ACK" && peer.seen && peer.hello {
			if p.version > peer.version {
				return nil, fmt.Errorf("opcua: ACK version exceeds HEL version")
			}
			// ACK receive/send correspond to the HEL send/receive limits.
			// A large HEL buffer still requires an ACK minimum of 8192;
			// allowing ECC must not relax a negotiation we actually observed.
			if p.receive > peer.send || p.send > peer.receive ||
				(peer.send >= 8192 && p.receive < 8192) || (peer.receive >= 8192 && p.send < 8192) {
				return nil, fmt.Errorf("opcua: ACK buffers outside HEL negotiation")
			}
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
			// Part 6 7.1.2.6 requires the RHE EndpointUrl to be returned in HEL.
			if s.reverseSeen && string(endpoint) != s.reverseEndpoint {
				return nil, fmt.Errorf("opcua: HEL endpoint differs from ReverseHello")
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
		if s.reverseSeen || s.transportSeen {
			return nil, fmt.Errorf("opcua: ReverseHello after transport establishment")
		}
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
		// Part 6 7.1.2.6 limits each UTF-8 encoded string value, independently
		// of the frame's resource budget. Preserve null/empty string handling.
		if len(uri) >= 4096 || len(url) >= 4096 {
			return nil, fmt.Errorf("opcua: ReverseHello string length")
		}
		out["Server URI"] = string(uri)
		out["Endpoint URL"] = string(url)
		s.reverseSeen, s.reverseDir, s.reverseEndpoint = true, dir, string(url)
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
	wrapped := previous > 4294966271 && sequence < 1024
	// Part 6 6.7.2.4 forbids sequence reuse and resetting on token renewal.
	// Captures can omit forward chunks; retain their gap observation. A replay
	// or backwards sequence must not satisfy a newly reused application ID.
	// Keep the default envelope observer's existing permissive semantics.
	if s.selected() && seen && sequence <= previous && !wrapped {
		return nil, protocolError(ErrDesynchronized, "OPC UA repeated or backwards sequence")
	}
	if seen && sequence != previous+1 && !wrapped {
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
		if s.selected() {
			for _, k := range []uaKey{key, {1 - dir, channel, request}} {
				if _, ok := s.appPending[k]; ok {
					delete(s.appPending, k)
					delete(s.pending, k)
				}
			}
		}
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
	if s.selected() && (chunk == 'C' || assembled != nil) {
		refs := len(s.currentRefs)
		parents := 1
		if assembled != nil {
			refs += len(assembled.refs)
			parents += len(assembled.parents)
		}
		if refs > max || parents > max {
			return nil, protocolError(ErrResourceExceeded, "OPC UA selected chunk evidence")
		}
		if s.appReserve != nil {
			if err := s.appReserve(int64(2*total + refs*160 + parents*16 + 256)); err != nil {
				return nil, err
			}
		}
	}
	if chunk == 'C' {
		if assembled == nil {
			if len(s.chunks) >= max {
				return nil, protocolError(ErrResourceExceeded, "OPC UA chunk streams")
			}
			assembled = &uaChunk{kind: kind, token: token}
			s.chunks[key] = assembled
		}
		if s.selected() {
			assembled.refs = append(assembled.refs, s.currentRefs...)
			if s.currentID != 0 {
				assembled.parents = append(assembled.parents, s.currentID)
			}
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
		if s.selected() {
			s.completedRefs = uaUniqueRefs(assembled.refs, s.currentRefs)
			s.completedParents = append([]uint64(nil), assembled.parents...)
		}
	}
	service, serviceBytes, err := uaService(b)
	if err != nil {
		return nil, err
	}
	out["Service Type ID"] = service
	out["Service Payload Bytes"] = len(b)
	out["Semantic Status"] = "service-envelope-only"
	out["Service Name"] = map[uint32]string{446: "OpenSecureChannelRequest", 449: "OpenSecureChannelResponse", 452: "CloseSecureChannelRequest", 461: "CreateSessionRequest", 464: "CreateSessionResponse", 467: "ActivateSessionRequest", 470: "ActivateSessionResponse", 473: "CloseSessionRequest", 476: "CloseSessionResponse", 527: "BrowseRequest", 530: "BrowseResponse", 533: "BrowseNextRequest", 536: "BrowseNextResponse", 631: "ReadRequest", 634: "ReadResponse", 673: "WriteRequest", 676: "WriteResponse", 712: "CallRequest", 715: "CallResponse"}[service]
	var readBody map[string]any
	if s.read && (service == 631 || service == 634) || s.browse && (service == 527 || service == 530) || s.write && (service == 673 || service == 676) || s.next && (service == 533 || service == 536) || s.call && (service == 712 || service == 715) {
		// Any selected-body refusal invalidates this observed pending exchange.
		// A later response cannot consume the rejected message's association.
		defer func() {
			if err != nil {
				k := key
				if service == 634 || service == 530 || service == 676 || service == 536 || service == 715 {
					k.dir = 1 - dir
				}
				delete(s.pending, k)
				delete(s.appPending, k)
			}
		}()
		label := "Read"
		if service == 527 || service == 530 {
			label = "Browse"
			readBody, err = decodeOPCUABrowse(b[serviceBytes:], service == 530, max, s.appReserve, s.valueOptions())
		} else if service == 673 || service == 676 {
			label = "Write"
			readBody, err = decodeOPCUAWrite(b[serviceBytes:], service == 676, max, s.appReserve, s.valueOptions())
		} else if service == 533 || service == 536 {
			label = "BrowseNext"
			readBody, err = decodeOPCUABrowseNext(b[serviceBytes:], service == 536, max, s.appReserve, s.valueOptions())
		} else if service == 712 || service == 715 {
			label = "Call"
			readBody, err = decodeOPCUACall(b[serviceBytes:], service == 715, max, s.appReserve, s.valueOptions())
		} else {
			readBody, err = decodeOPCUARead(b[serviceBytes:], service == 634, max, s.appReserve, s.valueOptions())
		}
		if err != nil {
			return nil, err
		}
		out[label] = readBody
		if label == "Read" {
			out["Semantic Status"] = "read-wire-observation"
		} else if label == "Browse" {
			out["Semantic Status"] = "browse-wire-observation"
		} else if label == "Write" {
			out["Semantic Status"] = "write-wire-observation"
		} else if label == "BrowseNext" {
			out["Semantic Status"] = "browsenext-wire-observation"
		} else {
			out["Semantic Status"] = "call-wire-observation"
		}
		if service == 634 || service == 530 || service == 676 || service == 536 || service == 715 {
			results, _ := readBody["Results"].([]any)
			diagnostics, _ := readBody["Diagnostic Infos"].([]any)
			// Diagnostic entries correspond to result entries even when the
			// capture did not include the request. An absent request is not
			// permission to accept an internally inconsistent response.
			if readBody["Header"].(map[string]any)["Service Result"].(uint32)&0x80000000 == 0 && len(diagnostics) != 0 && len(diagnostics) != len(results) {
				return nil, protocolError(ErrDesynchronized, "OPC UA Read diagnostic count")
			}
			requestKey := uaKey{1 - dir, channel, request}
			if pending, ok := s.appPending[requestKey]; ok {
				header := readBody["Header"].(map[string]any)
				if header["Request Handle"] != pending.handle {
					return nil, protocolError(ErrDesynchronized, "OPC UA Read response handle")
				}
				if header["Service Result"].(uint32)&0x80000000 == 0 && len(results) != pending.count {
					return nil, protocolError(ErrDesynchronized, "OPC UA Read response result count")
				}
				if service == 715 && pending.service == 712 {
					for i, result := range results {
						if i >= len(pending.argCounts) {
							break
						}
						m := result.(map[string]any)
						args, _ := m["Input Argument Results"].([]any)
						diags, _ := m["Input Argument Diagnostic Infos"].([]any)
						if len(args) != 0 && len(args) != pending.argCounts[i] || len(diags) != 0 && len(diags) != pending.argCounts[i] {
							return nil, protocolError(ErrDesynchronized, "OPC UA Call input argument count")
						}
					}
				}
				if service == 536 && pending.service == 533 && pending.release && (len(results) != 0 || len(diagnostics) != 0) {
					return nil, protocolError(ErrDesynchronized, "OPC UA BrowseNext release response must have empty results/diagnostics")
				}
				if service == 530 && pending.service == 527 && pending.maxRefs != 0 {
					for _, result := range results {
						refs, _ := result.(map[string]any)["References"].([]any)
						if uint64(len(refs)) > uint64(pending.maxRefs) {
							return nil, protocolError(ErrDesynchronized, "OPC UA Browse exceeds requested references per node")
						}
					}
				}
			}
		}
	}
	// Request ID is scoped by secure channel and direction. Other service bodies remain opaque.
	responseTo := map[uint32]uint32{449: 446, 464: 461, 470: 467, 476: 473, 530: 527, 536: 533, 634: 631, 676: 673, 715: 712}
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
			delete(s.appPending, key)
		}
	} else if _, ok := map[uint32]bool{446: true, 461: true, 467: true, 473: true, 527: true, 533: true, 631: true, 673: true, 712: true}[service]; ok {
		if _, exists := s.pending[key]; exists {
			return nil, protocolError(ErrDesynchronized, "OPC UA request ID reused")
		}
		if len(s.pending) >= max {
			return nil, protocolError(ErrResourceExceeded, "OPC UA pending requests")
		}
		s.pending[key] = service
		if readBody != nil {
			nodeKey := "Nodes To Read"
			if service == 527 {
				nodeKey = "Nodes To Browse"
			} else if service == 673 {
				nodeKey = "Nodes To Write"
			} else if service == 533 {
				nodeKey = "Continuation Points"
			} else if service == 712 {
				nodeKey = "Methods To Call"
			}
			nodes, _ := readBody[nodeKey].([]any)
			p := uaSelectedPending{service: service, handle: readBody["Header"].(map[string]any)["Request Handle"].(uint32), count: len(nodes)}
			if service == 527 {
				p.maxRefs = readBody["Requested Max References Per Node"].(uint32)
			}
			if service == 533 {
				p.release = readBody["Release Continuation Points"].(bool)
				if p.release {
					p.count = 0
				}
			}
			if service == 712 {
				p.argCounts = make([]int, len(nodes))
				for i, node := range nodes {
					args, _ := node.(map[string]any)["Input Arguments"].([]any)
					p.argCounts[i] = len(args)
				}
			}
			s.appPending[key] = p
		}
	}
	if kind == "CLO" {
		delete(s.plain, channel)
		delete(s.tokens, channel)
		for k := range s.pending {
			if k.channel == channel {
				delete(s.pending, k)
				delete(s.appPending, k)
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

// consumeEvent carries contributor identities across application chunks. Raw and
// its SHA remain the final captured envelope; parent events expose prior chunks.
func (s *binOPCUA) consumeEvent(dir int, e *ProtocolEvent, max, maxBytes int) (map[string]any, error) {
	s.currentRefs = e.SourceBytes.PacketRefs
	s.currentID = e.ID
	out, err := s.consume(dir, e.Raw, max, maxBytes)
	s.currentRefs = nil
	s.currentID = 0
	if err != nil && s.selected() {
		for k := range s.appPending {
			delete(s.pending, k)
		}
		clear(s.appPending)
		clear(s.chunks)
	}
	if err == nil && len(s.completedParents) > 0 {
		e.SourceBytes.PacketRefs = append([]PacketReference(nil), s.completedRefs...)
		e.SourceBytes.ParentPDUs = append([]uint64(nil), s.completedParents...)
	}
	s.completedRefs = nil
	s.completedParents = nil
	return out, err
}

func uaUniqueRefs(a, b []PacketReference) []PacketReference {
	out := make([]PacketReference, 0, len(a)+len(b))
	seen := make(map[PacketReference]bool, len(a)+len(b))
	for _, list := range [][]PacketReference{a, b} {
		for _, ref := range list {
			if !seen[ref] {
				out = append(out, ref)
				seen[ref] = true
			}
		}
	}
	return out
}

func (s *binOPCUA) selected() bool { return s.read || s.browse || s.write || s.next || s.call }
func (s *binOPCUA) observationProfile() string {
	if s.structured {
		switch {
		case s.read && !s.browse && !s.write && !s.next && !s.call:
			return "opcua-read-values"
		case s.browse && !s.read && !s.write && !s.next && !s.call:
			return "opcua-browse-values"
		case s.write && !s.read && !s.browse && !s.next && !s.call:
			return "opcua-write-values"
		case s.next && !s.read && !s.browse && !s.write && !s.call:
			return "opcua-browsenext-values"
		case s.call && !s.read && !s.browse && !s.write && !s.next:
			return "opcua-call-values"
		}
	}

	if s.read && !s.browse && !s.write && !s.next && !s.call {
		if s.arrays {
			return "opcua-read-variants"
		}
		return "opcua-read-scalar"
	}
	if s.browse && !s.read && !s.write && !s.next && !s.call {
		return "opcua-browse"
	}
	if s.write && !s.read && !s.browse && !s.next && !s.call {
		if s.arrays {
			return "opcua-write-variants"
		}
		return "opcua-write"
	}
	if s.next && !s.read && !s.browse && !s.write && !s.call {
		return "opcua-browsenext"
	}
	if s.call && !s.read && !s.browse && !s.write && !s.next {
		if s.arrays {
			return "opcua-call-variants"
		}
		return "opcua-call"
	}
	return "opcua-application"
}
