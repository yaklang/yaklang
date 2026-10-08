package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// binRDP is the M0 session state for MS-RDPBCGR TPKT/X.224 Cookie +
// negotiation, TLS/CredSSP boundary, and plaintext MCS/GCC. Port 3389
// is never consulted. Graphics, audio, and device redirection are out of scope.
type binRDP struct {
	client     int
	cookie     string
	requested  uint32
	selected   uint32
	failure    uint32
	sawCR      bool
	sawCC      bool
	tls        bool
	failed     bool
	channels   []string
	mcsInitial bool
}

const (
	rdpTPDUCR       = 0xe0
	rdpTPDUCC       = 0xd0
	rdpTPDUData     = 0xf0
	rdpTPDUDisc     = 0x80
	rdpNegReq       = 0x01
	rdpNegRsp       = 0x02
	rdpNegFailure   = 0x03
	rdpProtoRDP     = 0x00000000
	rdpProtoSSL     = 0x00000001
	rdpProtoHybrid  = 0x00000002
	rdpProtoRDSTLS  = 0x00000004
	rdpProtoHybridX = 0x00000008
)

func probeRDP(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] != 3 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 4 {
		return probeNeed("rdp", "bcgr", len(w), 11)
	}
	if w[1] != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 11 || n > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 7 {
		return probeNeed("rdp", "bcgr", len(w), min(n, 19))
	}
	code := w[5] & 0xf0
	if code != rdpTPDUCR && code != rdpTPDUCC {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 11 {
		return probeNeed("rdp", "bcgr", len(w), 11)
	}
	variable := w[11:min(len(w), n)]
	if rdpHasCookieOrNeg(variable) {
		_ = limit
		return probeAccept("rdp", "bcgr", 94)
	}
	if len(w) < n {
		return probeNeed("rdp", "bcgr", len(w), n)
	}
	return ProbeResult{Verdict: ProbeReject}
}

func rdpHasCookieOrNeg(variable []byte) bool {
	if bytes.HasPrefix(variable, []byte("Cookie:")) {
		return true
	}
	if len(variable) >= 8 && variable[0] >= rdpNegReq && variable[0] <= rdpNegFailure && binary.LittleEndian.Uint16(variable[2:4]) == 8 {
		return true
	}
	if i := bytes.Index(variable, []byte("\r\n")); i >= 0 && i+9 <= len(variable) {
		neg := variable[i+2:]
		if neg[0] >= rdpNegReq && neg[0] <= rdpNegFailure && binary.LittleEndian.Uint16(neg[2:4]) == 8 {
			return true
		}
	}
	return false
}

