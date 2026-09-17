package pcaputil

import (
	"encoding/binary"
	"fmt"
)

type binMySQL struct {
	server                                       int
	phase                                        string
	seq                                          byte
	serverCaps, serverMariaCaps, caps, mariaCaps uint64
	plugin                                       string
	maria                                        bool
	transaction                                  uint64
	command                                      byte
	// Incremental result framing resumes at complete packet boundaries.
	cursor, columns, stage, packets int
	resultSeq                       byte
}

func mysqlPacketLength(w []byte) int { return 4 + int(w[0]) + (int(w[1]) << 8) + (int(w[2]) << 16) }

func (f *binFlow) frameMySQL(dir int, w []byte) (int, *binSpec, error) {
	m := f.mysql
	if m == nil {
		return 0, nil, sessionContext("MySQL greeting was not observed")
	}
	if err := f.reserveSession(512); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	n := mysqlPacketLength(w)
	if n == 0xffffff+4 {
		return 0, nil, sessionContext("MySQL continuation packet exceeds supported message bound")
	}
	if n > 1<<20 {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > f.a.config.MaxMessageBytes || n > len(w) {
		return n, nil, nil
	}
	if n == 4 && m.phase != "auth-client" {
		return 0, nil, fmt.Errorf("mysql: empty packet outside authentication response")
	}
	server := dir == m.server
	entry := ""
	expected := m.seq
	switch m.phase {
	case "greeting":
		if !server {
			return 0, nil, sessionContext("MySQL client message before greeting")
		}
		entry = "MySQLGreetingFields"
	case "handshake":
		if server {
			return 0, nil, fmt.Errorf("mysql: server message while awaiting client handshake")
		}
		entry = "MySQLHandshakeResponse41Fields"
		if m.maria {
			entry = "MariaDBHandshakeResponse41Fields"
		}
		if n == 36 && binary.LittleEndian.Uint32(w[4:])&(1<<11) != 0 {
			entry = "MySQLSSLRequestFields"
			if m.maria {
				entry = "MariaDBSSLRequestFields"
			}
		}
	case "auth-server":
		if !server {
			return 0, nil, fmt.Errorf("mysql: unexpected client authentication message")
		}
		switch w[4] {
		case 0:
			entry = m.okEntry()
		case 0xff:
			entry = "MySQLError41Fields"
		case 0xfe:
			entry = "MySQLAuthSwitchFields"
		case 1:
			entry = "MySQLAuthMoreFields"
		default:
			return 0, nil, sessionContext("MySQL unsupported authentication exchange")
		}
	case "auth-client":
		if server {
			return 0, nil, fmt.Errorf("mysql: expected authentication continuation from client")
		}
		entry = "MySQLAuthResponseFields"
	case "command":
		if server {
			return 0, nil, sessionContext("MySQL response without observed command")
		}
		expected = 0
		if w[4] != 1 && w[4] != 2 && w[4] != 3 && w[4] != 14 {
			return 0, nil, sessionContext("MySQL unsupported command; prepared/binary commands require another profile")
		}
		if w[4] == 3 && m.caps&(1<<27) != 0 {
			return 0, nil, sessionContext("MySQL query attributes require another command profile")
		}
		entry = "MySQLCommandFields"
	case "response":
		if !server {
			return 0, nil, fmt.Errorf("mysql: command before preceding response completed")
		}
		switch w[4] {
		case 0:
			entry = m.okEntry()
		case 0xff:
			entry = "MySQLError41Fields"
		case 0xfb:
			return 0, nil, sessionContext("MySQL LOCAL INFILE transfer requires an explicit profile")
		default:
			if m.command != 3 {
				return 0, nil, fmt.Errorf("mysql: result set for non-query command")
			}
			if w[3] != expected {
				return 0, nil, fmt.Errorf("mysql: response sequence mismatch")
			}
			return f.frameMySQLResult(w)
		}
	case "closed":
		return 0, nil, sessionContext("MySQL message after observed session termination")
	default:
		return 0, nil, sessionContext("MySQL unsupported transport/phase")
	}
	if w[3] != expected {
		return 0, nil, fmt.Errorf("mysql: sequence %d, expected %d in %s", w[3], expected, m.phase)
	}
	return n, f.spec("mysql_fields", entry), nil
}

func (m *binMySQL) okEntry() string {
	if m.caps&(1<<23) != 0 {
		return "MySQLOKSessionTrackFields"
	}
	return "MySQLOK41Fields"
}

func mysqlLengthInteger(w []byte) (uint64, int, error) {
	if len(w) == 0 {
		return 0, 0, fmt.Errorf("mysql: missing length integer")
	}
	n := 0
	switch w[0] {
	case 0xfc:
		n = 2
	case 0xfd:
		n = 3
	case 0xfe:
		n = 8
	case 0xfb, 0xff:
		return 0, 0, fmt.Errorf("mysql: invalid count marker")
	default:
		return uint64(w[0]), 1, nil
	}
	if len(w) < 1+n {
		return 0, 0, fmt.Errorf("mysql: truncated length integer")
	}
	var v uint64
	for i := 0; i < n; i++ {
		v |= uint64(w[i+1]) << uint(8*i)
	}
	return v, n + 1, nil
}

func (f *binFlow) frameMySQLResult(w []byte) (int, *binSpec, error) {
	m := f.mysql
	deprecated := m.caps&(1<<24) != 0
	maria := m.maria && m.mariaCaps&(1<<4) != 0 && m.mariaCaps&(1<<3) != 0
	if m.mariaCaps&(1<<4) != 0 != (m.mariaCaps&(1<<3) != 0) {
		return 0, nil, sessionContext("MySQL partial MariaDB metadata capabilities")
	}
	entry := "MySQLTextResultSet41Fields"
	if maria {
		entry = "MariaDBTextResultSetFields"
	}
	if deprecated {
		if maria {
			return 0, nil, sessionContext("MySQL deprecated EOF with extended metadata requires another profile")
		}
		entry = "MySQLTextResultSetDeprecatedFields"
		if m.caps&(1<<23) != 0 {
			entry = "MySQLTextResultSetDeprecatedTrackFields"
		}
	}
	if m.cursor == 0 {
		n := mysqlPacketLength(w)
		count, used, err := mysqlLengthInteger(w[4:n])
		if err != nil {
			return 0, nil, err
		}
		if count == 0 || count > 4096 {
			return 0, nil, fmt.Errorf("mysql: column count outside 1..4096")
		}
		if maria {
			if n != 4+used+1 || w[n-1] != 1 {
				return 0, nil, sessionContext("MySQL cached/invalid metadata header")
			}
		} else if n != 4+used {
			return 0, nil, fmt.Errorf("mysql: invalid result header")
		}
		m.cursor, m.columns, m.stage, m.packets, m.resultSeq = n, int(count), 0, 1, w[3]
	}
	for {
		if len(w)-m.cursor < 4 {
			return 0, nil, nil
		}
		n := mysqlPacketLength(w[m.cursor:])
		end := m.cursor + n
		if end > 1<<20 {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		if end > f.a.config.MaxMessageBytes {
			return end, nil, nil
		}
		if end > len(w) {
			return 0, nil, nil
		}
		if n <= 4 {
			return 0, nil, fmt.Errorf("mysql: empty result packet")
		}
		if w[m.cursor+3] != m.resultSeq+1 {
			return 0, nil, fmt.Errorf("mysql: result sequence discontinuity")
		}
		m.resultSeq++
		m.packets++
		if m.packets > 8194 {
			return 0, nil, sessionContext("MySQL result packet limit")
		}
		body := w[m.cursor+4 : end]
		if body[0] == 0xff {
			return 0, nil, sessionContext("MySQL interrupted result set requires error-result profile")
		}
		switch m.stage {
		case 0:
			m.columns--
			if m.columns == 0 {
				if deprecated {
					m.stage = 2
				} else {
					m.stage = 1
				}
			}
		case 1:
			if len(body) != 5 || body[0] != 0xfe {
				return 0, nil, fmt.Errorf("mysql: missing metadata EOF")
			}
			m.stage = 2
		case 2:
			if body[0] == 0xfe && (!deprecated && len(body) == 5 || deprecated && len(body) >= 7 && len(body) < 0xffffff) {
				return end, f.spec("mysql_fields", entry), nil
			}
		}
		m.cursor = end
	}
}

func (m *binMySQL) consume(dir int, w []byte, entry string, result map[string]any) (map[string]any, error) {
	meta, ok := result["metadata"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mysql: missing validated profile metadata")
	}
	info := map[string]any{"Server Direction": m.server, "Phase": m.phase, "Transaction ID": m.transaction, "Payload Decrypted": false}
	m.seq = w[3] + 1
	switch m.phase {
	case "greeting":
		m.serverCaps, _ = meta["Capabilities"].(uint64)
		m.serverMariaCaps, _ = meta["MariaDB Extended Capabilities"].(uint64)
		m.plugin, _ = meta["Plugin Name"].(string)
		m.maria = m.serverCaps&1 == 0
		if m.serverCaps&(1<<9) == 0 {
			return nil, sessionContext("MySQL server does not offer protocol 4.1")
		}
		m.phase = "handshake"
	case "handshake":
		caps, _ := meta["Capabilities"].(uint64)
		m.caps = caps & m.serverCaps
		if m.caps&(1<<9) == 0 {
			return nil, fmt.Errorf("mysql: protocol 4.1 not negotiated")
		}
		m.mariaCaps, _ = meta["MariaDB Extended Capabilities"].(uint64)
		m.mariaCaps &= m.serverMariaCaps
		if plugin, ok := meta["Plugin Name"].(string); ok {
			m.plugin = plugin
		}
		info["Negotiated Capabilities"] = m.caps
		if entry == "MySQLSSLRequestFields" || entry == "MariaDBSSLRequestFields" {
			if m.caps&(1<<11) == 0 {
				return nil, fmt.Errorf("mysql: TLS not offered by server")
			}
			m.phase = "tls"
			info["TLS Requested"] = true
		} else {
			if m.caps&(1<<5|1<<26) != 0 {
				return nil, sessionContext("MySQL compressed transport requires another profile")
			}
			m.phase = "auth-server"
		}
	case "auth-server":
		switch w[4] {
		case 0:
			m.phase = "command"
			info["Authentication OK Observed"] = true
		case 0xff:
			m.phase = "closed"
		case 0xfe:
			m.plugin, _ = meta["Plugin Name"].(string)
			m.phase = "auth-client"
		case 1:
			data, _ := meta["Auth Data"].([]byte)
			if m.plugin != "caching_sha2_password" {
				return nil, sessionContext("MySQL AuthMoreData requires a supported authentication plugin")
			}
			if len(data) == 1 && data[0] == 3 {
				m.phase = "auth-server"
			} else {
				m.phase = "auth-client"
			}
		}
	case "auth-client":
		m.phase = "auth-server"
	case "command":
		m.transaction++
		m.command = w[4]
		info["Transaction ID"] = m.transaction
		info["Command"] = meta["Command Name"]
		if m.command == 1 {
			m.phase = "closed"
		} else {
			m.phase = "response"
		}
	case "response":
		status, _ := meta["Status Flags"].(uint64)
		if last, ok := meta["Result EOF"].(map[string]any); ok {
			status, _ = last["Status Flags"].(uint64)
			m.seq = m.resultSeq + 1
		}
		m.cursor, m.columns, m.stage, m.packets = 0, 0, 0, 0
		more := status&8 != 0
		if more && m.caps&(1<<17) == 0 {
			return nil, fmt.Errorf("mysql: multiple results without negotiated capability")
		}
		info["More Results"], info["Response Complete"] = more, !more
		if !more {
			m.phase = "command"
		}
	}
	info["Next Phase"] = m.phase
	return info, nil
}
