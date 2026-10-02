package pcaputil

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

// binTDS is the M0 session state for MS-TDS 7.x. Version comes from LOGIN7,
// encryption from PRELOGIN, and SQLBatch/RPC pair with the next type-4 token
// stream. Ports are never consulted.
type binTDS struct {
	client    int
	version   uint32
	version72 bool
	loggedIn  bool
	encrypt   byte // 0xff until an ENCRYPTION token is observed
	pending   []string
}

const tdsEncryptUnknown = 0xff

func tdsKnownType(typ byte) bool {
	switch typ {
	case 1, 2, 3, 4, 6, 7, 8, 14, 16, 17, 18:
		return true
	}
	return false
}

func tdsVersionKnown(ver uint32) bool {
	switch ver {
	case 0x71000001, 0x72090002, 0x730a0003, 0x730b0003, 0x74000004:
		return true
	}
	return false
}

func tdsVersionName(ver uint32) string {
	switch ver {
	case 0x71000001:
		return "7.1"
	case 0x72090002:
		return "7.2"
	case 0x730a0003:
		return "7.3"
	case 0x730b0003:
		return "7.3A"
	case 0x74000004:
		return "7.4"
	}
	return "7.x"
}

func tdsPacketName(typ byte) string {
	switch typ {
	case 1:
		return "SQLBatch"
	case 3:
		return "RPC"
	case 4:
		return "Tabular Result"
	case 6:
		return "Attention"
	case 14:
		return "Transaction Manager"
	case 16:
		return "LOGIN7"
	case 17:
		return "SSPI"
	case 18:
		return "PRELOGIN"
	}
	return fmt.Sprintf("Type %d", typ)
}

func tdsEncryptName(v byte) string {
	switch v {
	case 0:
		return "ENCRYPT_OFF"
	case 1:
		return "ENCRYPT_ON"
	case 2:
		return "ENCRYPT_NOT_SUP"
	case 3:
		return "ENCRYPT_REQ"
	}
	return "UNKNOWN"
}

func probeTDS(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] != 0x12 && w[0] != 0x10 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 8 {
		return probeNeed("tds", "7.x", len(w), 8)
	}
	// Window is required to be 0. Status bits are EOM/IGNORE/RESET*.
	if w[7] != 0 || w[1]&^0x1b != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n := int(binary.BigEndian.Uint16(w[2:4]))
	if n < 8 || n > 1<<20 || binary.BigEndian.Uint16(w[4:6]) != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] == 0x12 {
		_ = limit
		return probeAccept("tds", "7.x", 92)
	}
	if n < 16 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 16 {
		return probeNeed("tds", "7.x", len(w), 16)
	}
	loginLen := binary.LittleEndian.Uint32(w[8:12])
	ver := binary.LittleEndian.Uint32(w[12:16])
	if loginLen < 8 || loginLen > 1<<20 || int(loginLen) < n-8 && n > 16 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if !tdsVersionKnown(ver) {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("tds", tdsVersionName(ver), 90)
}

