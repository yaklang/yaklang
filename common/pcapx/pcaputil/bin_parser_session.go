package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

const binH2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

func (f *binFlow) detectDirection(dir int, w []byte) {
	// Explicit user bindings retain precedence.
	f.detect(w)
	if f.binding != nil || f.protocol != "" {
		return
	}
	p := probeWire(w, f.a.config.ProbeBytes)
	if p.Verdict != ProbeAccept {
		return
	}
	switch p.Protocol {
	case "http2":
		f.protocol, f.h2 = "http2", newBinHTTP2(dir)
	case "mysql":
		f.protocol, f.mysql = "mysql", &binMySQL{server: dir, phase: "greeting"}
	case "postgresql":
		f.protocol, f.pg = "postgresql", &binPostgres{frontend: -1}
	case "ldap":
		f.protocol, f.ldap = "ldap", &binLDAP{pending: map[uint64]ldapRequest{}}
	case "redis":
		f.protocol, f.redis = "redis", &binRedis{}
	case "websocket":
		client := dir
		if w[1]&128 == 0 {
			client = 1 - dir
		}
		f.protocol, f.ws = "websocket", &binWebSocket{client: client, phase: wsPhaseFromProbe(w)}
	}
}

func (f *binFlow) consumeSession(dir int, e *ProtocolEvent, result map[string]any) error {
	var err error
	switch f.protocol {
	case "http2":
		var previous *binH2Stream
		if len(e.Raw) >= 9 && !bytes.HasPrefix(e.Raw, []byte(binH2Preface)) {
			previous = f.h2.streams[binary.BigEndian.Uint32(e.Raw[5:9])&0x7fffffff]
		}
		e.Session, err = f.h2.consume(dir, e.Raw)
		if err == nil {
			err = f.consumeGRPC(dir, e, previous)
		}
	case "mysql":
		e.Session, err = f.mysql.consume(dir, e.Raw, e.Entry, result)
		if err == nil && f.mysql.phase == "tls" {
			f.protocol = "tls"
		}
	case "postgresql":
		e.Session, err = f.pg.consume(dir, e.Raw, e.Entry)
		if err == nil && e.Session["Encrypted"] == true {
			f.protocol, f.pg = "tls", nil
			e.Session["Protocol Transition"] = "postgresql->tls"
		}
	case "ldap":
		e.Session, err = f.ldap.consume(dir, e.Raw, e.Entry, f.a.budget.MaxCollectionElements)
		if err == nil && e.Session["StartTLS"] == true && e.Session["Message Name"] == "ExtendedResponse" {
			f.protocol, f.ldap = "tls", nil
			e.Session["Protocol Transition"] = "ldap->tls"
		}
	case "redis":
		e.Session, err = f.redis.consume(dir, e.Raw)
	case "websocket":
		if e.Entry == "WebSocket" {
			e.Session, err = f.ws.consume(dir, e.Raw)
		} else {
			e.Session = map[string]any{
				"Protocol Transition": "http->websocket",
				"Reason":              "101 Switching Protocols",
			}
		}
	}
	if err == nil && e.Session != nil {
		switch e.Protocol {
		case "http2":
			e.Summary = fmt.Sprintf("HTTP/2 stream %v frame %v", e.Session["Stream ID"], e.Session["Frame Type"])
			if kind, ok := e.Session["Header Kind"].(string); ok {
				e.Summary = fmt.Sprintf("HTTP/2 stream %v %s", e.Session["Stream ID"], kind)
			}
			if e.Session["GRPC"] == true {
				e.Protocol = "grpc"
				e.Summary = fmt.Sprintf("gRPC stream %v", e.Session["Stream ID"])
			}
		case "mysql":
			e.Summary = fmt.Sprintf("MySQL transaction %v %v", e.Session["Transaction ID"], e.Session["Phase"])
		case "postgresql":
			e.Summary = fmt.Sprintf("PostgreSQL %v", e.Session["Message Name"])
		case "ldap":
			e.Summary = fmt.Sprintf("LDAP %v id %v", e.Session["Message Name"], e.Session["Message ID"])
		case "redis":
			e.Summary = fmt.Sprintf("Redis %v", e.Session["RESP Type"])
		case "websocket":
			e.Summary = fmt.Sprintf("WebSocket %v", e.Session["Opcode Name"])
		}
	}
	return err
}

