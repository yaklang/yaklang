package pcaputil

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	binparser "github.com/yaklang/yaklang/common/bin-parser/parser"
)

// BinParserEvent owns its bytes and structured values. It never retains a
// pooled TrafficFlow/TrafficConnection. Callbacks may run concurrently for
// different flows; messages in one TCP flow are delivered in order.
type BinParserEvent struct {
	ID, FlowID                               uint64
	Timestamp                                time.Time
	Transport, Source, Destination, Protocol string
	Direction                                int    // 0: first observed endpoint -> peer, 1: reverse (not a role guess)
	Offset                                   uint64 // offset in the delivered, ordered direction, not an absolute TCP sequence
	Length                                   int
	Status, Summary, Rule, Entry, Error      string
	Raw                                      []byte
	Structured                               map[string]any
	plan                                     *binparser.StructuredPlan
	decodeSkip                               int
	decodeConfig                             map[string]any
	historySequence                          uint64
}

// Decode parses a complete event on demand. Incomplete/unrecognized samples
// cannot be decoded as if they were messages. The returned object belongs to
// the caller; no JSON conversion takes place.
func (e *BinParserEvent) Decode() (map[string]any, error) {
	if e.Structured != nil {
		return e.Structured, nil
	}
	if e.Status != "deferred" && e.Status != "decoded" && e.Status != "malformed" {
		return nil, fmt.Errorf("bin-parser: event is %s, not an exact message", e.Status)
	}
	if e.Rule == "" {
		return nil, fmt.Errorf("bin-parser: no exact rule for this message")
	}
	if e.decodeSkip > len(e.Raw) {
		return nil, fmt.Errorf("bin-parser: incomplete transport prefix")
	}
	wire := e.Raw[e.decodeSkip:]
	if e.decodeConfig != nil {
		return binparser.ParseStructuredWithConfig(wire, e.Rule, e.decodeConfig, e.Entry)
	}
	if e.plan != nil {
		return e.plan.Parse(wire)
	}
	if e.Entry == "" {
		return binparser.ParseStructured(wire, e.Rule)
	}
	return binparser.ParseStructured(wire, e.Rule, e.Entry)
}

// BinParserBinding admits an explicit protocol profile. Frame returns the
// complete wire length, 0 for more bytes, or an error. It must validate lengths
// before allocating and must not retain the borrowed input. A port is only a
// routing hint: Probe must confirm the wire. Profiles requiring negotiated
// capabilities/direction belong in a stateful upstream resolver, not a port
// guess. Bindings are tested once per direction, then cached on that flow.
type BinParserBinding struct {
	Protocol, Rule, Entry string
	Port                  uint16 // 0: any port
	Probe                 func([]byte) bool
	Frame                 func([]byte) (int, error)
}

type BinParserConfig struct {
	OnEvent                                       func(*BinParserEvent)
	OnStats                                       func(BinParserStats)
	Deferred                                      bool // explicit capture-first mode; full Decode() is done by the viewer
	MaxMessageBytes, MaxBufferedBytes, ProbeBytes int
	Bindings                                      []BinParserBinding
}

type BinParserStats struct {
	ContextRequired                                                                uint64
	Flows, ProbeCalls, Messages, Decoded, Deferred, Malformed, Incomplete, Unknown uint64
	InputBytes, MessageBytes, UnclassifiedBytes, LimitedBytes, CallbackPanics      uint64
	BufferedBytes, PeakBufferedBytes                                               int64
}

// WithBinParser enables full structured parsing after TCP framing and for
// supported UDP datagrams. It reuses pcapx's flow workers, applies bounded
// buffering, and disables unbounded legacy stream retention. Slow callbacks
// backpressure the capture; callers should use a bounded inspector for viewing.
func WithBinParser(callback func(*BinParserEvent)) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.OnEvent = callback
		return nil
	}
}

func WithBinParserConfig(config BinParserConfig) CaptureOption {
	return func(c *CaptureConfig) error {
		copyConfig := config
		copyConfig.Bindings = append([]BinParserBinding(nil), config.Bindings...)
		c.binParserConfig = &copyConfig
		return nil
	}
}

// These options compose with WithBinParser in either order.
func WithBinParserDeferred(deferred bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.Deferred = deferred
		return nil
	}
}

func WithBinParserStats(callback func(BinParserStats)) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.OnStats = callback
		return nil
	}
}

