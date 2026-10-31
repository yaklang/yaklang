package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// binSMB2 is the M0 session state for SMB2/SMB3 over Direct TCP. Dialect
// comes from NEGOTIATE. Encrypted transform headers are a boundary, not
// decryption. Ports including 445 are never consulted.
type binSMB2 struct {
	flow       *binFlow
	client     int
	dialect    uint16
	pending    map[smb2PendingKey]smb2Pending
	async      map[smb2AsyncKey]smb2PendingKey
	files      map[smb2FileKey]string
	smb1       *smb1NegotiateState
	smb1Setup  *smb1SetupState
	smb1Legacy *smb1LegacyState
	smb1Modern bool
}

type smb2PendingKey struct {
	sessionID uint64
	treeID    uint32
	messageID uint64
}

type smb2AsyncKey struct {
	sessionID uint64
	asyncID   uint64
}

type smb2FileKey struct {
	sessionID uint64
	treeID    uint32
	fileID    string
}

type smb2Pending struct {
	key          smb2PendingKey
	name         string
	path         string
	fileID       string
	related      bool
	direction    int
	hasDirection bool
	asyncID      uint64
	async        bool
	chain        *smb2RequestChain
	ordinal      int
	queryInfo    smb2QueryInfoSelector
}

type smb2QueryInfoSelector struct {
	infoType, fileInfoClass uint8
	outputLength            uint32
	additionalInformation   uint32
	flags                   uint32
}

// A chain belongs to one observed request PDU and is retained only by that
// PDU's outstanding requests. Pointer identity cannot collide with another
// PDU, flow, TCP generation, interface or VLAN. Results are bounded by size.
type smb2RequestChain struct {
	size    int
	results map[int]smb2CompoundResult
}

type smb2CompoundResult struct {
	fileID, path string
	messageID    uint64
	fileSourceID uint64
	status       uint32
}

type smb2CompoundLink struct {
	chain   *smb2RequestChain
	ordinal int
}

const (
	smb2Magic       = 0x424d53fe
	smb3Transform   = 0x424d53fd
	smb2HeaderSize  = 64
	smb2FlagResp    = 0x00000001
	smb2FlagAsync   = 0x00000002
	smb2FlagRelated = 0x00000004
)