func (f *binFlow) invalidateSession(dir int) {
	d := &f.directions[dir]
	if d.stopped {
		return
	}
	if len(d.buffer) > 0 {
		f.stop(dir, d.buffer, "context-required", "peer failure invalidated connection state")
	} else {
		d.stopped = true
		f.release(d)
	}
}

func (f *binFlow) closeSession() {
	f.h2, f.mysql, f.pg, f.ws, f.ldap, f.redis = nil, nil, nil, nil, nil, nil
	f.a.buffered.Add(-f.sessionBytes)
	f.sessionBytes = 0
}

func (f *binFlow) finishSession(reason TrafficFlowCloseReason) {
	emit := func(dir int, session map[string]any, why string) {
		e := f.event(dir, nil, "incomplete", string(reason)+": "+why)
		e.Session = session
		f.a.incomplete.Add(1)
		f.a.emit(e)
	}
	if h := f.h2; h != nil && f.protocol == "http2" {
		ids := make([]int, 0, len(h.streams))
		for id := range h.streams {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		for _, id := range ids {
			emit(h.client, map[string]any{"Stream ID": uint32(id)}, "HTTP/2 exchange did not reach END_STREAM in both directions")
		}
	}
	if m := f.mysql; m != nil && f.protocol == "mysql" && m.phase != "command" && m.phase != "closed" {
		emit(m.server, map[string]any{"Phase": m.phase, "Transaction ID": m.transaction}, "MySQL exchange ended before its expected response")
	}
	if ws := f.ws; ws != nil {
		for dir, op := range ws.opcode {
			if op != 0 {
				emit(dir, map[string]any{"Opcode": uint64(op)}, "WebSocket message ended before its final continuation")
			}
		}
	}
	if p := f.pg; p != nil && p.pending > 0 {
		emit(max(p.frontend, 0), map[string]any{"Outstanding": p.pending}, "PostgreSQL exchange ended with unmatched extended-query messages")
	}
	if l := f.ldap; l != nil && len(l.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(l.pending)}, "LDAP exchange ended with unmatched MessageIDs")
	}
}

// Conservative capacity accounting includes dictionaries and map/queue slots.
// Reservations are released on close; repeated streams reuse the high-water budget.
func (f *binFlow) reserveSession(target int64) error {
	if target <= f.sessionBytes {
		return nil
	}
	delta := target - f.sessionBytes
	for {
		current := f.a.buffered.Load()
		if current+delta > int64(f.a.config.MaxBufferedBytes) {
			return protocolError(ErrResourceExceeded, "connection state exceeds capture memory budget")
		}
		if f.a.buffered.CompareAndSwap(current, current+delta) {
			f.sessionBytes = target
			for peak := f.a.peak.Load(); current+delta > peak; peak = f.a.peak.Load() {
				if f.a.peak.CompareAndSwap(peak, current+delta) {
					break
				}
			}
			return nil
		}
	}
}

func sessionContext(why string) error { return fmt.Errorf("%w: %s", errBinContext, why) }

// Context snapshots contain only these value types. No live decoder, pooled
// connection or mutable dictionary is retained by an event or inspector.
func cloneSessionValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = cloneSessionValue(v)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(x))
		for i, v := range x {
			out[i] = cloneSessionValue(v).(map[string]any)
		}
		return out
	case []byte:
		return bytes.Clone(x)
	default:
		return v
	}
}

func cloneSession(v map[string]any) map[string]any {
	if v == nil {
		return nil
	}
	return cloneSessionValue(v).(map[string]any)
}

func sessionSnapshotBytes(v any) int { return sessionSnapshotSize(v, true) }

// History budgets count logical owned byte lengths, not spare input capacity.
// Both forms remain approximate budgets rather than exact Go heap sizes.
func sessionSnapshotSize(v any, byteCapacity bool) int {
	switch x := v.(type) {
	case map[string]any:
		if x == nil {
			return 0
		}
		n := 64
		for k, v := range x {
			n += len(k) + 32 + sessionSnapshotSize(v, byteCapacity)
		}
		return n
	case []map[string]any:
		n := 24 + len(x)*8
		for _, v := range x {
			n += sessionSnapshotSize(v, byteCapacity)
		}
		return n
	case string:
		return len(x) + 16
	case []byte:
		if byteCapacity {
			return cap(x) + 24
		}
		return len(x) + 24
	case nil:
		return 0
	default:
		return 16
	}
}

func protocolHistoryBytes(e *ProtocolEvent) int {
	return len(e.Raw) + sessionSnapshotSize(e.Session, false)
}