type binSpec struct {
	rule, entry string
	plan        *binparser.StructuredPlan
}
type binParser struct {
	bindings                                                                   map[uint16][]*BinParserBinding
	contextRequired                                                            atomic.Uint64
	config                                                                     BinParserConfig
	specs                                                                      map[string]*binSpec
	flows, probes, messages, decoded, deferred, malformed, incomplete, unknown atomic.Uint64
	input, messageBytes, unclassified, limited, panics, ids                    atomic.Uint64
	buffered, peak                                                             atomic.Int64
	err                                                                        atomic.Pointer[binParserError]
}
type binParserError struct{ err error }

func (c *CaptureConfig) prepareBinParser() error {
	if c.binParserConfig == nil {
		return nil
	}
	if c.DisableAssembly || c.EnableCache || c.requiresFullStream {
		return fmt.Errorf("bin-parser requires exclusive streaming reassembly; disable capture cache and built-in full-stream HTTP/TLS helpers")
	}
	config := *c.binParserConfig
	if config.OnEvent == nil {
		return fmt.Errorf("bin-parser requires an event callback")
	}
	if config.MaxMessageBytes == 0 {
		config.MaxMessageBytes = 1 << 20
	}
	if config.MaxBufferedBytes == 0 {
		config.MaxBufferedBytes = 32 << 20
	}
	if config.ProbeBytes == 0 {
		config.ProbeBytes = 64
	}
	if config.MaxMessageBytes < 64 || config.MaxMessageBytes > 16<<20 || config.MaxBufferedBytes < config.MaxMessageBytes || config.ProbeBytes < 16 || config.ProbeBytes > config.MaxMessageBytes {
		return fmt.Errorf("invalid bin-parser buffer/probe limits")
	}
	a := &binParser{config: config, specs: make(map[string]*binSpec)}
	a.bindings = make(map[uint16][]*BinParserBinding)
	for _, s := range builtinBinSpecs {
		a.addSpec(s[0], s[1])
	}
	for _, b := range config.Bindings {
		if b.Protocol == "" || b.Rule == "" || b.Probe == nil || b.Frame == nil {
			return fmt.Errorf("bin-parser binding requires protocol, rule, probe and frame")
		}
		a.addSpec(b.Rule, b.Entry)
	}
	for i := range a.config.Bindings {
		b := &a.config.Bindings[i]
		a.bindings[b.Port] = append(a.bindings[b.Port], b)
	}
	c.binParser = a
	c.reassemblyOptions.Stream = true
	return nil
}

func (a *binParser) addSpec(rule, entry string) {
	key := rule + "/" + entry
	if a.specs[key] != nil {
		return
	}
	plan, _ := binparser.PrepareStructured(rule, entry)
	a.specs[key] = &binSpec{rule, entry, plan}
}

func (a *binParser) stats() BinParserStats {
	return BinParserStats{ContextRequired: a.contextRequired.Load(), Flows: a.flows.Load(), ProbeCalls: a.probes.Load(), Messages: a.messages.Load(), Decoded: a.decoded.Load(), Deferred: a.deferred.Load(), Malformed: a.malformed.Load(), Incomplete: a.incomplete.Load(), Unknown: a.unknown.Load(), InputBytes: a.input.Load(), MessageBytes: a.messageBytes.Load(), UnclassifiedBytes: a.unclassified.Load(), LimitedBytes: a.limited.Load(), CallbackPanics: a.panics.Load(), BufferedBytes: a.buffered.Load(), PeakBufferedBytes: a.peak.Load()}
}

func (c *CaptureConfig) finishBinParser() error {
	if c.binParser == nil {
		return nil
	}
	a := c.binParser
	if a.config.OnStats != nil {
		a.config.OnStats(a.stats())
	}
	if e := a.err.Load(); e != nil {
		return e.err
	}
	return nil
}

func (a *binParser) emit(e *BinParserEvent) {
	e.ID = a.ids.Add(1)
	defer func() {
		if p := recover(); p != nil {
			a.panics.Add(1)
			a.err.CompareAndSwap(nil, &binParserError{fmt.Errorf("bin-parser callback panic: %v", p)})
		}
	}()
	a.config.OnEvent(e)
}