func probeSMB2(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 8 {
		return probeNeed("smb2", "2/3", len(w), 8)
	}
	n := smb2DirectLength(w)
	if n < 52 || n > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	magic := binary.LittleEndian.Uint32(w[4:8])
	switch magic {
	case smb2Magic:
		if n < smb2HeaderSize {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) < 4+smb2HeaderSize {
			return probeNeed("smb2", "2/3", len(w), 4+smb2HeaderSize)
		}
		if binary.LittleEndian.Uint16(w[8:10]) != smb2HeaderSize {
			return ProbeResult{Verdict: ProbeReject}
		}
		next := uint64(binary.LittleEndian.Uint32(w[24:28]))
		if next != 0 && (next < smb2HeaderSize || next%8 != 0 || next+64 > uint64(n)) {
			return ProbeResult{Verdict: ProbeReject}
		}
		_ = limit
		return probeAccept("smb2", "2/3", 94)
	case smb3Transform:
		if n <= 52 {
			return ProbeResult{Verdict: ProbeReject}
		}
		if len(w) < 56 {
			return probeNeed("smb2", "3-transform", len(w), 56)
		}
		if binary.LittleEndian.Uint16(w[44:46]) != 0 || binary.LittleEndian.Uint16(w[46:48]) != 1 || binary.LittleEndian.Uint32(w[40:44]) != uint32(n-52) {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeAccept("smb2", "3-transform", 90)
	default:
		return ProbeResult{Verdict: ProbeReject}
	}
}

func smb2DirectLength(w []byte) int {
	if len(w) < 4 {
		return 0
	}
	return int(w[1])<<16 | int(w[2])<<8 | int(w[3])
}

func (f *binFlow) frameSMB2(w []byte) (int, *binSpec, error) {
	if f.nbss && len(w) > 0 && w[0] != 0 {
		return f.frameNBSS(0, w)
	}
	s := f.smb2
	if s == nil {
		return 0, nil, sessionContext("SMB2 session was not observed")
	}
	s.flow = f
	if err := s.reserveState(0); err != nil {
		if f.protocol == "smb" {
			return 0, nil, smb1ResourceRefusal(err)
		}
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	if w[0] != 0 {
		return 0, nil, fmt.Errorf("smb2: Direct TCP zero byte required")
	}
	n := smb2DirectLength(w)
	if n < 4 {
		return 0, nil, fmt.Errorf("smb: Direct TCP length smaller than signature")
	}
	total := 4 + n
	if total > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if total > len(w) {
		return total, nil, nil
	}
	magic := binary.LittleEndian.Uint32(w[4:8])
	switch magic {
	case 0x424d53ff:
		if n >= 35 && (w[8] == 0x75 || w[8] == 0x06) {
			if !boundedSMB1FileEnvelope(w[:total]) {
				return 0, nil, fmt.Errorf("CIFS file command: WordCount/ByteCount exceed exact DirectTCP boundary")
			}
			entry, _, _, _ := smb1FileEntry(w[8], w[13]&0x80 != 0)
			f.protocol = "smb"
			return total, f.spec("cifs", entry), nil
		}
		if probeSMB1SessionSetupExtended(w[:total], 65598).Verdict == ProbeAccept {
			f.protocol = "smb"
			entry := "CIFSDirectTCPSessionSetupExtendedRequest"
			if w[13]&0x80 != 0 {
				entry = "CIFSDirectTCPSessionSetupExtendedResponse"
			}
			return total, f.spec("cifs", entry), nil
		}
		if n >= 32 && w[8] == 0x73 {
			return 0, nil, smb1SetupUnsupported(w[:total])
		}
		if probeSMB1Negotiate(w[:total], f.a.budget.MaxFrameBytes).Verdict == ProbeAccept {
			f.protocol = "smb"
			return total, f.spec("cifs", "CIFSDirectTCP"), nil
		}
		if probeSMB1NegotiateExtendedResponse(w[:total], 65608).Verdict == ProbeAccept {
			f.protocol = "smb"
			return total, f.spec("cifs", "CIFSDirectTCPNegotiateExtendedResponse"), nil
		}
		if n >= 35 && w[8] == 0x72 {
			fixed := 39 + 2*int(w[36])
			if total < fixed || int(binary.LittleEndian.Uint16(w[fixed-2:fixed])) != total-fixed {
				return 0, nil, fmt.Errorf("smb: negotiate WordCount/ByteCount exceed the complete record boundary")
			}
		}
		// Availability of a bounded native profile is separate from wire
		// validity. Other complete SMB1 commands/response forms stay explicit
		// unsupported observations instead of being called malformed.
		if n >= 32 && w[8] != 0x72 {
			return 0, nil, protocolError(ErrUnsupportedFeature, "SMB1 command 0x%02x has no selected native body profile", w[8])
		}
		if n >= 35 && w[13]&0x80 != 0 && (w[36] != 17 || binary.LittleEndian.Uint32(w[9:13]) != 0 || binary.LittleEndian.Uint16(w[14:16])&0x800 == 0 || n >= 56 && binary.LittleEndian.Uint32(w[56:60])&0x80000000 == 0) {
			return 0, nil, protocolError(ErrUnsupportedFeature, "SMB1 negotiate response form is outside the WC17 extended-security profile")
		}
		return 0, nil, fmt.Errorf("smb: unsupported or invalid SMB1 negotiate profile")
	case smb3Transform:
		if n < 52 {
			return 0, nil, fmt.Errorf("smb2: truncated transform header")
		}
		orig := binary.LittleEndian.Uint32(w[4+36 : 4+40])
		ciphertextLen := n - 52
		if orig == 0 || ciphertextLen == 0 {
			return 0, nil, fmt.Errorf("smb2: empty transform ciphertext")
		}
		if uint64(ciphertextLen) < uint64(orig) {
			return 0, nil, fmt.Errorf("smb2: truncated transform ciphertext")
		}
		if uint64(ciphertextLen) > uint64(orig) {
			return 0, nil, fmt.Errorf("smb2: transform ciphertext length disagrees with original size")
		}
		f.protocol = "smb2"
		return total, f.spec("smb3", "SMB3Transform"), nil
	case smb2Magic:
		if n < smb2HeaderSize {
			return 0, nil, fmt.Errorf("smb2: Direct TCP length smaller than header")
		}
		if binary.LittleEndian.Uint16(w[8:10]) != 64 {
			return 0, nil, fmt.Errorf("smb2: header StructureSize must be 64")
		}
		f.protocol = "smb2"
		return total, f.spec("smb2", "SMB2"), nil
	default:
		return 0, nil, fmt.Errorf("smb2: ProtocolId is not SMB2 or SMB3 transform")
	}
}

func (s *binSMB2) reserveState(additionalPathBytes int64) error {
	if s.flow == nil {
		return nil
	}
	var pathBytes int64
	var chainBytes int64
	seen := make(map[*smb2RequestChain]bool)
	for _, pending := range s.pending {
		pathBytes += int64(len(pending.path))
		if chain := pending.chain; chain != nil && !seen[chain] {
			seen[chain] = true
			chainBytes += int64(chain.size)*128 + 64
			for _, result := range chain.results {
				pathBytes += int64(len(result.path))
			}
		}
	}
	for _, path := range s.files {
		pathBytes += int64(len(path))
	}
	// Includes the private chain identity and the retained QUERY_INFO selector.
	target := int64(256) + int64(len(s.pending)+1)*160 + int64(len(s.async)+1)*32 + int64(len(s.files)+1)*64
	target += pathBytes + chainBytes + additionalPathBytes + s.smb1.ownedBytes() + s.smb1Setup.ownedBytes() + s.smb1Legacy.ownedBytes()
	return s.flow.reserveSession(target)
}

func (s *binSMB2) consume(raw []byte, max int) (map[string]any, error) {
	return s.consumeFrom(-1, raw, max)
}

func (s *binSMB2) consumeFrom(dir int, raw []byte, max int) (map[string]any, error) {
	if s.pending == nil {
		s.pending = make(map[smb2PendingKey]smb2Pending)
		s.async = make(map[smb2AsyncKey]smb2PendingKey)
		s.files = make(map[smb2FileKey]string)
	}
	if len(raw) < 8 {
		return nil, fmt.Errorf("smb2: truncated Direct TCP header")
	}
	if raw[0] != 0 {
		return nil, fmt.Errorf("smb2: Direct TCP zero byte required")
	}
	n := smb2DirectLength(raw)
	if 4+n != len(raw) {
		return nil, fmt.Errorf("smb2: Direct TCP length disagrees with framed message")
	}
	if max <= 0 {
		max = 4096
	}
	payload := raw[4:]
	magic := binary.LittleEndian.Uint32(payload[:4])
	if magic == smb3Transform {
		if len(payload) < 52 {
			return nil, fmt.Errorf("smb2: truncated transform header")
		}
		sid := binary.LittleEndian.Uint64(payload[44:52])
		orig := binary.LittleEndian.Uint32(payload[36:40])
		ciphertextLen := len(payload) - 52
		if orig == 0 || ciphertextLen == 0 {
			return nil, fmt.Errorf("smb2: empty transform ciphertext")
		}
		if uint64(ciphertextLen) < uint64(orig) {
			return nil, fmt.Errorf("smb2: truncated transform ciphertext")
		}
		if uint64(ciphertextLen) > uint64(orig) {
			return nil, fmt.Errorf("smb2: transform ciphertext length disagrees with original size")
		}
		return map[string]any{
			"Packet Name":          "transform",
			"Encrypted":            true,
			"Content Visibility":   "opaque",
			"Decryption Performed": false,
			"Session ID":           sid,
			"Original Size":        orig,
			"Context Level":        "opaque",
			"Version Name":         smb2DialectName(s.dialect),
		}, nil
	}
	cmds, err := smb2Walk(payload, max)
	if err != nil {
		return nil, err
	}
	if len(cmds) == 0 {
		return nil, fmt.Errorf("smb2: missing command")
	}
	info := cmds[0]
	info["Context Level"] = "observed"
	names := make([]string, 0, len(cmds))
	links := smb2RequestLinks(cmds)
	for index, c := range cmds {
		name, _ := c["Packet Name"].(string)
		names = append(names, name)
		if s.dialect != 0 {
			// Connection observations are not a field in this command. In
			// particular, never overwrite the current NEGOTIATE's wire value.
			c["Connection Dialect"] = s.dialect
			c["Connection Version Name"] = smb2DialectName(s.dialect)
			if name != "NEGOTIATE" {
				c["Version Name"] = smb2DialectName(s.dialect)
			}
		}
		if err := s.associateCommand(c, max, dir, links[index]); err != nil {
			return nil, err
		}
		if name == "NEGOTIATE" && c["Response"] == true && c["Status"] == uint32(0) {
			if d, ok := c["Dialect"].(uint16); ok && d != 0 {
				// 0x02ff is the multi-protocol wildcard response. It asks for a
				// new SMB2 NEGOTIATE and is not a selected session dialect.
				if d == 0x02ff {
					c["Dialect Negotiated"], c["SMB2 Negotiate Required"] = false, true
					// An SMB1 offer has no SMB2 pending entry. Its response can
					// clear a selection, but a same-direction observation cannot.
					if c["Association Status"] == "matched" || dir < 0 || dir != s.client {
						s.dialect = 0
						c["Dialect Context Updated"] = true
					} else {
						c["Dialect Context Updated"] = false
					}
				} else if c["Association Status"] == "matched" {
					s.dialect = d
					c["Dialect Context Updated"] = true
				} else {
					c["Dialect Context Updated"] = false
				}
			}
		}
	}
	if len(cmds) > 1 {
		// Keep the top-level summary distinct from cmds[0]. Reusing the first
		// command map here creates a cycle through info["Commands"][0], which
		// breaks owned event snapshots and JSON projection.
		info = cloneSession(info)
		info["Compound"] = true
		info["Compound Count"] = len(cmds)
		info["Command Names"] = names
		info["Commands"] = cmds
	}
	return info, nil
}

func (s *binSMB2) associate(c map[string]any, max int) error {
	return s.associateFrom(c, max, -1)
}

func (s *binSMB2) associateFrom(c map[string]any, max, dir int) error {
	return s.associateCommand(c, max, dir, smb2CompoundLink{})
}

func (s *binSMB2) associateCommand(c map[string]any, max, dir int, link smb2CompoundLink) error {
	mid, _ := c["Message ID"].(uint64)
	name, _ := c["Packet Name"].(string)
	sid, tid := smb2EffectiveIDs(c)
	asyncID, _ := c["Async ID"].(uint64)
	resp, _ := c["Response"].(bool)
	key := smb2PendingKey{sessionID: sid, treeID: tid, messageID: mid}
	if !resp {
		if name == "CANCEL" {
			c["Response Expected"] = false
			c["Cancellation Target Message ID"] = mid
			return nil
		}
		if len(s.pending) >= max {
			return protocolError(ErrResourceExceeded, "SMB2 outstanding requests exceed budget")
		}
		if _, exists := s.pending[key]; exists {
			return protocolError(ErrMalformedMessage, "SMB2 MessageId reused within the same session/tree before response")
		}
		pending := smb2Pending{key: key, name: name, direction: dir, hasDirection: dir >= 0, chain: link.chain, ordinal: link.ordinal}
		pending.path, _ = c["File Name"].(string)
		pending.related, _ = c["Related File ID Ignored"].(bool)
		if name == "QUERY_INFO" {
			body := c["Body"].(map[string]any)
			pending.queryInfo = smb2QueryInfoSelector{
				infoType: body["Info Type"].(uint8), fileInfoClass: body["File Info Class"].(uint8),
				outputLength: body["Output Buffer Length"].(uint32), additionalInformation: body["Additional Information"].(uint32), flags: body["Flags"].(uint32),
			}
		}
		clonePath := false
		if fileID, ok := c["FileId"].(string); ok && fileID != "" {
			if pending.related {
				c["File Context Status"] = "related-compound-file-context-is-server-substituted"
			} else {
				pending.fileID = fileID
				if fileName := s.files[smb2FileKey{sessionID: sid, treeID: tid, fileID: fileID}]; fileName != "" {
					pending.path = fileName
					clonePath = true
					c["File Context"] = fileName
					c["File Context Status"] = "observed-open"
				} else {
					c["File Context Status"] = "missing-create"
				}
			}
		}
		additional := int64(len(pending.path))
		if link.chain != nil && link.ordinal == 0 {
			additional += int64(link.chain.size)*128 + 64
		}
		if err := s.reserveState(additional); err != nil {
			return err
		}
		if clonePath {
			pending.path = strings.Clone(pending.path)
		}
		s.pending[key] = pending
		c["Outstanding"] = true
		return nil
	}

	pendingKey, pending, matched := s.findResponse(key, asyncID, name, c)
	if !matched {
		c["Unmatched"] = true
		c["Association Status"] = "missing-or-ambiguous-request"
		c["Context Level"] = "partial"
		return nil
	}
	if dir >= 0 && pending.hasDirection && pending.direction == dir {
		c["Unmatched"] = true
		c["Association Status"] = "direction-mismatch"
		c["Context Level"] = "partial"
		return nil
	}
	c["Matched Request"] = pending.name
	c["Association Status"] = "matched"
	c["Request Session ID"] = pending.key.sessionID
	c["Request Tree ID"] = pending.key.treeID
	if name == "QUERY_INFO" {
		// These observations belong to the matched request, not this response's
		// StructureSize bytes. The owning flow already fixes interface/VLAN and
		// TCP generation; direction and MessageId/session/tree were checked above.
		c["Request Message ID"] = pending.key.messageID
		c["Request Info Type"] = pending.queryInfo.infoType
		c["Request File Info Class"] = pending.queryInfo.fileInfoClass
		c["Request Additional Information"] = pending.queryInfo.additionalInformation
		c["Request Query Flags"] = pending.queryInfo.flags
		c["Request Output Buffer Length"] = pending.queryInfo.outputLength
		if c["Error Response"] != true {
			c["Output Content Status"] = "opaque: selector from matched request; typed content not decoded"
			smb2DecodeMatchedQueryOutput(c, pending.queryInfo)
		}
	}
	if pending.fileID != "" {
		c["Request File ID"] = pending.fileID
		if pending.path != "" {
			c["File Context"] = pending.path
			c["File Context Status"] = "observed-open"
		}
	}
	status, _ := c["Status"].(uint32)
	if status == 0x00000103 { // STATUS_PENDING: preserve until the async final response.
		if gotID, ok := c["Async ID"].(uint64); ok {
			if pending.async && pending.asyncID != gotID {
				return protocolError(ErrMalformedMessage, "SMB2 outstanding request changed its AsyncId")
			}
			asyncKey := smb2AsyncKey{sessionID: sid, asyncID: gotID}
			if oldKey, exists := s.async[asyncKey]; exists && oldKey != pendingKey {
				return protocolError(ErrMalformedMessage, "SMB2 AsyncId collision within a session")
			}
			if err := s.reserveState(0); err != nil {
				return err
			}
			s.async[asyncKey] = pendingKey
			pending.async, pending.asyncID = true, gotID
			s.pending[pendingKey] = pending
		}
		c["Operation Pending"] = true
		return nil
	}
	delete(s.pending, pendingKey)
	if pending.async {
		delete(s.async, smb2AsyncKey{sessionID: sid, asyncID: pending.asyncID})
	}

	var fileSourceID = pending.key.messageID
	if pending.related {
		c["File Context Status"] = "missing-matched-compound-predecessor"
		if pending.chain != nil && pending.ordinal > 0 {
			previous, observed := pending.chain.results[pending.ordinal-1]
			// A failed READ/WRITE still used its known request FileId. A
			// failed CREATE never generated one. Keep identity provenance
			// separate from the predecessor's operation status.
			if observed && previous.fileID != "" {
				pending.fileID, pending.path = previous.fileID, previous.path
				fileSourceID = previous.fileSourceID
				c["Request File ID"] = pending.fileID
				c["Inherited File ID"] = true
				c["File Context Source Message ID"] = fileSourceID
				c["Related Predecessor Message ID"] = previous.messageID
				c["Related Predecessor Status"] = previous.status
				if pending.path != "" {
					c["File Context"] = pending.path
				}
				c["File Context Status"] = "matched-related-request-chain"
			}
		}
	}
	retainResult := false
	if pending.chain != nil {
		for _, remaining := range s.pending {
			if remaining.chain == pending.chain {
				retainResult = true
				break
			}
		}
		if retainResult {
			if len(pending.chain.results) >= pending.chain.size {
				return protocolError(ErrResourceExceeded, "SMB2 compound results exceed observed request chain")
			}
			if err := s.reserveState(int64(len(pending.path))); err != nil {
				return err
			}
		}
	}
	if name == "CREATE" && status == 0 {
		fileID, _ := c["FileId"].(string)
		if fileID != "" {
			// Async responses have AsyncId instead of TreeId. The matched
			// request owns the association domain; never invent a header TID.
			fileKey := smb2FileKey{sessionID: pending.key.sessionID, treeID: pending.key.treeID, fileID: fileID}
			if len(s.files) >= max && s.files[fileKey] == "" {
				return protocolError(ErrResourceExceeded, "SMB2 open file contexts exceed budget")
			}
			pathDelta := int64(len(pending.path)) - int64(len(s.files[fileKey]))
			if s.files[fileKey] == "" {
				pathDelta = int64(len(pending.path))
			}
			if retainResult {
				pathDelta += int64(len(pending.path))
			}
			if err := s.reserveState(pathDelta); err != nil {
				return err
			}
			s.files[fileKey] = pending.path
			c["File Context Status"] = "opened"
			pending.fileID = fileID
		}
	}
	if name == "CLOSE" && status == 0 {
		if pending.fileID != "" {
			delete(s.files, smb2FileKey{sessionID: pending.key.sessionID, treeID: pending.key.treeID, fileID: pending.fileID})
			c["File Context Status"] = "closed"
		} else {
			c["File Context Status"] = "missing-request-file-context"
		}
	}
	if name == "TREE_DISCONNECT" && status == 0 {
		s.removeTree(pending.key.sessionID, pending.key.treeID)
		c["Tree Context Status"] = "disconnected"
	}
	if name == "LOGOFF" && status == 0 {
		s.removeSession(pending.key.sessionID)
		c["Session Context Status"] = "logged-off"
	}
	if retainResult {
		result := smb2CompoundResult{fileID: pending.fileID, path: pending.path, messageID: pending.key.messageID, fileSourceID: fileSourceID, status: status}
		pending.chain.results[pending.ordinal] = result
	}
	return nil
}

func smb2RequestLinks(commands []map[string]any) []smb2CompoundLink {
	links := make([]smb2CompoundLink, len(commands))
	for start := 0; start < len(commands); {
		if commands[start]["Response"] == true {
			start++
			continue
		}
		end := start + 1
		for end < len(commands) && commands[end]["Response"] == false && commands[end]["Related"] == true {
			end++
		}
		if end-start > 1 {
			chain := &smb2RequestChain{size: end - start, results: make(map[int]smb2CompoundResult)}
			for i := start; i < end; i++ {
				links[i] = smb2CompoundLink{chain: chain, ordinal: i - start}
			}
		}
		start = end
	}
	return links
}

func (s *binSMB2) findResponse(key smb2PendingKey, asyncID uint64, name string, c map[string]any) (smb2PendingKey, smb2Pending, bool) {
	if asyncID != 0 {
		status, _ := c["Status"].(uint32)
		if status != 0x00000103 {
			if pendingKey, ok := s.async[smb2AsyncKey{sessionID: key.sessionID, asyncID: asyncID}]; ok {
				pending, exists := s.pending[pendingKey]
				return pendingKey, pending, exists && pending.name == name
			}
			return smb2PendingKey{}, smb2Pending{}, false
		}
		// STATUS_PENDING establishes an AsyncId for this literal MessageId.
		// An existing AsyncId mapping cannot select a different pending request
		// and thereby hide the collision checked by associateCommand.
	}
	if pending, ok := s.pending[key]; ok {
		return key, pending, pending.name == name
	}
	// SESSION_SETUP assigns the SessionId in the response; TREE_CONNECT
	// assigns the TreeId. Only allow their exact command-specific fallback.
	allowSessionSetup := name == "SESSION_SETUP"
	allowTreeConnect := name == "TREE_CONNECT"
	status, _ := c["Status"].(uint32)
	allowSessionError := key.sessionID == 0 && key.treeID == 0 && status>>30 >= 2 && status != 0x00000103
	if name == "QUERY_INFO" && c["Error Response"] != true {
		// BUFFER_OVERFLOW is a partial QUERY_INFO output, not a lost-session
		// ERROR. It must retain the actual session/tree or observed AsyncId.
		allowSessionError = false
	}
	allowLogoffClear := name == "LOGOFF" && key.sessionID == 0 && key.treeID == 0 && asyncID == 0
	async, _ := c["Async ID"].(uint64)
	if async != 0 {
		allowSessionSetup = false
		allowTreeConnect = false
	}
	var foundKey smb2PendingKey
	var found smb2Pending
	count := 0
	for candidateKey, candidate := range s.pending {
		if candidateKey.messageID != key.messageID {
			continue
		}
		if (allowSessionError || allowLogoffClear) && candidate.name == name {
			// A server that no longer has this session can return a zero SID
			// error (e.g. a repeated LOGOFF). Correlate only a unique MID/command
			// on this capture flow, with the opposite direction checked below.
			foundKey, found = candidateKey, candidate
			count++
			continue
		}
		if allowSessionSetup && candidate.name == "SESSION_SETUP" && candidateKey.sessionID == 0 && candidateKey.treeID == 0 {
			foundKey, found = candidateKey, candidate
			count++
			continue
		}
		if allowTreeConnect && candidate.name == "TREE_CONNECT" && candidateKey.sessionID == key.sessionID && candidateKey.treeID == 0 {
			foundKey, found = candidateKey, candidate
			count++
		}
		status, _ := c["Status"].(uint32)
		if async != 0 && status == 0x00000103 && candidateKey.sessionID == key.sessionID && candidate.name == name {
			foundKey, found = candidateKey, candidate
			count++
		}
	}
	if count != 1 {
		return smb2PendingKey{}, smb2Pending{}, false
	}
	return foundKey, found, true
}

func (s *binSMB2) removeTree(sessionID uint64, treeID uint32) {
	for key := range s.files {
		if key.sessionID == sessionID && key.treeID == treeID {
			delete(s.files, key)
		}
	}
	for key, pending := range s.pending {
		if key.sessionID == sessionID && key.treeID == treeID {
			delete(s.pending, key)
			if pending.async {
				delete(s.async, smb2AsyncKey{sessionID: sessionID, asyncID: pending.asyncID})
			}
		}
	}
}

func (s *binSMB2) removeSession(sessionID uint64) {
	for key := range s.files {
		if key.sessionID == sessionID {
			delete(s.files, key)
		}
	}
	for key, pending := range s.pending {
		if key.sessionID == sessionID {
			delete(s.pending, key)
			if pending.async {
				delete(s.async, smb2AsyncKey{sessionID: sessionID, asyncID: pending.asyncID})
			}
		}
	}
}

func smb2Walk(payload []byte, max int) ([]map[string]any, error) {
	var cmds []map[string]any
	at := 0
	var previous map[string]any
	for {
		if at+smb2HeaderSize > len(payload) {
			return nil, fmt.Errorf("smb2: truncated header")
		}
		h := payload[at:]
		if binary.LittleEndian.Uint32(h[:4]) != smb2Magic {
			return nil, fmt.Errorf("smb2: ProtocolId must be \\xfeSMB")
		}
		if binary.LittleEndian.Uint16(h[4:6]) != 64 {
			return nil, fmt.Errorf("smb2: header StructureSize must be 64")
		}
		if binary.LittleEndian.Uint32(h[16:20])&smb2FlagAsync != 0 && binary.LittleEndian.Uint64(h[32:40]) == 0 {
			return nil, fmt.Errorf("smb2: ASYNC header requires a nonzero AsyncId")
		}
		next := int(binary.LittleEndian.Uint32(h[20:24]))
		end := len(payload)
		if next != 0 {
			if next < smb2HeaderSize || next%8 != 0 || at+next > len(payload) {
				return nil, fmt.Errorf("smb2: invalid NextCommand")
			}
			end = at + next
		}
		cmd, err := smb2ParseCommand(h[:end-at], max)
		if err != nil {
			// No association has run yet. A caller may isolate this body-only
			// failure after independently validating every enclosing header.
			return nil, &smb2BodyDecodeError{command: len(cmds), cause: err}
		}
		if len(cmds) > 0 && cmd["Response"] != cmds[0]["Response"] {
			return nil, protocolError(ErrMalformedMessage, "SMB2 compound mixes requests and responses")
		}
		if cmd["Response"] == false && len(cmds) > 1 && (cmd["Related"] == true) != (cmds[1]["Related"] == true) {
			return nil, protocolError(ErrMalformedMessage, "SMB2 compound mixes related and unrelated requests")
		}
		smb2OffsetRanges(cmd, 4+at)
		if related, _ := cmd["Related"].(bool); related {
			if previous == nil {
				return nil, protocolError(ErrMalformedMessage, "SMB2 related compound operation has no predecessor")
			}
			// MS-SMB2 3.3.5.2.7.2: related requests use the preceding
			// operation's context even when the ignored literal IDs happen
			// to be concrete. Responses carry their own returned IDs.
			if cmd["Response"] == false {
				effectiveSID, effectiveTID := smb2EffectiveIDs(previous)
				cmd["Effective Session ID"] = effectiveSID
				cmd["Inherited Session Context"] = true
				cmd["Effective Tree ID"] = effectiveTID
				cmd["Inherited Tree Context"] = true
			}
			if fileID, _ := cmd["FileId"].(string); cmd["Response"] == false && fileID != "" {
				cmd["Related File ID Ignored"] = true
				if fileID == "ffffffffffffffffffffffffffffffff" {
					cmd["Related File ID Placeholder"] = true
				}
				cmd["File Context Status"] = "related-compound-file-context-is-server-substituted"
			}
			if previousName, ok := previous["Packet Name"].(string); ok {
				cmd["Related Previous Command"] = previousName
			}
		}
		if len(cmds) >= max {
			return nil, protocolError(ErrResourceExceeded, "SMB2 compound command count exceeds limit")
		}
		cmds = append(cmds, cmd)
		previous = cmd
		if next == 0 {
			return cmds, nil
		}
		at += next
	}
}

func smb2ParseCommand(pdu []byte, max int) (map[string]any, error) {
	if len(pdu) < smb2HeaderSize {
		return nil, fmt.Errorf("smb2: truncated command header")
	}
	info := smb2CommandHeader(pdu)
	cmd := info["Command"].(uint16)
	resp := info["Response"].(bool)
	body := pdu[smb2HeaderSize:]
	switch cmd {
	case 1:
		if len(body) > stream_parser.SMB2QueryInfoMaxBodyBytes {
			return nil, protocolError(ErrResourceExceeded, "SMB2 SESSION_SETUP body exceeds bounded profile")
		}
		if resp && info["Status"] != uint32(0) && info["Status"] != uint32(0xc0000016) {
			fields, err := stream_parser.DecodeSMB2ErrorBody(body)
			if err != nil {
				return nil, err
			}
			delete(fields, "Error Response")
			smb2OffsetRanges(fields, 64)
			info["Body"], info["Error Response"] = fields, true
			return info, nil
		}
		if !resp || info["Status"] == uint32(0) || info["Status"] == uint32(0xc0000016) {
			minimum, structure := 24, uint16(25)
			if resp {
				minimum, structure = 8, 9
			}
			if len(body) < minimum || binary.LittleEndian.Uint16(body) != structure {
				if !resp && len(body) >= 2 && binary.LittleEndian.Uint16(body) == 17 {
					return nil, protocolError(ErrUnsupportedVersion, "SESSION_SETUP observed StructureSize 17; supported modern request layout requires 25")
				}
				return nil, fmt.Errorf("smb2: SESSION_SETUP complete %d-byte fixed body and StructureSize %d required", minimum, structure)
			}
		}
		at := 12
		if resp {
			at = 4
		}
		if len(body) >= at+4 && (!resp || info["Status"] == uint32(0) || info["Status"] == uint32(0xc0000016)) {
			offset, n := int(binary.LittleEndian.Uint16(body[at:at+2])), int(binary.LittleEndian.Uint16(body[at+2:at+4]))
			minimum := 88
			if resp {
				minimum = 72
				info["Session Flags"] = binary.LittleEndian.Uint16(body[2:4])
				info["Guest Session"] = binary.LittleEndian.Uint16(body[2:4])&1 != 0
			}
			if n > 0 {
				if offset < minimum || offset > len(pdu) || n > len(pdu)-offset {
					return nil, fmt.Errorf("smb2: security buffer outside command")
				}
				info["Authentication"] = decodeAuthenticationEnvelope(pdu[offset : offset+n])
			}
		}
	case 0:
		if err := smb2NegotiateFields(info, pdu, resp, max); err != nil {
			return nil, err
		}
	case 3:
		if !resp && len(body) >= 8 {
			off := int(binary.LittleEndian.Uint16(body[4:6]))
			n := int(binary.LittleEndian.Uint16(body[6:8]))
			at := off - smb2HeaderSize
			if at >= 0 && at+n <= len(body) {
				info["Path"] = smb2UTF16LE(body[at : at+n])
			}
		}
	case 5:
		if err := smb2CreateFields(info, pdu, resp, max); err != nil {
			return nil, err
		}
	case 6:
		if err := smb2CloseFields(info, pdu, resp); err != nil {
			return nil, err
		}
	case 8, 9:
		if !resp && len(body) >= 32 {
			info["FileId"] = hex.EncodeToString(body[16:32])
			if cmd == 8 {
				info["Length"] = binary.LittleEndian.Uint32(body[4:8])
			} else {
				info["Length"] = binary.LittleEndian.Uint32(body[4:8])
			}
		} else if resp && cmd == 9 && len(body) >= 8 {
			info["Count"] = binary.LittleEndian.Uint32(body[4:8])
		}
	case 16:
		if len(body) > stream_parser.SMB2QueryInfoMaxBodyBytes {
			return nil, protocolError(ErrResourceExceeded, "SMB2 QUERY_INFO body exceeds bounded profile")
		}
		status, _ := info["Status"].(uint32)
		fields, err := stream_parser.DecodeSMB2QueryInfoBody(body, resp, status)
		if err != nil {
			return nil, err
		}
		if fields["Error Response"] == true {
			info["Error Response"] = true
			delete(fields, "Error Response")
		} else if resp {
			info["Output Content Status"] = "opaque: request context unavailable"
		} else {
			// Keep the historical flat representation distinct from the owned
			// raw body leaf; both spans refer to this request's actual FileId.
			fid := fields["File ID"].([]byte)
			smb2Literal(info, "FileId", 88, 104, hex.EncodeToString(fid))
		}
		smb2OffsetRanges(fields, 64)
		info["Body"] = fields
	}
	return info, nil
}

func smb2CommandName(cmd uint16) string {
	switch cmd {
	case 0:
		return "NEGOTIATE"
	case 1:
		return "SESSION_SETUP"
	case 2:
		return "LOGOFF"
	case 3:
		return "TREE_CONNECT"
	case 4:
		return "TREE_DISCONNECT"
	case 5:
		return "CREATE"
	case 6:
		return "CLOSE"
	case 7:
		return "FLUSH"
	case 8:
		return "READ"
	case 9:
		return "WRITE"
	case 10:
		return "LOCK"
	case 11:
		return "IOCTL"
	case 12:
		return "CANCEL"
	case 13:
		return "ECHO"
	case 14:
		return "QUERY_DIRECTORY"
	case 15:
		return "CHANGE_NOTIFY"
	case 16:
		return "QUERY_INFO"
	case 17:
		return "SET_INFO"
	case 18:
		return "OPLOCK_BREAK"
	}
	return fmt.Sprintf("CMD_%d", cmd)
}

func smb2DialectName(d uint16) string {
	switch d {
	case 0x02ff:
		return "wildcard (SMB2 negotiate required)"
	case 0x0202:
		return "2.0.2"
	case 0x0210:
		return "2.1"
	case 0x0300:
		return "3.0"
	case 0x0302:
		return "3.0.2"
	case 0x0311:
		return "3.1.1"
	}
	return "2/3"
}

func smb2UTF16LE(b []byte) string {
	if len(b)%2 != 0 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(u))
}
