package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"
)

// binSMB2 is the M0 session state for SMB2/SMB3 over Direct TCP. Dialect
// comes from NEGOTIATE. Encrypted transform headers are a boundary, not
// decryption. Ports including 445 are never consulted.
type binSMB2 struct {
	flow    *binFlow
	client  int
	dialect uint16
	pending map[smb2PendingKey]smb2Pending
	async   map[smb2AsyncKey]smb2PendingKey
	files   map[smb2FileKey]string
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
		_ = limit
		return probeAccept("smb2", "2/3", 94)
	case smb3Transform:
		if n <= 52 {
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
	s := f.smb2
	if s == nil {
		return 0, nil, sessionContext("SMB2 session was not observed")
	}
	s.flow = f
	if err := s.reserveState(0); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	if w[0] != 0 {
		return 0, nil, fmt.Errorf("smb2: Direct TCP zero byte required")
	}
	n := smb2DirectLength(w)
	if n < 52 {
		return 0, nil, fmt.Errorf("smb2: Direct TCP length smaller than transform header")
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
		return total, f.spec("smb3", "SMB3Transform"), nil
	case smb2Magic:
		if n < smb2HeaderSize {
			return 0, nil, fmt.Errorf("smb2: Direct TCP length smaller than header")
		}
		if binary.LittleEndian.Uint16(w[8:10]) != 64 {
			return 0, nil, fmt.Errorf("smb2: header StructureSize must be 64")
		}
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
	for _, pending := range s.pending {
		pathBytes += int64(len(pending.path))
	}
	for _, path := range s.files {
		pathBytes += int64(len(path))
	}
	target := int64(256) + int64(len(s.pending)+1)*96 + int64(len(s.async)+1)*32 + int64(len(s.files)+1)*64
	target += pathBytes + additionalPathBytes
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
	if s.dialect != 0 {
		info["Dialect"] = s.dialect
		info["Version Name"] = smb2DialectName(s.dialect)
	}
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		names = append(names, fmt.Sprint(c["Packet Name"]))
		if err := s.associateFrom(c, max, dir); err != nil {
			return nil, err
		}
		if name, _ := c["Packet Name"].(string); name == "NEGOTIATE" {
			if d, ok := c["Dialect"].(uint16); ok && d != 0 {
				s.dialect = d
				info["Dialect"] = d
				info["Version Name"] = smb2DialectName(d)
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
	mid, _ := c["Message ID"].(uint64)
	name, _ := c["Packet Name"].(string)
	sid, _ := c["Session ID"].(uint64)
	tid, _ := c["Tree ID"].(uint32)
	asyncID, _ := c["Async ID"].(uint64)
	resp, _ := c["Response"].(bool)
	key := smb2PendingKey{sessionID: sid, treeID: tid, messageID: mid}
	if !resp {
		if len(s.pending) >= max {
			return protocolError(ErrResourceExceeded, "SMB2 outstanding requests exceed budget")
		}
		if _, exists := s.pending[key]; exists {
			return protocolError(ErrMalformedMessage, "SMB2 MessageId reused within the same session/tree before response")
		}
		pending := smb2Pending{key: key, name: name, direction: dir, hasDirection: dir >= 0}
		pending.path, _ = c["File Name"].(string)
		pending.related, _ = c["Related File ID Placeholder"].(bool)
		clonePath := false
		if fileID, ok := c["FileId"].(string); ok && fileID != "" {
			if related, _ := c["Related File ID Placeholder"].(bool); related {
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
		if err := s.reserveState(int64(len(pending.path))); err != nil {
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

	if name == "CLOSE" && pending.related {
		if relatedFileID, _ := c["Related Previous File ID"].(string); relatedFileID != "" {
			pending.fileID = relatedFileID
			c["FileId"] = relatedFileID
			c["Inherited File ID"] = true
		}
	}
	if name == "CREATE" {
		fileID, _ := c["FileId"].(string)
		if fileID != "" {
			fileKey := smb2FileKey{sessionID: sid, treeID: tid, fileID: fileID}
			if len(s.files) >= max && s.files[fileKey] == "" {
				return protocolError(ErrResourceExceeded, "SMB2 open file contexts exceed budget")
			}
			pathDelta := int64(len(pending.path)) - int64(len(s.files[fileKey]))
			if s.files[fileKey] == "" {
				pathDelta = int64(len(pending.path))
			}
			if err := s.reserveState(pathDelta); err != nil {
				return err
			}
			s.files[fileKey] = pending.path
			c["File Context Status"] = "opened"
		}
	}
	if name == "CLOSE" && status == 0 {
		delete(s.files, smb2FileKey{sessionID: sid, treeID: tid, fileID: pending.fileID})
		c["File Context Status"] = "closed"
	}
	if name == "TREE_DISCONNECT" && status == 0 {
		s.removeTree(sid, tid)
		c["Tree Context Status"] = "disconnected"
	}
	if name == "LOGOFF" && status == 0 {
		s.removeSession(sid)
		c["Session Context Status"] = "logged-off"
	}
	return nil
}

func (s *binSMB2) findResponse(key smb2PendingKey, asyncID uint64, name string, c map[string]any) (smb2PendingKey, smb2Pending, bool) {
	if asyncID != 0 {
		if pendingKey, ok := s.async[smb2AsyncKey{sessionID: key.sessionID, asyncID: asyncID}]; ok {
			pending, exists := s.pending[pendingKey]
			return pendingKey, pending, exists && pending.name == name
		}
		status, _ := c["Status"].(uint32)
		if status != 0x00000103 { // STATUS_PENDING still carries the request MessageId.
			return smb2PendingKey{}, smb2Pending{}, false
		}
	}
	if pending, ok := s.pending[key]; ok {
		return key, pending, pending.name == name
	}
	// SESSION_SETUP assigns the SessionId in the response; TREE_CONNECT
	// assigns the TreeId. Only allow their exact command-specific fallback.
	allowSessionSetup := name == "SESSION_SETUP"
	allowTreeConnect := name == "TREE_CONNECT"
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
		next := int(binary.LittleEndian.Uint32(h[20:24]))
		end := len(payload)
		if next != 0 {
			if next < smb2HeaderSize || next%8 != 0 || at+next > len(payload) {
				return nil, fmt.Errorf("smb2: invalid NextCommand")
			}
			end = at + next
		}
		cmd, err := smb2ParseCommand(h[:end-at])
		if err != nil {
			return nil, err
		}
		if related, _ := cmd["Related"].(bool); related {
			if previous == nil {
				return nil, protocolError(ErrMalformedMessage, "SMB2 related compound operation has no predecessor")
			}
			if sid, _ := cmd["Session ID"].(uint64); sid == ^uint64(0) {
				cmd["Session ID"] = previous["Session ID"]
				cmd["Inherited Session Context"] = true
			}
			if tid, _ := cmd["Tree ID"].(uint32); tid == ^uint32(0) {
				cmd["Tree ID"] = previous["Tree ID"]
				cmd["Inherited Tree Context"] = true
			}
			if fileID, _ := cmd["FileId"].(string); fileID == "ffffffffffffffffffffffffffffffff" {
				cmd["Related File ID Placeholder"] = true
				cmd["File Context Status"] = "related-compound-file-context-is-server-substituted"
			}
			if fileID, _ := previous["FileId"].(string); fileID != "" && fileID != "ffffffffffffffffffffffffffffffff" {
				cmd["Related Previous File ID"] = fileID
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

func smb2ParseCommand(pdu []byte) (map[string]any, error) {
	cmd := binary.LittleEndian.Uint16(pdu[12:14])
	flags := binary.LittleEndian.Uint32(pdu[16:20])
	mid := binary.LittleEndian.Uint64(pdu[24:32])
	resp := flags&smb2FlagResp != 0
	info := map[string]any{
		"Command":     cmd,
		"Packet Name": smb2CommandName(cmd),
		"Flags":       flags,
		"Response":    resp,
		"Message ID":  mid,
		"Credit":      binary.LittleEndian.Uint16(pdu[14:16]),
		"Status":      binary.LittleEndian.Uint32(pdu[8:12]),
	}
	if flags&smb2FlagAsync == 0 {
		info["Tree ID"] = binary.LittleEndian.Uint32(pdu[36:40])
		info["Session ID"] = binary.LittleEndian.Uint64(pdu[40:48])
	} else {
		info["Async ID"] = binary.LittleEndian.Uint64(pdu[32:40])
		info["Session ID"] = binary.LittleEndian.Uint64(pdu[40:48])
	}
	if flags&smb2FlagRelated != 0 {
		info["Related"] = true
	}
	body := pdu[smb2HeaderSize:]
	switch cmd {
	case 0:
		if !resp {
			if len(body) >= 4 {
				count := int(binary.LittleEndian.Uint16(body[2:4]))
				var dialects []uint16
				for i := 0; i < count && 36+2*(i+1) <= len(body); i++ {
					dialects = append(dialects, binary.LittleEndian.Uint16(body[36+2*i:]))
				}
				info["Dialects"] = dialects
			}
		} else if len(body) >= 6 {
			d := binary.LittleEndian.Uint16(body[4:6])
			info["Dialect"] = d
			info["Version Name"] = smb2DialectName(d)
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
		if !resp && len(body) >= 48 {
			off := int(binary.LittleEndian.Uint16(body[44:46]))
			n := int(binary.LittleEndian.Uint16(body[46:48]))
			at := off - smb2HeaderSize
			if at >= 0 && at+n <= len(body) {
				info["File Name"] = smb2UTF16LE(body[at : at+n])
			}
		} else if resp && len(body) >= 80 {
			info["FileId"] = hex.EncodeToString(body[64:80])
		}
	case 6:
		if len(body) >= 24 {
			info["FileId"] = hex.EncodeToString(body[8:24])
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
	case 8:
		return "READ"
	case 9:
		return "WRITE"
	case 11:
		return "IOCTL"
	case 13:
		return "ECHO"
	}
	return fmt.Sprintf("CMD_%d", cmd)
}

func smb2DialectName(d uint16) string {
	switch d {
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