type binDirection struct {
	http    *binHTTPState
	buffer  []byte
	offset  uint64
	ts      time.Time
	stopped bool
}
type binFlow struct {
	httpMethods []string
	a           *binParser
	id          uint64
	endpoints   [2]string
	ports       [2]uint16
	protocol    string
	level       byte
	binding     *BinParserBinding
	directions  [2]binDirection
}

func (a *binParser) newFlow(t *TrafficFlow) *binFlow {
	return &binFlow{a: a, id: a.flows.Add(1), endpoints: [2]string{t.ClientConn.LocalAddr().String(), t.ServerConn.LocalAddr().String()}, ports: [2]uint16{uint16(t.ClientConn.LocalPort()), uint16(t.ServerConn.LocalPort())}}
}

func (f *binFlow) event(dir int, raw []byte, status, detail string) *BinParserEvent {
	d := &f.directions[dir]
	return &BinParserEvent{FlowID: f.id, Timestamp: d.ts, Transport: "tcp", Source: f.endpoints[dir], Destination: f.endpoints[1-dir], Direction: dir, Offset: d.offset, Length: len(raw), Protocol: f.protocol, Status: status, Summary: detail, Raw: append([]byte(nil), raw...)}
}

func (f *binFlow) release(d *binDirection) {
	f.a.buffered.Add(-int64(cap(d.buffer)))
	d.buffer = nil
}

// Capacity, not length, is charged to the shared capture budget. Partial
// messages grow geometrically; complete messages bypass this buffer entirely.
func (f *binFlow) retain(d *binDirection, data []byte) bool {
	if len(data) > f.a.config.MaxMessageBytes {
		return false
	}
	if cap(d.buffer) < len(data) {
		capacity := max(len(data), min(2*cap(d.buffer), f.a.config.MaxMessageBytes))
		delta := int64(capacity - cap(d.buffer))
		for {
			current := f.a.buffered.Load()
			if current+delta > int64(f.a.config.MaxBufferedBytes) {
				return false
			}
			if f.a.buffered.CompareAndSwap(current, current+delta) {
				for peak := f.a.peak.Load(); current+delta > peak; peak = f.a.peak.Load() {
					if f.a.peak.CompareAndSwap(peak, current+delta) {
						break
					}
				}
				break
			}
		}
		grown := make([]byte, len(data), capacity)
		copy(grown, data)
		d.buffer = grown
	} else {
		d.buffer = d.buffer[:len(data)]
		copy(d.buffer, data)
	}
	return true
}

func (f *binFlow) stop(dir int, wire []byte, status, reason string) {
	d := &f.directions[dir]
	preview := wire[:min(len(wire), f.a.config.ProbeBytes)]
	e := f.event(dir, preview, status, reason)
	e.Length = len(wire)
	if status == "unrecognized" {
		f.a.unknown.Add(1)
		f.a.unclassified.Add(uint64(len(wire)))
	} else if status == "limited" {
		f.a.limited.Add(uint64(len(wire)))
	} else if status == "context-required" {
		f.a.contextRequired.Add(1)
		f.a.unclassified.Add(uint64(len(wire)))
	} else {
		f.a.malformed.Add(1)
	}
	d.offset += uint64(len(wire))
	d.stopped = true
	f.release(d)
	f.a.emit(e)
}

