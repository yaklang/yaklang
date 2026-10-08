package pcaputil

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type mysqlStatement struct {
	id              uint32
	params, columns int
	types           []uint16
	result          []map[string]any
	cursor          bool
}
type mysqlPreparedState struct {
	statements map[uint32]*mysqlStatement
	active     *mysqlStatement
	left       int
	stage      string
	request    uint64
	rows       uint64
}

func (m *binMySQL) preparedMemory() int64 {
	n := int64(512)
	for _, s := range m.prepared.statements {
		n += 128 + int64(len(s.types)*2)
		n += int64(sessionSnapshotBytes(s.result))
	}
	return n
}
func (m *binMySQL) preparedPacket(w []byte) bool {
	return len(w) > 4 && (m.phase == "command" && (w[4] >= 0x16 && w[4] <= 0x1a || w[4] == 0x1c) || m.phase == "prepared")
}
func (f *binFlow) frameMySQLPrepared(dir int, w []byte) (int, *binSpec, error) {
	m := f.mysql
	n := mysqlPacketLength(w)
	if m.caps&(1<<25) != 0 || m.mariaCaps&(1<<4) != 0 {
		return 0, nil, protocolError(ErrUnsupportedFeature, "MySQL optional/extended prepared metadata requires another profile")
	}
	if err := f.reserveSession(m.preparedMemory() + int64(n)*8); err != nil {
		return 0, nil, err
	}
	expected := m.seq
	if m.phase == "command" {
		expected = 0
		if dir == m.server {
			return 0, nil, sessionContext("MySQL prepared command from server")
		}
	} else if dir != m.server {
		return 0, nil, fmt.Errorf("mysql: command before prepared response complete")
	}
	if w[3] != expected {
		return 0, nil, fmt.Errorf("mysql: prepared packet sequence mismatch")
	}
	if w[4] == 0x18 && m.phase == "command" {
		return 0, nil, protocolError(ErrUnsupportedFeature, "MySQL LONG_DATA requires another profile")
	}
	// Semantic decoder handles this stateful packet; no auth-response/tree fallback.
	spec := *f.spec("mysql_fields", "MySQLAuthResponseFields")
	spec.entry = "MySQLPreparedFields"
	return n, &spec, nil
}
func (f *binFlow) consumeMySQLPrepared(dir int, e *ProtocolEvent) (map[string]any, error) {
	m := f.mysql
	p := &m.prepared
	w := e.Raw
	b := w[4:]
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	info := map[string]any{"Profile": "prepared", "Phase": m.phase, "Session State Validated": true, "Server Direction": m.server}
	if p.statements == nil {
		p.statements = map[uint32]*mysqlStatement{}
	}
	m.seq = w[3] + 1
	limit := f.a.budget.MaxCollectionElements
	fail := func(msg string) (map[string]any, error) { return nil, fmt.Errorf("mysql: %s", msg) }
	if m.phase == "command" {
		p.request = e.ID
		e.TransactionID = e.ID
		m.transaction++
		m.command = b[0]
		m.phase = "prepared"
		p.rows = 0
		info["Command"] = map[byte]string{0x16: "COM_STMT_PREPARE", 0x17: "COM_STMT_EXECUTE", 0x19: "COM_STMT_CLOSE", 0x1a: "COM_STMT_RESET", 0x1c: "COM_STMT_FETCH"}[b[0]]
		if b[0] == 0x16 {
			if len(b) < 2 {
				return fail("empty prepared SQL")
			}
			p.stage = "prepare"
			p.active = nil
			info["SQL"] = string(b[1:])
		} else {
			if len(b) < 5 {
				return fail("truncated statement ID")
			}
			id := binary.LittleEndian.Uint32(b[1:])
			info["Statement ID"] = id
			s, ok := p.statements[id]
			if !ok {
				return nil, sessionContext("MySQL unknown prepared statement")
			}
			p.active = s
			switch b[0] {
			case 0x19:
				if len(b) != 5 {
					return fail("CLOSE trailing bytes")
				}
				delete(p.statements, id)
				p.active = nil
				m.phase = "command"
				info["Response Expected"] = false
			case 0x1a:
				if len(b) != 5 {
					return fail("RESET trailing bytes")
				}
				p.stage = "reset"
			case 0x1c:
				if len(b) != 9 {
					return fail("FETCH length")
				}
				if !s.cursor {
					return nil, sessionContext("MySQL FETCH without observed cursor")
				}
				p.stage = "rows"
				info["Fetch Rows"] = binary.LittleEndian.Uint32(b[5:])
			case 0x17:
				if len(b) < 10 || binary.LittleEndian.Uint32(b[6:10]) != 1 {
					return fail("EXECUTE header")
				}
				if b[5] > 1 {
					return nil, protocolError(ErrUnsupportedFeature, "MySQL cursor flag unsupported")
				}
				if m.caps&(1<<27) != 0 {
					return nil, protocolError(ErrUnsupportedFeature, "MySQL prepared query attributes unsupported")
				}
				at := 10
				var params []map[string]any
				if s.params > 0 {
					nb := (s.params + 7) / 8
					if len(b)-at < nb+1 {
						return fail("EXECUTE null bitmap")
					}
					nulls := b[at : at+nb]
					at += nb
					fresh := b[at]
					at++
					if fresh > 1 {
						return fail("EXECUTE parameter type flag")
					}
					if fresh == 1 {
						if len(b)-at < s.params*2 {
							return fail("EXECUTE parameter types")
						}
						s.types = make([]uint16, s.params)
						for i := range s.types {
							s.types[i] = binary.LittleEndian.Uint16(b[at:])
							if s.types[i]&0x7f00 != 0 {
								return fail("EXECUTE reserved type flags")
							}
							at += 2
						}
					}
					if len(s.types) != s.params {
						return nil, sessionContext("MySQL EXECUTE missing reused types")
					}
					for i, t := range s.types {
						v := map[string]any{"Type": byte(t), "Unsigned": t&0x8000 != 0, "NULL": nulls[i/8]&(1<<uint(i%8)) != 0}
						if v["NULL"] != true {
							_, n, err := stream_parser.DecodeMySQLBinaryValue(b[at:], byte(t), t&0x8000 != 0)
							if err != nil {
								return nil, err
							}
							at += n
							v["Redacted"] = true
							v["Octets"] = n
						}
						params = append(params, v)
					}
				}
				if at != len(b) {
					return fail("EXECUTE trailing bytes")
				}
				info["Parameters"] = params
				info["Parameter Types"] = append([]uint16(nil), s.types...)
				p.stage = "result"
				s.cursor = b[5] == 1
			default:
				return nil, protocolError(ErrUnsupportedFeature, "MySQL prepared command unsupported")
			}
		}
	} else {
		e.ResponseTo = p.request
		e.TransactionID = p.request
		if p.active != nil {
			info["Statement ID"] = p.active.id
		}
		if b[0] == 0xff {
			if len(b) < 9 {
				return fail("ERR length")
			}
			info["Error Code"] = binary.LittleEndian.Uint16(b[1:])
			info["Response Complete"] = true
			m.phase = "command"
			if p.active != nil {
				if p.stage == "params" || p.stage == "prepare-columns" || p.stage == "params-eof" || p.stage == "prepare-eof" {
					delete(p.statements, p.active.id)
				}
				p.active.cursor = false
			}
			p.stage = ""
			return info, nil
		}
		deprecated := m.caps&(1<<24) != 0
		finish := func(status uint16) error {
			if status&8 != 0 && m.caps&(1<<18) == 0 {
				return fmt.Errorf("mysql: prepared multiple results without negotiated capability")
			}
			more := status&8 != 0
			info["More Results"] = more
			info["Response Complete"] = !more
			info["Status Flags"] = status
			if more {
				p.stage = "result"
			} else {
				m.phase = "command"
			}
			if p.active != nil {
				p.active.cursor = status&0x40 != 0 && status&0x80 == 0
			}
			info["Rows Observed"] = p.rows
			return nil
		}
		eof := func() (uint16, error) {
			if b[0] != 0xfe {
				return 0, fmt.Errorf("mysql: expected result terminator")
			}
			if !deprecated {
				if len(b) != 5 {
					return 0, fmt.Errorf("mysql: EOF length")
				}
				return binary.LittleEndian.Uint16(b[3:]), nil
			}
			_, u, err := mysqlLengthInteger(b[1:])
			if err != nil {
				return 0, err
			}
			_, v, err := mysqlLengthInteger(b[1+u:])
			if err != nil || len(b) < 1+u+v+4 {
				return 0, fmt.Errorf("mysql: OK terminator length")
			}
			return binary.LittleEndian.Uint16(b[1+u+v:]), nil
		}
		switch p.stage {
		case "prepare":
			if len(b) != 12 || b[0] != 0 || b[9] != 0 {
				return fail("PREPARE_OK layout")
			}
			s := &mysqlStatement{id: binary.LittleEndian.Uint32(b[1:]), columns: int(binary.LittleEndian.Uint16(b[5:])), params: int(binary.LittleEndian.Uint16(b[7:]))}
			if s.columns+s.params > limit || len(p.statements) >= limit {
				return nil, protocolError(ErrResourceExceeded, "MySQL prepared metadata limit")
			}
			p.statements[s.id] = s
			p.active = s
			info["Statement ID"] = s.id
			info["Parameter Count"] = s.params
			info["Column Count"] = s.columns
			p.left = s.params
			p.stage = "params"
			if p.left == 0 {
				p.left = s.columns
				p.stage = "prepare-columns"
			}
			if p.left == 0 {
				m.phase = "command"
				info["Response Complete"] = true
			}
		case "params", "prepare-columns", "result-columns":
			c, err := stream_parser.DecodeMySQLColumnPacket(w, m.mariaCaps&(1<<4) != 0)
			if err != nil {
				return nil, err
			}
			info["Column"] = c
			if p.stage != "params" {
				p.active.result = append(p.active.result, c)
			}
			p.left--
			if p.left == 0 {
				switch p.stage {
				case "params":
					if deprecated {
						p.left = p.active.columns
						p.stage = "prepare-columns"
						if p.left == 0 {
							m.phase = "command"
						}
					} else {
						p.stage = "params-eof"
					}
				case "prepare-columns":
					if deprecated {
						m.phase = "command"
						info["Response Complete"] = true
					} else {
						p.stage = "prepare-eof"
					}
				case "result-columns":
					if deprecated {
						p.stage = "rows"
					} else {
						p.stage = "columns-eof"
					}
				}
			}
		case "params-eof":
			if _, err := eof(); err != nil {
				return nil, err
			}
			p.left = p.active.columns
			p.stage = "prepare-columns"
			if p.left == 0 {
				m.phase = "command"
				info["Response Complete"] = true
			}
		case "prepare-eof":
			if _, err := eof(); err != nil {
				return nil, err
			}
			m.phase = "command"
			info["Response Complete"] = true
		case "columns-eof":
			status, err := eof()
			if err != nil {
				return nil, err
			}
			p.stage = "rows"
			if status&0x40 != 0 {
				if err := finish(status); err != nil {
					return nil, err
				}
			}
		case "result", "reset":
			if b[0] == 0 {
				_, u, err := mysqlLengthInteger(b[1:])
				if err != nil {
					return nil, err
				}
				_, v, err := mysqlLengthInteger(b[1+u:])
				if err != nil || len(b) < 1+u+v+4 {
					return fail("OK length")
				}
				if p.stage == "reset" {
					p.active.cursor = false
				}
				if err := finish(binary.LittleEndian.Uint16(b[1+u+v:])); err != nil {
					return nil, err
				}
			} else {
				if p.stage == "reset" {
					return fail("RESET requires OK or ERR")
				}
				n, u, err := mysqlLengthInteger(b)
				if err != nil || u != len(b) || n == 0 {
					return fail("binary result column count")
				}
				if n > uint64(limit) {
					return nil, protocolError(ErrResourceExceeded, "MySQL columns")
				}
				p.active.result = nil
				p.left = int(n)
				p.stage = "result-columns"
			}
		case "rows":
			if b[0] == 0xfe {
				status, err := eof()
				if err != nil {
					return nil, err
				}
				if err := finish(status); err != nil {
					return nil, err
				}
				break
			}
			if b[0] != 0 {
				return fail("binary row marker")
			}
			cols := p.active.result
			nb := (len(cols) + 9) / 8
			if len(cols) == 0 || len(b) < 1+nb {
				return fail("binary row null bitmap")
			}
			at := 1 + nb
			var values []map[string]any
			for i, c := range cols {
				typ := byte(c["Column Type"].(uint64))
				null := b[1+(i+2)/8]&(1<<uint((i+2)%8)) != 0
				v := map[string]any{"Type": typ, "NULL": null, "Name": c["Column Alias"]}
				if !null {
					value, n, err := stream_parser.DecodeMySQLBinaryValue(b[at:], typ, c["Column Flags"].(uint64)&32 != 0)
					if err != nil {
						return nil, err
					}
					at += n
					v["Value"] = value
				}
				values = append(values, v)
			}
			if at != len(b) {
				return fail("binary row trailing bytes")
			}
			p.rows++
			info["Values"] = values
		default:
			return fail("unknown prepared response phase")
		}
	}
	info["Next Phase"] = m.phase
	info["Prepared Stage"] = p.stage
	if err := f.reserveSession(m.preparedMemory()); err != nil {
		return nil, err
	}
	return info, nil
}
