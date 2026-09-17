package pcaputil

import (
	"bytes"
	"fmt"
	"sort"
)

const binH2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

func (f *binFlow) detectDirection(dir int, w []byte) {
	// Explicit user bindings retain precedence.
	f.detect(w)
	if f.binding != nil {
		return
	}
	if len(w) >= len(binH2Preface) && bytes.HasPrefix(w, []byte(binH2Preface)) {
		f.protocol, f.h2 = "http2", newBinHTTP2(dir)
		return
	}
	// A v10 greeting with a printable, terminated version and sequence zero
	// supplies the server role even on nonstandard ports.
	if len(w) >= 6 && w[3] == 0 && w[4] == 10 {
		end := bytes.IndexByte(w[5:], 0)
		if end > 0 {
			valid := true
			for _, c := range w[5 : 5+end] {
				if c < 32 || c > 126 {
					valid = false
				}
			}
			if valid {
				f.protocol, f.mysql = "mysql", &binMySQL{server: dir, phase: "greeting"}
			}
		}
	}
}

func (f *binFlow) consumeSession(dir int, e *ProtocolEvent, result map[string]any) error {
	var err error
	if f.protocol == "http2" {
		e.Session, err = f.h2.consume(dir, e.Raw)
	}
	if f.protocol == "mysql" {
		e.Session, err = f.mysql.consume(dir, e.Raw, e.Entry, result)
		if err == nil && f.mysql.phase == "tls" {
			// SSLRequest establishes a framing boundary, not authenticated TLS.
			f.protocol = "tls"
		}
	}
	if err == nil && e.Session != nil {
		if e.Protocol == "http2" {
			e.Summary = fmt.Sprintf("HTTP/2 stream %v frame %v", e.Session["Stream ID"], e.Session["Frame Type"])
			if kind, ok := e.Session["Header Kind"].(string); ok {
				e.Summary = fmt.Sprintf("HTTP/2 stream %v %s", e.Session["Stream ID"], kind)
			}
		} else {
			e.Summary = fmt.Sprintf("MySQL transaction %v %v", e.Session["Transaction ID"], e.Session["Phase"])
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
	f.h2 = nil
	f.mysql = nil
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
			return sessionContext("connection state exceeds capture memory budget")
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