func (f *binFlow) feed(dir int, data []byte, ts time.Time) {
	a, d := f.a, &f.directions[dir]
	defer func() {
		if p := recover(); p != nil {
			a.err.CompareAndSwap(nil, &binParserError{fmt.Errorf("bin-parser flow panic: %v", p)})
			a.malformed.Add(1)
			d.stopped = true
			f.release(d)
		}
	}()
	a.input.Add(uint64(len(data)))
	if d.stopped {
		a.unclassified.Add(uint64(len(data)))
		d.offset += uint64(len(data))
		return
	}
	if len(d.buffer) == 0 {
		d.ts = ts
	}
	wire := data
	detected := false
	if len(d.buffer) != 0 {
		old := len(d.buffer)
		if old+len(data) > a.config.MaxMessageBytes {
			room := a.config.MaxMessageBytes - old
			if room == 0 {
				f.stop(dir, d.buffer, "limited", "message buffer limit reached")
				a.limited.Add(uint64(len(data)))
				return
			}
			// Split an arrival which finishes one large message and starts another.
			a.input.Add(^uint64(len(data) - 1))
			f.feed(dir, data[:room], ts)
			f.feed(dir, data[room:], ts)
			return
		}
		// Reserve without a temporary concatenation allocation.
		if cap(d.buffer) < old+len(data) {
			capacity := max(old+len(data), min(cap(d.buffer)*2, a.config.MaxMessageBytes))
			delta := int64(capacity - cap(d.buffer))
			current := a.buffered.Add(delta)
			if current > int64(a.config.MaxBufferedBytes) {
				a.buffered.Add(-delta)
				f.stop(dir, d.buffer, "limited", "capture buffer limit reached")
				a.limited.Add(uint64(len(data)))
				return
			}
			for peak := a.peak.Load(); current > peak; peak = a.peak.Load() {
				if a.peak.CompareAndSwap(peak, current) {
					break
				}
			}
			grown := make([]byte, old, capacity)
			copy(grown, d.buffer)
			d.buffer = grown
		}
		d.buffer = append(d.buffer, data...)
		wire = d.buffer
	}
	for len(wire) > 0 {
		if f.protocol == "" {
			a.probes.Add(1)
			f.detect(wire[:min(len(wire), a.config.ProbeBytes)])
			detected = f.protocol != ""
			if f.protocol == "" {
				if len(wire) >= a.config.ProbeBytes {
					f.stop(dir, wire, "unrecognized", "bounded detection exhausted; subsequent bytes are counted without VM retries")
					return
				}
				break
			}
		}
		n, spec, err := f.frameDirection(dir, wire)
		if err != nil {
			status := "malformed"
			if errors.Is(err, errBinContext) {
				status = "context-required"
			}
			f.stop(dir, wire, status, err.Error())
			return
		}
		if n < 0 || n > a.config.MaxMessageBytes {
			f.stop(dir, wire, "limited", "declared message exceeds limit")
			return
		}
		if n == 0 || n > len(wire) {
			break
		}
		if spec == nil {
			f.stop(dir, wire, "context-required", "protocol recognized, but no supported exact profile for this phase")
			return
		}
		e := f.event(dir, wire[:n], "deferred", f.protocol)
		e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
		if f.protocol == "dns" {
			e.decodeSkip = 2
		}
		if f.protocol == "http" {
			e.decodeConfig = d.http.config()
			e.Summary = d.http.summary
			f.finishHTTP(dir)
		}
		a.messages.Add(1)
		a.messageBytes.Add(uint64(n))
		if !a.config.Deferred {
			result, err := e.Decode()
			if err != nil {
				e.Status, e.Error = "malformed", err.Error()
				a.malformed.Add(1)
			} else {
				e.Status, e.Structured = "decoded", result
				a.decoded.Add(1)
			}
		} else {
			a.deferred.Add(1)
		}
		d.offset += uint64(n)
		wire = wire[n:]
		failed := e.Error != ""
		a.emit(e)
		if failed {
			if len(wire) > 0 {
				f.stop(dir, wire, "context-required", "decode failure invalidated stream affinity")
			} else {
				d.stopped = true
				f.release(d)
			}
			return
		}
		d.ts = ts
	}
	if len(wire) == 0 {
		f.release(d)
	} else if !f.retain(d, wire) {
		f.stop(dir, wire, "limited", "capture buffer limit reached")
	}
	if detected && f.protocol != "" {
		other := &f.directions[1-dir]
		if len(other.buffer) > 0 && !other.stopped {
			f.feed(1-dir, nil, other.ts)
		}
	}
}

func (f *binFlow) close(reason TrafficFlowCloseReason) {
	for dir := range f.directions {
		d := &f.directions[dir]
		if len(d.buffer) != 0 {
			if h := d.http; h != nil && h.closeDelimited && reason == TrafficFlowCloseReason_FIN {
				e := f.event(dir, d.buffer, "deferred", h.summary)
				e.Rule, e.Entry, e.decodeConfig = "application-layer.http", "HTTPExact", h.config()
				f.a.messages.Add(1)
				f.a.messageBytes.Add(uint64(len(d.buffer)))
				if f.a.config.Deferred {
					f.a.deferred.Add(1)
				} else if result, err := e.Decode(); err != nil {
					e.Status, e.Error = "malformed", err.Error()
					f.a.malformed.Add(1)
				} else {
					e.Status, e.Structured = "decoded", result
					f.a.decoded.Add(1)
				}
				f.release(d)
				f.finishHTTP(dir)
				f.a.emit(e)
				continue
			}
			e := f.event(dir, d.buffer, "incomplete", string(reason)+": exact message boundary was not reached")
			f.a.incomplete.Add(1)
			f.release(d)
			f.a.emit(e)
		} else {
			f.release(d)
		}
	}
}