func (f *binFlow) frameTDS(w []byte) (int, *binSpec, error) {
	t := f.tds
	if t == nil {
		return 0, nil, sessionContext("TDS session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(t.pending))*32); err != nil {
		return 0, nil, err
	}
	at := 0
	var typ byte
	for packets := 0; ; packets++ {
		if len(w) < at+8 {
			return 0, nil, nil
		}
		h := w[at : at+8]
		if h[7] != 0 {
			return 0, nil, fmt.Errorf("tds: window must be 0")
		}
		n := int(binary.BigEndian.Uint16(h[2:4]))
		if n < 8 {
			return 0, nil, fmt.Errorf("tds: length smaller than header")
		}
		if packets == 0 {
			typ = h[0]
			if !tdsKnownType(typ) {
				return 0, nil, fmt.Errorf("tds: unknown packet type %d", typ)
			}
		} else if h[0] != typ {
			return 0, nil, fmt.Errorf("tds: packet type changed before EOM")
		}
		if n > f.a.config.MaxMessageBytes || at+n > f.a.config.MaxMessageBytes {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		if at+n > len(w) {
			return at + n, nil, nil
		}
		at += n
		if h[1]&1 == 1 {
			break
		}
	}
	spec, err := t.specFor(typ, f)
	if err != nil {
		return 0, nil, err
	}
	return at, spec, nil
}

func (t *binTDS) specFor(typ byte, f *binFlow) (*binSpec, error) {
	switch typ {
	case 18, 16, 6, 14, 17, 2:
		return f.spec("tds", "TDS"), nil
	case 4:
		if t.version == 0 {
			return f.spec("tds", "TDS"), nil
		}
		if t.version72 {
			return f.spec("tds_fields", "TDSResponse72Fields"), nil
		}
		return f.spec("tds_fields", "TDSResponse71Fields"), nil
	case 1:
		if t.version == 0 {
			return nil, sessionContext("TDS SQLBatch requires observed LOGIN7 version")
		}
		if t.version72 {
			return f.spec("tds_fields", "TDSBatch72Fields"), nil
		}
		return f.spec("tds_fields", "TDSBatch71Fields"), nil
	case 3:
		if t.version == 0 {
			return nil, sessionContext("TDS RPC requires observed LOGIN7 version")
		}
		if t.version72 {
			return f.spec("tds_fields", "TDSRPC72Fields"), nil
		}
		return f.spec("tds_fields", "TDSRPC71Fields"), nil
	default:
		return nil, fmt.Errorf("tds: unsupported packet type %d", typ)
	}
}

func (t *binTDS) consume(dir int, raw []byte, result map[string]any, maxPending int) (map[string]any, error) {
	body, typ, err := tdsMessagePayload(raw)
	if err != nil {
		return nil, err
	}
	if t.client < 0 {
		t.client = dir
	}
	info := map[string]any{
		"Packet Type":    typ,
		"Packet Name":    tdsPacketName(typ),
		"Context Level":  "observed",
		"TDS Version":    t.version,
		"Version Name":   tdsVersionName(t.version),
		"Logged In":      t.loggedIn,
		"Session Client": t.client == dir,
	}
	if t.encrypt != tdsEncryptUnknown {
		info["Encryption"] = t.encrypt
		info["Encryption Name"] = tdsEncryptName(t.encrypt)
	}
	if meta, ok := result["metadata"].(map[string]any); ok {
		for _, key := range []string{"SQL Text", "RPC Requests", "Response Tokens", "TDS Packet Count", "Layout Context"} {
			if v, ok := meta[key]; ok {
				info[key] = v
			}
		}
	}
	switch typ {
	case 18:
		pre, err := tdsParsePrelogin(body)
		if err != nil {
			return nil, err
		}
		t.applyPrelogin(pre, info)
		if err := t.push("PRELOGIN", maxPending); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case 16:
		login, err := tdsParseLogin7(body)
		if err != nil {
			return nil, err
		}
		for k, v := range login {
			info[k] = v
		}
		ver := login["TDS Version"].(uint32)
		t.version = ver
		t.version72 = ver >= 0x72090002
		info["Version Name"] = tdsVersionName(ver)
		if err := t.push("LOGIN7", maxPending); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case 1:
		if text := tdsSQLText(info["SQL Text"]); text != "" {
			info["SQL Text"] = text
		}
		if err := t.push("SQLBatch", maxPending); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case 3:
		if rpcs, ok := info["RPC Requests"].([]map[string]any); ok && len(rpcs) > 0 {
			if id, ok := rpcs[0]["Procedure ID"]; ok {
				info["Procedure ID"] = id
			}
		}
		if err := t.push("RPC", maxPending); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case 4:
		if t.version == 0 {
			pre, err := tdsParsePrelogin(body)
			if err != nil {
				return nil, err
			}
			t.applyPrelogin(pre, info)
			info["Packet Name"] = "PRELOGIN Response"
			t.match("PRELOGIN", info)
			if t.encrypt == 1 || t.encrypt == 3 {
				info["Encrypted"] = true
				info["Protocol Transition"] = "tds->tls"
			}
			return info, nil
		}
		if toks, ok := info["Response Tokens"].([]map[string]any); ok {
			names := make([]string, 0, len(toks))
			for _, tok := range toks {
				if n, ok := tok["Name"].(string); ok {
					names = append(names, n)
				}
			}
			info["Token Names"] = names
		}
		t.match("", info)
		if name, _ := info["Matched Request"].(string); name == "LOGIN7" {
			t.loggedIn = true
			info["Logged In"] = true
		}
	case 6:
		if err := t.push("Attention", maxPending); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	default:
		info["Association Status"] = "untracked"
	}
	return info, nil
}

func (t *binTDS) applyPrelogin(pre, info map[string]any) {
	for k, v := range pre {
		info[k] = v
	}
	if enc, ok := pre["Encryption"].(byte); ok {
		t.encrypt = enc
		info["Encryption"] = enc
		info["Encryption Name"] = tdsEncryptName(enc)
	}
}

func (t *binTDS) push(name string, max int) error {
	if max <= 0 {
		max = 4096
	}
	if len(t.pending) >= max {
		return protocolError(ErrResourceExceeded, "TDS outstanding requests exceed budget")
	}
	t.pending = append(t.pending, name)
	return nil
}

func (t *binTDS) match(want string, info map[string]any) {
	if len(t.pending) == 0 {
		info["Unmatched"] = true
		info["Association Status"] = "missing-request"
		info["Context Level"] = "partial"
		return
	}
	if want != "" && t.pending[0] != want {
		info["Unmatched"] = true
		info["Association Status"] = "missing-request"
		info["Context Level"] = "partial"
		return
	}
	info["Matched Request"] = t.pending[0]
	info["Association Status"] = "matched"
	t.pending = t.pending[1:]
}

func tdsMessagePayload(raw []byte) ([]byte, byte, error) {
	if len(raw) < 8 {
		return nil, 0, fmt.Errorf("tds: truncated packet header")
	}
	var body []byte
	typ := raw[0]
	for at := 0; at < len(raw); {
		if len(raw)-at < 8 {
			return nil, 0, fmt.Errorf("tds: truncated packet header")
		}
		n := int(binary.BigEndian.Uint16(raw[at+2 : at+4]))
		if n < 8 || at+n > len(raw) {
			return nil, 0, fmt.Errorf("tds: packet length disagrees with framed message")
		}
		if raw[at] != typ {
			return nil, 0, fmt.Errorf("tds: packet type changed before EOM")
		}
		body = append(body, raw[at+8:at+n]...)
		eom := raw[at+1]&1 != 0
		at += n
		if eom {
			if at != len(raw) {
				return nil, 0, fmt.Errorf("tds: trailing bytes after EOM")
			}
			return body, typ, nil
		}
	}
	return nil, 0, fmt.Errorf("tds: missing EOM")
}

func tdsParsePrelogin(payload []byte) (map[string]any, error) {
	pos := 0
	var tokens []map[string]any
	encrypt := byte(tdsEncryptUnknown)
	for pos < len(payload) {
		tok := payload[pos]
		if tok == 0xff {
			break
		}
		if pos+5 > len(payload) {
			return nil, fmt.Errorf("tds: truncated PRELOGIN token")
		}
		off := int(binary.BigEndian.Uint16(payload[pos+1 : pos+3]))
		n := int(binary.BigEndian.Uint16(payload[pos+3 : pos+5]))
		if off < 0 || n < 0 || off+n > len(payload) {
			return nil, fmt.Errorf("tds: PRELOGIN token out of range")
		}
		data := payload[off : off+n]
		item := map[string]any{"Token": tok, "Offset": off, "Length": n, "Data": append([]byte(nil), data...)}
		switch tok {
		case 0:
			item["Name"] = "VERSION"
			if n >= 2 {
				item["Version Major"] = data[0]
				item["Version Minor"] = data[1]
			}
		case 1:
			item["Name"] = "ENCRYPTION"
			if n >= 1 {
				encrypt = data[0]
				item["Encryption"] = encrypt
				item["Encryption Name"] = tdsEncryptName(encrypt)
			}
		case 2:
			item["Name"] = "INSTOPT"
		case 3:
			item["Name"] = "THREADID"
		case 4:
			item["Name"] = "MARS"
		default:
			item["Name"] = fmt.Sprintf("Token %d", tok)
		}
		tokens = append(tokens, item)
		pos += 5
	}
	info := map[string]any{"PRELOGIN Tokens": tokens}
	if encrypt != tdsEncryptUnknown {
		info["Encryption"] = encrypt
		info["Encryption Name"] = tdsEncryptName(encrypt)
	}
	return info, nil
}

func tdsParseLogin7(payload []byte) (map[string]any, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("tds: truncated LOGIN7")
	}
	n := binary.LittleEndian.Uint32(payload[:4])
	if int(n) != len(payload) {
		return nil, fmt.Errorf("tds: LOGIN7 length %d disagrees with payload %d", n, len(payload))
	}
	ver := binary.LittleEndian.Uint32(payload[4:8])
	if !tdsVersionKnown(ver) {
		return nil, protocolError(ErrUnsupportedVersion, "TDS LOGIN7 version 0x%08x is outside the 7.x profile", ver)
	}
	info := map[string]any{"Login Length": n, "TDS Version": ver, "Version Name": tdsVersionName(ver)}
	if len(payload) >= 12 {
		info["Packet Size"] = binary.LittleEndian.Uint32(payload[8:12])
	}
	if len(payload) >= 40 {
		ib := int(binary.LittleEndian.Uint16(payload[36:38]))
		cch := int(binary.LittleEndian.Uint16(payload[38:40]))
		end := ib + cch*2
		if ib >= 40 && end <= len(payload) && cch >= 0 {
			info["HostName"] = tdsUTF16LE(payload[ib:end])
		}
	}
	return info, nil
}

func tdsUTF16LE(b []byte) string {
	if len(b)%2 != 0 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(u))
}

func tdsSQLText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		if s, ok := x["Text"].(string); ok {
			return s
		}
	}
	return ""
}
