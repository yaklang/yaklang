package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"unicode/utf16"
)

// binSMB2 is the M0 session state for SMB2/SMB3 over Direct TCP. Dialect
// comes from NEGOTIATE. Encrypted transform headers are a boundary, not
// decryption. Ports including 445 are never consulted.
type binSMB2 struct {
	client  int
	dialect uint16
	pending map[uint64]string
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
	if err := f.reserveSession(256 + int64(len(s.pending))*24); err != nil {
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
		orig := int(binary.LittleEndian.Uint32(w[4+36 : 4+40]))
		if n < 52+orig && orig > 0 && n != 52 {
			// Direct TCP length covers header plus ciphertext.
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

func (s *binSMB2) consume(raw []byte, max int) (map[string]any, error) {
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
	payload := raw[4:]
	magic := binary.LittleEndian.Uint32(payload[:4])
	if magic == smb3Transform {
		if len(payload) < 52 {
			return nil, fmt.Errorf("smb2: truncated transform header")
		}
		sid := binary.LittleEndian.Uint64(payload[44:52])
		orig := binary.LittleEndian.Uint32(payload[36:40])
		return map[string]any{
			"Packet Name":   "transform",
			"Encrypted":     true,
			"Session ID":    sid,
			"Original Size": orig,
			"Context Level": "observed",
			"Version Name":  smb2DialectName(s.dialect),
		}, nil
	}
	cmds, err := smb2Walk(payload)
	if err != nil {
		return nil, err
	}
	if len(cmds) == 0 {
		return nil, fmt.Errorf("smb2: missing command")
	}
	if max <= 0 {
		max = 4096
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
		if err := s.associate(c, max); err != nil {
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
		info["Compound"] = true
		info["Compound Count"] = len(cmds)
		info["Command Names"] = names
		info["Commands"] = cmds
	}
	return info, nil
}

func (s *binSMB2) associate(c map[string]any, max int) error {
	mid, _ := c["Message ID"].(uint64)
	name, _ := c["Packet Name"].(string)
	resp, _ := c["Response"].(bool)
	if !resp {
		if len(s.pending) >= max {
			return protocolError(ErrResourceExceeded, "SMB2 outstanding MessageIds exceed budget")
		}
		s.pending[mid] = name
		c["Outstanding"] = true
		return nil
	}
	if want, ok := s.pending[mid]; ok {
		delete(s.pending, mid)
		c["Matched Request"] = want
		c["Association Status"] = "matched"
		if infoWant, _ := c["Packet Name"].(string); infoWant == "NEGOTIATE" || want == "NEGOTIATE" {
			// dialect already applied from the response body
		}
	} else {
		c["Unmatched"] = true
		c["Association Status"] = "missing-request"
		c["Context Level"] = "partial"
	}
	return nil
}

func smb2Walk(payload []byte) ([]map[string]any, error) {
	var cmds []map[string]any
	at := 0
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
		cmds = append(cmds, cmd)
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