func (f *binFlow) frameRDP(w []byte) (int, *binSpec, error) {
	r := f.rdp
	if r == nil {
		return 0, nil, sessionContext("RDP session was not observed")
	}
	if err := f.reserveSession(512 + int64(len(r.channels))*16); err != nil {
		return 0, nil, err
	}
	if r.tls {
		if len(w) == 0 {
			return 0, nil, nil
		}
		return 0, nil, protocolError(ErrEncrypted, "RDP selected TLS/CredSSP; ciphertext is not parsed")
	}
	if len(w) < 4 {
		if len(w) > 0 && w[0] != 3 {
			return 0, nil, fmt.Errorf("rdp: expected TPKT version 3")
		}
		return 0, nil, nil
	}
	if w[0] != 3 || w[1] != 0 {
		if w[0] >= 20 && w[0] <= 23 && w[1] == 3 {
			return 0, nil, protocolError(ErrEncrypted, "RDP selected TLS/CredSSP; ciphertext is not parsed")
		}
		return 0, nil, fmt.Errorf("rdp: expected TPKT version 3")
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 7 {
		return 0, nil, fmt.Errorf("rdp: TPKT shorter than X.224 header")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	code := w[5] & 0xf0
	switch code {
	case rdpTPDUCR, rdpTPDUCC:
		return n, f.spec("msrdp", "RDP"), nil
	case rdpTPDUData, rdpTPDUDisc:
		return n, f.spec("msrdp", "TPKT"), nil
	default:
		return 0, nil, fmt.Errorf("rdp: unexpected X.224 TPDU 0x%02x", w[5])
	}
}

func (r *binRDP) consume(raw []byte) (map[string]any, error) {
	if len(raw) < 7 || raw[0] != 3 {
		return nil, fmt.Errorf("rdp: truncated TPKT")
	}
	n := int(binary.BigEndian.Uint16(raw[2:4]))
	if n != len(raw) {
		return nil, fmt.Errorf("rdp: TPKT length disagrees with framed message")
	}
	code := raw[5] & 0xf0
	info := map[string]any{
		"TPKT Version":  uint64(3),
		"TPDU Code":     raw[5],
		"Context Level": "observed",
	}
	switch code {
	case rdpTPDUCR:
		return r.consumeCR(raw, info)
	case rdpTPDUCC:
		return r.consumeCC(raw, info)
	case rdpTPDUData:
		return r.consumeMCS(raw, info)
	case rdpTPDUDisc:
		info["Packet Name"] = "DisconnectRequest"
		return info, nil
	default:
		return nil, fmt.Errorf("rdp: unexpected X.224 TPDU 0x%02x", raw[5])
	}
}

func (r *binRDP) consumeCR(raw []byte, info map[string]any) (map[string]any, error) {
	variable, err := rdpCRCCVariable(raw)
	if err != nil {
		return nil, err
	}
	cookie, user, neg, err := rdpParseVariable(variable)
	if err != nil {
		return nil, err
	}
	info["Packet Name"] = "ConnectionRequest"
	if cookie != "" {
		info["Cookie"] = cookie
		info["Username"] = user
		r.cookie = user
	}
	if neg != nil {
		if neg[0] != rdpNegReq {
			return info, fmt.Errorf("rdp: Connection Request negotiation type must be 1")
		}
		req := binary.LittleEndian.Uint32(neg[4:8])
		r.requested = req
		info["Negotiation Type"] = "RDP_NEG_REQ"
		info["Requested Protocols"] = req
		info["Requested Protocol Names"] = rdpProtocolNames(req)
	}
	r.sawCR = true
	info["Outstanding"] = true
	return info, nil
}

func (r *binRDP) consumeCC(raw []byte, info map[string]any) (map[string]any, error) {
	variable, err := rdpCRCCVariable(raw)
	if err != nil {
		return nil, err
	}
	_, _, neg, err := rdpParseVariable(variable)
	if err != nil {
		return nil, err
	}
	info["Packet Name"] = "ConnectionConfirm"
	if r.sawCR {
		info["Matched Request"] = "ConnectionRequest"
		info["Association Status"] = "matched"
	} else {
		info["Unmatched"] = true
		info["Association Status"] = "missing-request"
		info["Context Level"] = "partial"
	}
	r.sawCC = true
	if neg == nil {
		info["Selected Protocol"] = uint32(rdpProtoRDP)
		info["Selected Protocol Name"] = "PROTOCOL_RDP"
		return info, nil
	}
	switch neg[0] {
	case rdpNegRsp:
		sel := binary.LittleEndian.Uint32(neg[4:8])
		r.selected = sel
		info["Negotiation Type"] = "RDP_NEG_RSP"
		info["Selected Protocol"] = sel
		info["Selected Protocol Name"] = rdpSelectedName(sel)
		if sel&(rdpProtoSSL|rdpProtoHybrid|rdpProtoHybridX|rdpProtoRDSTLS) != 0 {
			r.tls = true
			info["TLS Expected"] = true
			if sel&(rdpProtoHybrid|rdpProtoHybridX) != 0 {
				info["Security"] = "CredSSP"
			} else {
				info["Security"] = "TLS"
			}
			info["Protocol Transition"] = "rdp->tls"
		} else {
			info["Security"] = "RDP"
		}
	case rdpNegFailure:
		code := binary.LittleEndian.Uint32(neg[4:8])
		r.failed, r.failure = true, code
		info["Negotiation Type"] = "RDP_NEG_FAILURE"
		info["Failure Code"] = code
		info["Failure Reason"] = rdpFailureName(code)
	default:
		return info, fmt.Errorf("rdp: Connection Confirm negotiation type must be 2 or 3")
	}
	return info, nil
}

func (r *binRDP) consumeMCS(raw []byte, info map[string]any) (map[string]any, error) {
	if r.tls {
		return info, protocolError(ErrEncrypted, "RDP MCS after TLS/CredSSP is not parsed without keys")
	}
	if len(raw) < 8 {
		return nil, fmt.Errorf("rdp: truncated X.224 Data")
	}
	if raw[5]&0xf0 != rdpTPDUData {
		return nil, fmt.Errorf("rdp: expected X.224 Data TPDU")
	}
	payload := raw[7:]
	kind, user, err := rdpMCSKind(payload)
	if err != nil {
		return nil, err
	}
	info["Packet Name"] = kind
	if key, blocks := rdpParseGCC(user); key != "" {
		info["GCC Key"] = key
		if names := rdpChannelNames(blocks); len(names) > 0 {
			info["Channels"] = names
			r.channels = names
		}
		if v, ok := rdpCoreVersion(blocks); ok {
			info["RDP Version"] = v
		}
	}
	switch kind {
	case "Connect-Initial":
		r.mcsInitial = true
		info["Outstanding"] = true
	case "Connect-Response":
		if r.mcsInitial {
			info["Matched Request"] = "Connect-Initial"
			info["Association Status"] = "matched"
		} else {
			info["Unmatched"] = true
			info["Association Status"] = "missing-request"
			info["Context Level"] = "partial"
		}
	}
	return info, nil
}

func rdpCRCCVariable(raw []byte) ([]byte, error) {
	if len(raw) < 11 {
		return nil, fmt.Errorf("rdp: truncated X.224 CR/CC")
	}
	li := int(raw[4])
	if 5+li != len(raw) {
		return nil, fmt.Errorf("rdp: X.224 length disagrees with TPKT")
	}
	if li < 6 {
		return nil, fmt.Errorf("rdp: X.224 header shorter than CR/CC fixed part")
	}
	return raw[11:], nil
}

func rdpParseVariable(variable []byte) (cookie, user string, neg []byte, err error) {
	rest := variable
	if bytes.HasPrefix(rest, []byte("Cookie:")) {
		i := bytes.Index(rest, []byte("\r\n"))
		if i < 0 {
			return "", "", nil, fmt.Errorf("rdp: truncated Cookie line")
		}
		cookie = string(rest[:i])
		user = rdpCookieUser(cookie)
		rest = rest[i+2:]
	}
	if len(rest) == 0 {
		return cookie, user, nil, nil
	}
	if len(rest) < 8 {
		return "", "", nil, fmt.Errorf("rdp: truncated negotiation")
	}
	if rest[0] < rdpNegReq || rest[0] > rdpNegFailure {
		return "", "", nil, fmt.Errorf("rdp: invalid negotiation type 0x%02x", rest[0])
	}
	if binary.LittleEndian.Uint16(rest[2:4]) != 8 {
		return "", "", nil, fmt.Errorf("rdp: negotiation length must be 8")
	}
	return cookie, user, rest[:8], nil
}

func rdpCookieUser(cookie string) string {
	const p = "Cookie: mstshash="
	if strings.HasPrefix(cookie, p) {
		return strings.TrimSpace(strings.TrimPrefix(cookie, p))
	}
	if i := strings.IndexByte(cookie, '='); i >= 0 {
		return strings.TrimSpace(cookie[i+1:])
	}
	return ""
}

func rdpMCSKind(payload []byte) (kind string, user []byte, err error) {
	if len(payload) < 2 {
		return "", nil, fmt.Errorf("rdp: truncated MCS PDU")
	}
	tag, body, err := rdpReadBER(payload)
	if err != nil {
		return "", nil, err
	}
	switch {
	case tag == 0x7f65:
		user, err = rdpMCSUserData(body, 6)
		return "Connect-Initial", user, err
	case tag == 0x7f66:
		user, err = rdpMCSUserData(body, 3)
		return "Connect-Response", user, err
	default:
		return "", nil, fmt.Errorf("rdp: expected MCS Connect-Initial/Response, got tag 0x%x", tag)
	}
}

func rdpReadBER(w []byte) (tag int, body []byte, err error) {
	if len(w) < 2 {
		return 0, nil, fmt.Errorf("rdp: truncated BER")
	}
	at := 0
	if w[0] == 0x7f {
		tag = 0x7f00 | int(w[1])
		at = 2
	} else {
		tag = int(w[0])
		at = 1
	}
	if at >= len(w) {
		return 0, nil, fmt.Errorf("rdp: truncated BER length")
	}
	n, h, err := berLength(w[at:])
	if err != nil || h == 0 || at+h+n > len(w) {
		return 0, nil, fmt.Errorf("rdp: truncated BER value")
	}
	return tag, w[at+h : at+h+n], nil
}

func rdpMCSUserData(body []byte, skip int) ([]byte, error) {
	at := 0
	for i := 0; i < skip; i++ {
		_, _, next, err := snmpReadTLV(body, at)
		if err != nil {
			return nil, fmt.Errorf("rdp: truncated MCS SEQUENCE")
		}
		at = next
	}
	tag, val, _, err := snmpReadTLV(body, at)
	if err != nil || tag != 0x04 {
		return nil, fmt.Errorf("rdp: expected MCS userData OCTET STRING")
	}
	return val, nil
}

func rdpParseGCC(user []byte) (key string, blocks [][]byte) {
	for _, magic := range []string{"Duca", "McDn"} {
		i := bytes.Index(user, []byte(magic))
		if i < 0 || i+6 > len(user) {
			continue
		}
		n := int(binary.BigEndian.Uint16(user[i+4 : i+6]))
		rest := user[i+6:]
		if n > len(rest) {
			n = len(rest)
		}
		rest = rest[:n]
		var out [][]byte
		for at := 0; at+4 <= len(rest); {
			ln := int(binary.LittleEndian.Uint16(rest[at+2 : at+4]))
			if ln < 4 || at+ln > len(rest) {
				break
			}
			out = append(out, rest[at:at+ln])
			at += ln
		}
		return magic, out
	}
	return "", nil
}

func rdpChannelNames(blocks [][]byte) []string {
	for _, b := range blocks {
		typ := binary.LittleEndian.Uint16(b[:2])
		if typ != 0xC003 || len(b) < 8 {
			continue
		}
		count := int(binary.LittleEndian.Uint32(b[4:8]))
		at := 8
		var names []string
		for i := 0; i < count && at+12 <= len(b); i++ {
			name := string(bytes.TrimRight(b[at:at+8], "\x00"))
			if name != "" {
				names = append(names, name)
			}
			at += 12
		}
		return names
	}
	return nil
}

func rdpCoreVersion(blocks [][]byte) (uint32, bool) {
	for _, b := range blocks {
		typ := binary.LittleEndian.Uint16(b[:2])
		if (typ == 0xC001 || typ == 0x0C01) && len(b) >= 8 {
			return binary.LittleEndian.Uint32(b[4:8]), true
		}
	}
	return 0, false
}

func rdpProtocolNames(mask uint32) []string {
	if mask == 0 {
		return []string{"PROTOCOL_RDP"}
	}
	var names []string
	if mask&rdpProtoSSL != 0 {
		names = append(names, "PROTOCOL_SSL")
	}
	if mask&rdpProtoHybrid != 0 {
		names = append(names, "PROTOCOL_HYBRID")
	}
	if mask&rdpProtoRDSTLS != 0 {
		names = append(names, "PROTOCOL_RDSTLS")
	}
	if mask&rdpProtoHybridX != 0 {
		names = append(names, "PROTOCOL_HYBRID_EX")
	}
	return names
}

func rdpSelectedName(sel uint32) string {
	switch sel {
	case rdpProtoRDP:
		return "PROTOCOL_RDP"
	case rdpProtoSSL:
		return "PROTOCOL_SSL"
	case rdpProtoHybrid:
		return "PROTOCOL_HYBRID"
	case rdpProtoRDSTLS:
		return "PROTOCOL_RDSTLS"
	case rdpProtoHybridX:
		return "PROTOCOL_HYBRID_EX"
	}
	return fmt.Sprintf("0x%08x", sel)
}

func rdpFailureName(code uint32) string {
	switch code {
	case 1:
		return "SSL_REQUIRED_BY_SERVER"
	case 2:
		return "SSL_NOT_ALLOWED_BY_SERVER"
	case 3:
		return "SSL_CERT_NOT_ON_SERVER"
	case 4:
		return "INCONSISTENT_FLAGS"
	case 5:
		return "HYBRID_REQUIRED_BY_SERVER"
	case 6:
		return "SSL_WITH_USER_AUTH_REQUIRED_BY_SERVER"
	}
	return fmt.Sprintf("failure_%d", code)
}