// BinParserInspector is a bounded message history for CLI/Yak viewing. It
// retains raw messages and routing metadata, not eager field trees. Details
// decode outside the capture lock. Eviction is observable, never capture loss.
type BinParserInspector struct {
	mu                           sync.Mutex
	rows                         []*BinParserEvent
	head, count, bytes, maxBytes int
	evicted                      uint64
	received                     uint64
}

func NewBinParserInspector(messages, bytes int) (*BinParserInspector, error) {
	if messages <= 0 || messages > 1000000 || bytes <= 0 {
		return nil, errors.New("inspector requires positive bounded message/byte limits")
	}
	return &BinParserInspector{rows: make([]*BinParserEvent, messages), maxBytes: bytes}, nil
}

func (v *BinParserInspector) OnEvent(e *BinParserEvent) {
	row := *e
	row.Structured = nil
	row.Raw = append([]byte(nil), e.Raw...)
	v.mu.Lock()
	defer v.mu.Unlock()
	v.received++
	row.historySequence = v.received
	if len(row.Raw) > v.maxBytes {
		v.evicted++
		return
	}
	for v.count > 0 && (v.count == len(v.rows) || v.bytes+len(row.Raw) > v.maxBytes) {
		v.bytes -= len(v.rows[v.head].Raw)
		v.rows[v.head] = nil
		v.head = (v.head + 1) % len(v.rows)
		v.count--
		v.evicted++
	}
	v.rows[(v.head+v.count)%len(v.rows)] = &row
	v.count++
	v.bytes += len(row.Raw)
}

// Rows returns metadata only, oldest first, optionally filtered by protocol and
// flow. Details(id) returns owned bytes plus the complete structured result.
func (v *BinParserInspector) Rows(protocol string, flow uint64) []*BinParserEvent {
	v.mu.Lock()
	defer v.mu.Unlock()
	var rows []*BinParserEvent
	for i := 0; i < v.count; i++ {
		e := v.rows[(v.head+i)%len(v.rows)]
		if (protocol == "" || protocol == e.Protocol) && (flow == 0 || flow == e.FlowID) {
			row := *e
			row.Raw = nil
			rows = append(rows, &row)
		}
	}
	return rows
}

func (v *BinParserInspector) Details(id uint64) (*BinParserEvent, error) {
	v.mu.Lock()
	var row *BinParserEvent
	for i := 0; i < v.count; i++ {
		e := v.rows[(v.head+i)%len(v.rows)]
		if e.ID == id {
			copyEvent := *e
			copyEvent.Raw = append([]byte(nil), e.Raw...)
			row = &copyEvent
			break
		}
	}
	v.mu.Unlock()
	if row == nil {
		return nil, fmt.Errorf("message %d is absent or evicted", id)
	}
	if row.Rule != "" {
		result, err := row.Decode()
		if err != nil {
			return row, err
		}
		row.Structured = result
		row.Status = "decoded"
	}
	return row, nil
}

func (v *BinParserInspector) Evicted() uint64 { v.mu.Lock(); defer v.mu.Unlock(); return v.evicted }

// RowsAfter returns at most limit newest matching arrivals after a viewer
// cursor. The cursor follows arrival order, not concurrently assigned event IDs.
// Omitted counts evicted history plus rows exceeding the presentation limit,
// never lost analysis. Filters only affect presentation. Returned rows own their
// metadata and omit Raw/Structured, just like Rows.
func (v *BinParserInspector) RowsAfter(after uint64, limit int, protocol string, flow uint64) (rows []*BinParserEvent, cursor, omitted uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	cursor = v.received
	if after >= cursor || limit <= 0 {
		return
	}
	var retained, matching uint64
	for i := v.count - 1; i >= 0; i-- {
		e := v.rows[(v.head+i)%len(v.rows)]
		if e.historySequence <= after {
			break
		}
		retained++
		if (protocol == "" || protocol == e.Protocol) && (flow == 0 || flow == e.FlowID) {
			matching++
			if len(rows) < limit {
				row := *e
				row.Raw, row.Structured = nil, nil
				rows = append(rows, &row)
			}
		}
	}
	omitted = cursor - after - retained + matching - uint64(len(rows))
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return
}
