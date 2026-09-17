package pcaputil

import (
	"encoding/binary"
	"fmt"
)

type binPostgres struct {
	frontend int // -1 until Startup/SSL/Cancel or unambiguous backend is seen
	tx       byte
	pid      uint32
	pending  int
	ssl      bool
}

func probePostgres(w []byte, limit int) ProbeResult {
	if len(w) < 8 {
		if len(w) > 0 && w[0] == 0 {
			return probeNeed("postgresql", "3.0", len(w), 8)
		}
		if len(w) >= 1 && postgresTyped(w[0]) {
			return probeNeed("postgresql", "3.0", len(w), 5)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if postgresTyped(w[0]) {
		n := int(binary.BigEndian.Uint32(w[1:5]))
		if n < 4 || n > 1<<20 {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeAccept("postgresql", "3.0", 70)
	}
	n := int(binary.BigEndian.Uint32(w[:4]))
	if n < 8 || n > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	code := binary.BigEndian.Uint32(w[4:8])
	if code == 80877103 || code == 80877102 || code == 80877104 || code == 196608 {
		return probeAccept("postgresql", "3.0", 95)
	}
	_ = limit
	return ProbeResult{Verdict: ProbeReject}
}

func postgresTyped(b byte) bool {
	switch b {
	case 'Q', 'P', 'B', 'D', 'E', 'S', 'C', 'X', 'H', 'p', 'F', 'd', 'c', 'f',
		'R', 'K', 'Z', 'T', '1', '2', '3', 'N', 'A', 'G', 'I', 'V', 'W', 'n', 't', 'v':
		return true
	}
	return false
}

func (f *binFlow) framePostgres(dir int, w []byte) (int, *binSpec, error) {
	p := f.pg
	if p == nil {
		return 0, nil, sessionContext("PostgreSQL session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	if p.ssl && p.frontend >= 0 && dir != p.frontend && len(w) >= 1 && (w[0] == 'S' || w[0] == 'N' || w[0] == 'G') {
		return 1, f.spec("postgresql_fields", "PostgreSQLSSLResponseFields"), nil
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	typed := postgresTyped(w[0])
	var n int
	if typed {
		if len(w) < 5 {
			return 0, nil, nil
		}
		n = 1 + int(binary.BigEndian.Uint32(w[1:5]))
		if n < 5 {
			return 0, nil, fmt.Errorf("postgresql: typed length too small")
		}
	} else {
		n = int(binary.BigEndian.Uint32(w[:4]))
		if n < 8 {
			return 0, nil, fmt.Errorf("postgresql: untyped length too small")
		}
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	entry, err := p.entry(dir, w[:n], typed)
	if err != nil {
		return 0, nil, err
	}
	return n, f.spec("postgresql_fields", entry), nil
}

func (p *binPostgres) entry(dir int, w []byte, typed bool) (string, error) {
	if !typed {
		code := binary.BigEndian.Uint32(w[4:8])
		p.frontend = dir
		switch code {
		case 80877103:
			p.ssl = true
			return "PostgreSQLSSLRequestFields", nil
		case 80877102:
			return "PostgreSQLCancelFields", nil
		case 196608:
			return "PostgreSQLStartupFields", nil
		default:
			return "", fmt.Errorf("postgresql: unknown untyped protocol %d", code)
		}
	}
	if len(w) == 1 {
		if p.frontend >= 0 && dir != p.frontend {
			return "PostgreSQLSSLResponseFields", nil
		}
		return "", sessionContext("PostgreSQL SSL response without a client SSLRequest")
	}
	typ := w[0]
	frontend := p.frontend == dir
	if p.frontend < 0 {
		switch typ {
		case 'R', 'K', 'Z', 'T', 'S', '1', '2', '3', 'N', 'A':
			p.frontend = 1 - dir
			frontend = false
		case 'Q', 'P', 'B', 'X', 'H', 'p':
			p.frontend = dir
			frontend = true
		case 'E':
			if postgresLooksLikeError(w[5:]) {
				p.frontend = 1 - dir
				frontend = false
			} else {
				p.frontend = dir
				frontend = true
			}
		default:
			return "", sessionContext("PostgreSQL ambiguous typed message before Startup")
		}
	}
	if frontend {
		return "PostgreSQLFrontendFields", nil
	}
	return "PostgreSQLBackendFields", nil
}

func postgresLooksLikeError(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	// Execute is portal cstring + int32. Error is (field type + cstring)+ + NUL.
	if body[0] == 0 && len(body) == 5 {
		return false
	}
	t := body[0]
	return t >= 'A' && t <= 'Z'
}

func (p *binPostgres) consume(dir int, raw []byte, entry string) (map[string]any, error) {
	info := map[string]any{
		"Entry":                     entry,
		"Frontend":                  p.frontend == dir,
		"Session State Validated":   false,
		"Context Level":             "observed",
	}
	if entry == "PostgreSQLSSLResponseFields" && len(raw) == 1 {
		info["Transport Response"] = string(raw)
		info["Message Name"] = "SSLResponse"
		p.ssl = false
		if raw[0] == 'S' {
			info["Encrypted"] = true
		}
		return info, nil
	}
	if len(raw) >= 5 && postgresTyped(raw[0]) {
		info["Type Code"] = string([]byte{raw[0]})
		name := postgresMessageName(raw[0], p.frontend == dir)
		info["Message Name"] = name
		switch name {
		case "Query", "Parse", "Bind", "Describe", "Execute", "Sync":
			p.pending++
			info["Outstanding"] = p.pending
		case "ReadyForQuery":
			if len(raw) >= 6 {
				p.tx = raw[5]
				info["Transaction Status"] = string([]byte{p.tx})
			}
			p.pending = 0
		case "CommandComplete", "ErrorResponse":
			if p.pending > 0 {
				p.pending--
			}
		case "BackendKeyData":
			if len(raw) >= 13 {
				p.pid = binary.BigEndian.Uint32(raw[5:9])
				info["Process ID"] = p.pid
			}
		}
	} else if len(raw) >= 8 {
		info["Message Name"] = entry
	}
	return info, nil
}

func postgresMessageName(typ byte, frontend bool) string {
	if frontend {
		switch typ {
		case 'Q':
			return "Query"
		case 'P':
			return "Parse"
		case 'B':
			return "Bind"
		case 'D':
			return "Describe"
		case 'E':
			return "Execute"
		case 'S':
			return "Sync"
		case 'C':
			return "Close"
		case 'X':
			return "Terminate"
		case 'H':
			return "Flush"
		}
		return string([]byte{typ})
	}
	switch typ {
	case 'R':
		return "Authentication"
	case 'E':
		return "ErrorResponse"
	case 'Z':
		return "ReadyForQuery"
	case 'T':
		return "RowDescription"
	case 'D':
		return "DataRow"
	case 'C':
		return "CommandComplete"
	case 'K':
		return "BackendKeyData"
	case '1':
		return "ParseComplete"
	case '2':
		return "BindComplete"
	case '3':
		return "CloseComplete"
	case 'S':
		return "ParameterStatus"
	case 'N':
		return "NoticeResponse"
	}
	return string([]byte{typ})
}
