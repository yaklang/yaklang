package pcaputil

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type postgresObject struct {
	statement string
	columns   []map[string]any
	formats   []uint64
}
type postgresOperation struct {
	id, cycle                     uint64
	kind, name, statement, target string
	ts                            time.Time
	formats                       []uint64
}
type postgresState struct {
	statements     map[string]postgresObject
	portals        map[string]postgresObject
	queue          []postgresOperation
	cycle          uint64
	clock          time.Time
	recovery, lost bool
	copyMode       string
	copyBytes      [2]uint64
	copyDone       [2]bool
	columns        []map[string]any
}

func (s *postgresState) storage() int64 {
	n := int64(512 + len(s.queue)*128)
	for _, q := range s.queue {
		n += int64(len(q.name) + len(q.statement) + 8*len(q.formats))
	}
	for k, v := range s.statements {
		n += int64(96 + len(k))
		n += int64(sessionSnapshotBytes(v.columns))
	}
	for k, v := range s.portals {
		n += int64(96 + len(k) + len(v.statement) + 8*len(v.formats))
		n += int64(sessionSnapshotBytes(v.columns))
	}
	n += int64(sessionSnapshotBytes(s.columns))
	return n
}
func (s *postgresState) pop() {
	s.queue[0] = postgresOperation{}
	s.queue = s.queue[1:]
	if len(s.queue) == 0 {
		s.queue = nil
	}
}
func (f *binFlow) consumePostgres(dir int, e *ProtocolEvent, result map[string]any) (map[string]any, error) {
	p := f.pg
	s := &p.state
	info, err := p.consume(dir, e.Raw, e.Entry)
	if err != nil {
		return info, err
	}
	meta, _ := result["metadata"].(map[string]any)
	for k, v := range meta {
		info[k] = v
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	if len(s.queue) > 0 && s.clock.Sub(s.queue[0].ts) > 30*time.Second {
		s.queue = nil
		s.lost = true
		s.copyMode = ""
		s.columns = nil
	}
	frontend := dir == p.frontend
	name, _ := info["Message Name"].(string)
	text := func(k string) string { v, _ := meta[k].(string); return v }
	if s.statements == nil {
		s.statements = map[string]postgresObject{}
		s.portals = map[string]postgresObject{}
	}
	if s.lost {
		info["Context Level"] = "expired-context"
		info["Session State Validated"] = false
		return info, nil
	}
	limit := f.a.budget.MaxCollectionElements
	if frontend {
		switch name {
		case "Query", "Parse", "Bind", "Describe", "Execute", "Close", "Sync":
			if s.recovery && name != "Sync" {
				info["Ignored Until Sync"] = true
				break
			}
			if len(s.queue) >= limit {
				return nil, protocolError(ErrResourceExceeded, "PostgreSQL pending operations")
			}
			if s.cycle == 0 {
				s.cycle = e.ID
			}
			q := postgresOperation{id: e.ID, cycle: s.cycle, kind: name, name: text("Portal"), statement: text("Statement"), target: text("Target Type"), ts: e.Timestamp}
			if name == "Parse" {
				q.name = q.statement
			}
			if name == "Describe" || name == "Close" {
				q.name = text("Target")
			}
			if name == "Bind" {
				q.formats, _ = meta["Result Formats"].([]uint64)
				q.formats = append([]uint64(nil), q.formats...)
				_, known := s.statements[q.statement]
				for _, pending := range s.queue {
					if pending.kind == "Parse" && pending.name == q.statement {
						known = true
					}
				}
				info["Statement Observed"] = known
			}
			if name == "Execute" {
				if portal, ok := s.portals[q.name]; ok {
					q.statement = portal.statement
					info["Statement"] = portal.statement
					info["Portal Observed"] = true
				}
			}
			s.queue = append(s.queue, q)
			e.TransactionID = q.cycle
			info["Cycle ID"] = q.cycle
			if name == "Sync" || name == "Query" {
				s.cycle = 0
			}
		case "CopyData", "CopyDone", "CopyFail":
			if s.copyMode != "in" && s.copyMode != "both" {
				return nil, sessionContext("PostgreSQL frontend COPY without observed COPY IN/BOTH")
			}
		}
	} else {
		switch name {
		case "NotificationResponse", "NoticeResponse", "ParameterStatus":
			info["Asynchronous"] = true
		case "Authentication", "BackendKeyData":
		default:
			if len(s.queue) == 0 {
				info["Unmatched"] = true
				break
			}
			q := s.queue[0]
			if q.kind == "Execute" || q.kind == "Describe" && q.target == "P" {
				if o, ok := s.portals[q.name]; ok {
					q.statement = o.statement
				}
			}
			e.ResponseTo = q.id
			e.TransactionID = q.cycle
			info["Cycle ID"] = q.cycle
			info["Operation"] = q.kind
			info["Statement"] = q.statement
			info["Portal"] = q.name
			valid := false
			complete := false
			switch name {
			case "ParseComplete":
				valid = q.kind == "Parse"
				complete = true
				if valid {
					s.statements[q.name] = postgresObject{statement: q.name}
				}
			case "BindComplete":
				valid = q.kind == "Bind"
				complete = true
				if valid {
					s.portals[q.name] = postgresObject{statement: q.statement, formats: q.formats}
				}
			case "CloseComplete":
				valid = q.kind == "Close"
				complete = true
				if valid {
					if q.target == "S" {
						delete(s.statements, q.name)
					} else {
						delete(s.portals, q.name)
					}
				}
			case "ParameterDescription":
				valid = q.kind == "Describe"
			case "RowDescription", "NoData":
				valid = q.kind == "Describe" || q.kind == "Query" || q.kind == "Execute"
				cols, _ := meta["Columns"].([]map[string]any)
				s.columns = cols
				if q.kind == "Describe" {
					complete = true
					if q.target == "P" {
						o := s.portals[q.name]
						o.columns = cols
						s.portals[q.name] = o
					} else {
						o := s.statements[q.name]
						o.columns = cols
						s.statements[q.name] = o
					}
				}
			case "DataRow":
				valid = q.kind == "Execute" || q.kind == "Query"
				cols := s.columns
				if q.kind == "Execute" {
					if o, ok := s.portals[q.name]; ok {
						cols = o.columns
						if len(cols) == 0 {
							cols = s.statements[o.statement].columns
						}
					}
				}
				values, _ := meta["Values"].([]map[string]any)

				if len(cols) > 0 {
					if len(cols) != len(values) {
						return nil, fmt.Errorf("postgresql: row/column count mismatch")
					}
					copyCols := make([]map[string]any, len(cols))
					for i, c := range cols {
						copyCols[i] = cloneSession(c)
					}
					if q.kind == "Execute" {
						if o, ok := s.portals[q.name]; ok {
							if len(o.formats) > 1 && len(o.formats) != len(cols) {
								return nil, fmt.Errorf("postgresql: result format count mismatch")
							}
							for i := range copyCols {
								var format uint64
								if len(o.formats) == 1 {
									format = o.formats[0]
								} else if len(o.formats) > 1 {
									format = o.formats[i]
								}
								copyCols[i]["Format Code"] = format
							}
						}
					}
					stream_parser.AnnotatePostgreSQLRow(info, copyCols)
				}

			case "PortalSuspended":
				valid = q.kind == "Execute"
				complete = true
				info["Portal Suspended"] = true
			case "CommandComplete", "EmptyQueryResponse":
				valid = q.kind == "Execute" || q.kind == "Query"
				complete = q.kind == "Execute"
				s.copyMode = ""
				s.columns = nil
			case "ReadyForQuery":
				valid = q.kind == "Sync" || q.kind == "Query"
				complete = true
				s.recovery = false
				s.copyMode = ""
				s.columns = nil
				if info["Transaction Status"] == "I" {
					s.portals = map[string]postgresObject{}
				}
			case "CopyInResponse", "CopyOutResponse", "CopyBothResponse":
				valid = q.kind == "Execute" || q.kind == "Query"
				s.copyMode = map[string]string{"CopyInResponse": "in", "CopyOutResponse": "out", "CopyBothResponse": "both"}[name]
				s.copyBytes = [2]uint64{}
				s.copyDone = [2]bool{}
			case "CopyData", "CopyDone":
				valid = s.copyMode == "out" || s.copyMode == "both"
				if !valid {
					return nil, sessionContext("PostgreSQL backend COPY without COPY OUT/BOTH")
				}
			case "ErrorResponse":
				valid = true
				info["Operation Failed"] = true
				s.copyMode = ""
				s.columns = nil
				if q.kind != "Query" {
					s.recovery = true
					for len(s.queue) > 0 && s.queue[0].kind != "Sync" {
						s.pop()
					}
				}
			}
			info["Session State Validated"] = valid
			if !valid {
				info["Context Level"] = "partial-or-unexpected-phase"
			}
			if complete && valid {
				s.pop()
			}
		}
	}
	if name == "CopyData" || name == "CopyDone" || name == "CopyFail" {
		if s.copyMode == "" {
			return nil, sessionContext("PostgreSQL COPY payload without COPY state")
		}
		if s.copyDone[dir] {
			return nil, fmt.Errorf("postgresql: COPY message after CopyDone")
		}
		if name == "CopyData" {
			s.copyBytes[dir] += uint64(len(e.Raw) - 5)
		} else {
			s.copyDone[dir] = true
		}
		if len(s.queue) > 0 {
			e.TransactionID = s.queue[0].cycle
			e.ResponseTo = s.queue[0].id
		}
	}
	if s.copyMode != "" {
		info["Copy Mode"] = s.copyMode
		info["Copy Bytes"] = s.copyBytes
		info["Copy Done"] = s.copyDone
	}
	if len(s.statements)+len(s.portals) > limit {
		return nil, protocolError(ErrResourceExceeded, "PostgreSQL statement/portal limit")
	}
	info["Outstanding"] = len(s.queue)
	if err = f.reserveSession(s.storage()); err != nil {
		return nil, err
	}
	return info, nil
}
