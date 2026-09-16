package pcaputil

import (
	"bytes"
	"encoding/binary"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2/hpack"
)

func h2TestHeaders(t testing.TB, fields ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	e := hpack.NewEncoder(&b)
	for i := 0; i < len(fields); i += 2 {
		require.NoError(t, e.WriteField(hpack.HeaderField{Name: fields[i], Value: fields[i+1]}))
	}
	return b.Bytes()
}

func TestLiveHTTP2RejectsInvalidState(t *testing.T) {
	fixture := h2TestFixtures(t)
	req := func(id uint32, flags byte, fields ...string) sessionStep {
		return sessionStep{0, h2TestFrame(1, flags, id, h2TestHeaders(t, fields...))}
	}
	valid := []string{":method", "GET", ":scheme", "http", ":path", "/"}
	tests := []struct {
		name   string
		prefix int
		steps  []sessionStep
		status string
	}{
		{"initial-not-settings", 0, []sessionStep{{0, append([]byte(binH2Preface), h2TestFrame(6, 0, 0, make([]byte, 8))...)}}, "malformed"},
		{"invalid-settings", 4, []sessionStep{{1, h2TestFrame(4, 0, 0, []byte{0, 5, 0, 0, 0, 1})}}, "malformed"},
		{"server-enable-push", 4, []sessionStep{{1, h2TestFrame(4, 0, 0, []byte{0, 2, 0, 0, 0, 0})}}, "malformed"},
		{"unsolicited-ack", 4, []sessionStep{{0, h2TestFrame(4, 1, 0, nil)}}, "malformed"},
		{"orphan-continuation", 4, []sessionStep{{0, h2TestFrame(9, 4, 1, []byte{0x82})}}, "malformed"},
		{"wrong-continuation-stream", 4, []sessionStep{{0, append(h2TestFrame(1, 1, 1, []byte{0x82}), h2TestFrame(9, 4, 3, []byte{0x84})...)}}, "malformed"},
		{"interleaved-continuation", 4, []sessionStep{{0, append(h2TestFrame(1, 0, 1, []byte{0x82}), h2TestFrame(6, 0, 0, make([]byte, 8))...)}}, "malformed"},
		{"missing-dictionary", 4, []sessionStep{{0, h2TestFrame(1, 5, 1, []byte{0xbe})}}, "malformed"},
		{"uppercase-header", 4, []sessionStep{req(1, 5, append(valid, "X-Upper", "value")...)}, "malformed"},
		{"duplicate-pseudo", 4, []sessionStep{req(1, 5, append(valid, ":method", "POST")...)}, "malformed"},
		{"late-pseudo", 4, []sessionStep{req(1, 5, "x", "y", ":method", "GET", ":path", "/", ":scheme", "http")}, "malformed"},
		{"missing-method", 4, []sessionStep{req(1, 5, ":path", "/", ":scheme", "http")}, "malformed"},
		{"connection-header", 4, []sessionStep{req(1, 5, append(valid, "connection", "close")...)}, "malformed"},
		{"data-idle", 4, []sessionStep{{0, h2TestFrame(0, 1, 1, []byte("x"))}}, "context-required"},
		{"data-half-closed", 5, []sessionStep{{0, h2TestFrame(0, 1, 1, []byte("x"))}}, "malformed"},
		{"response-without-request", 4, []sessionStep{{1, h2TestFrame(1, 5, 1, h2TestHeaders(t, ":status", "200"))}}, "context-required"},
		{"zero-window-increment", 4, []sessionStep{{0, h2TestFrame(8, 0, 0, make([]byte, 4))}}, "malformed"},
		{"connection-window-overflow", 4, []sessionStep{{0, h2TestFrame(8, 0, 0, []byte{0x7f, 0xff, 0xff, 0xff})}}, "malformed"},
		{"new-stream-after-goaway", 12, []sessionStep{req(5, 5, valid...)}, "context-required"},
		{"reused-stream", 11, []sessionStep{req(1, 5, valid...)}, "malformed"},
		{"frame-too-large", 4, []sessionStep{{0, []byte{0, 0x40, 1, 0, 0, 0, 0, 0, 1}}}, "malformed"},
		{"hpack-table-local-limit", 4, []sessionStep{{1, h2TestFrame(4, 0, 0, []byte{0, 1, 0, 2, 0, 0})}}, "context-required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, chunk := range []int{0, 1, 7} {
				steps := append(append([]sessionStep{}, fixture[:tt.prefix]...), tt.steps...)
				events, _ := sessionTestFlow(t, "http2", steps, chunk, false)
				require.NotEmpty(t, events)
				last := events[len(events)-1]
				require.Equal(t, tt.status, last.Status, "chunk=%d %s %s", chunk, last.Error, last.Summary)
			}
		})
	}
}

func TestLiveHTTP2SettingsHPACKAndTrailers(t *testing.T) {
	base := h2TestFixtures(t)[:4]
	// The server reduces the table available to the client, which acknowledges
	// before emitting a zero-size update. The server's own dictionary is intact.
	var b bytes.Buffer
	e := hpack.NewEncoder(&b)
	e.SetMaxDynamicTableSize(0)
	for _, h := range []hpack.HeaderField{{Name: ":method", Value: "POST"}, {Name: ":scheme", Value: "http"}, {Name: ":path", Value: "/"}} {
		require.NoError(t, e.WriteField(h))
	}
	steps := append(append([]sessionStep{}, base...), sessionStep{1, h2TestFrame(4, 0, 0, []byte{0, 1, 0, 0, 0, 0})}, sessionStep{0, h2TestFrame(4, 1, 0, nil)}, sessionStep{0, h2TestFrame(1, 4, 1, b.Bytes())}, sessionStep{0, h2TestFrame(0, 0, 1, []byte("body"))}, sessionStep{0, h2TestFrame(1, 5, 1, h2TestHeaders(t, "x-trailer", "yes"))}, sessionStep{1, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":status", "103", "link", "</style.css>"))}, sessionStep{1, h2TestFrame(1, 5, 1, h2TestHeaders(t, ":status", "200"))})
	events, _ := sessionTestFlow(t, "http2", steps, 1, true)
	assertSessionEvents(t, events, "http2", true)
	require.Equal(t, "trailers", events[8].Session["Header Kind"])
	require.Equal(t, "informational", events[9].Session["Header Kind"])
	require.Equal(t, true, events[10].Session["Exchange Complete"])
	bad := append([]sessionStep{}, steps[:6]...)
	bad = append(bad, sessionStep{0, h2TestFrame(1, 5, 1, h2TestHeaders(t, ":method", "GET", ":scheme", "http", ":path", "/"))})
	events, _ = sessionTestFlow(t, "http2", bad, 0, false)
	require.Equal(t, "malformed", events[len(events)-1].Status)
	require.Contains(t, events[len(events)-1].Error, "table reduction")
}

func TestLiveMySQLRejectsInvalidState(t *testing.T) {
	fixture := mysqlTestFixtures(false)
	badResult := mysqlTestResult(1, false, 2)
	badResult[8] = 0 // destroy a column length after a valid result header
	tests := []struct {
		name   string
		prefix int
		steps  []sessionStep
		status string
	}{
		{"wrong-handshake-sequence", 1, []sessionStep{{0, mysqlTestPacket(2, fixture[1].wire[4:])}}, "malformed"},
		{"wrong-auth-direction", 2, []sessionStep{{0, mysqlTestPacket(2, []byte{0})}}, "malformed"},
		{"unexpected-command", 2, []sessionStep{{0, mysqlTestPacket(0, []byte{3, 'x'})}}, "malformed"},
		{"response-without-command", 6, []sessionStep{{1, mysqlTestPacket(1, []byte{0, 0, 0, 2, 0, 0, 0})}}, "context-required"},
		{"command-sequence", 6, []sessionStep{{0, mysqlTestPacket(1, []byte{3, 'x'})}}, "malformed"},
		{"prepared-statement", 6, []sessionStep{{0, mysqlTestPacket(0, []byte{0x16, 'x'})}}, "context-required"},
		{"local-infile", 7, []sessionStep{{1, mysqlTestPacket(1, []byte{0xfb, 'x'})}}, "context-required"},
		{"pipelined-command", 7, []sessionStep{{0, mysqlTestPacket(0, []byte{14})}}, "malformed"},
		{"invalid-column-count", 7, []sessionStep{{1, mysqlTestPacket(1, []byte{0xfc, 1, 0x10})}}, "malformed"},
		{"invalid-column", 7, []sessionStep{{1, badResult}}, "malformed"},
		{"wrong-result-sequence", 7, []sessionStep{{1, mysqlTestResult(2, false, 2)}}, "malformed"},
		{"empty-command", 6, []sessionStep{{0, mysqlTestPacket(0, nil)}}, "malformed"},
		{"after-quit", 13, []sessionStep{{0, mysqlTestPacket(0, []byte{3, 'x'})}}, "context-required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, chunk := range []int{0, 1, 7} {
				steps := append(append([]sessionStep{}, fixture[:tt.prefix]...), tt.steps...)
				events, _ := sessionTestFlow(t, "mysql", steps, chunk, false)
				require.NotEmpty(t, events)
				last := events[len(events)-1]
				require.Equal(t, tt.status, last.Status, "chunk=%d %s %s", chunk, last.Error, last.Summary)
			}
		})
	}
}

func TestLiveMySQLTLSCompressionAndFailedAuthentication(t *testing.T) {
	for _, mode := range []string{"tls", "compression", "auth-error", "query-attributes"} {
		t.Run(mode, func(t *testing.T) {
			caps := mysqlTestCaps
			if mode == "tls" {
				caps |= 1 << 11
			}
			if mode == "compression" {
				caps |= 1 << 5
			}
			if mode == "query-attributes" {
				caps |= 1 << 27
			}
			g, r := mysqlTestHandshake(caps)
			steps := []sessionStep{{1, g}, {0, r}}
			switch mode {
			case "tls":
				steps[1].wire = mysqlTestPacket(1, r[4:36])
				steps = append(steps, sessionStep{0, []byte{23, 3, 3, 0, 3, 1, 2, 3}})
			case "auth-error":
				steps = append(steps, sessionStep{1, mysqlTestPacket(2, []byte("\xff\x15\x04#28000denied"))})
			case "query-attributes":
				steps = append(steps, sessionStep{1, mysqlTestPacket(2, []byte{0, 0, 0, 2, 0, 0, 0})}, sessionStep{0, mysqlTestPacket(0, []byte{3, 'x'})})
			}
			events, _ := sessionTestFlow(t, "mysql", steps, 1, false)
			last := events[len(events)-1]
			if mode == "compression" || mode == "query-attributes" {
				require.Equal(t, "context-required", last.Status)
			} else {
				require.Equal(t, "decoded", last.Status, last.Error)
			}
			if mode == "tls" {
				require.Equal(t, "tls", last.Protocol)
				require.Equal(t, true, events[1].Session["TLS Requested"])
			}
			if mode == "auth-error" {
				require.Equal(t, "closed", last.Session["Next Phase"])
				require.NotContains(t, last.Session, "Authentication OK Observed")
			}
		})
	}
}

func TestLiveProtocolTruncationAndOwnership(t *testing.T) {
	for _, protocol := range []string{"http2", "mysql"} {
		t.Run(protocol, func(t *testing.T) {
			steps := h2TestFixtures(t)
			index := 4
			if protocol == "mysql" {
				steps = mysqlTestFixtures(false)
				index = 7
			}
			// Every strict prefix of a multi-frame/block or result set is incomplete.
			for n := 1; n < len(steps[index].wire); n++ {
				selected := append([]sessionStep{}, steps[:index]...)
				selected = append(selected, sessionStep{steps[index].dir, steps[index].wire[:n]})
				events, _ := sessionTestFlow(t, protocol, selected, 0, true)
				last := events[len(events)-1]
				// MySQL fixture contains two complete results, so an exact first-result
				// boundary legitimately ends on a deferred event awaiting another result.
				if protocol == "mysql" && n == len(mysqlTestResult(1, false, 10)) {
					require.Equal(t, "incomplete", last.Status)
				} else {
					require.Equal(t, "incomplete", last.Status, "prefix=%d %s", n, last.Error)
				}
			}
			events, _ := sessionTestFlow(t, protocol, steps, 0, true)
			v, err := NewBinParserInspector(100, 1<<20)
			require.NoError(t, err)
			for i, e := range events {
				e.ID = uint64(i + 1)
				v.OnEvent(e)
			}
			id := uint64(5)
			if protocol == "mysql" {
				id = 8
			}
			detail, err := v.Details(id)
			require.NoError(t, err)
			detail.Session["Phase"] = "tampered"
			if hs, ok := detail.Session["Headers"].([]map[string]any); ok && len(hs) > 0 {
				hs[0]["Value"] = "tampered"
			}
			again, err := v.Details(id)
			require.NoError(t, err)
			require.NotEqual(t, "tampered", again.Session["Phase"])
			if hs, ok := again.Session["Headers"].([]map[string]any); ok {
				require.NotEqual(t, "tampered", hs[0]["Value"])
			}
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 10; j++ {
						_, e := v.Details(id)
						require.NoError(t, e)
					}
				}()
			}
			wg.Wait()
			for _, row := range v.Rows("", 0) {
				require.Nil(t, row.Session)
			}
		})
	}
	// A tiny compressed frame can expand to large context; history charges it.
	v, err := NewBinParserInspector(10, 100)
	require.NoError(t, err)
	v.OnEvent(&ProtocolEvent{ID: 1, Raw: []byte{1}, Session: map[string]any{"Header": strings.Repeat("x", 200)}})
	require.EqualValues(t, 1, v.Evicted())
}

func TestLiveProtocolMemoryBudgetAndIsolation(t *testing.T) {
	var events []*ProtocolEvent
	c := NewDefaultConfig()
	require.NoError(t, WithBinParserConfig(BinParserConfig{MaxMessageBytes: 64, MaxBufferedBytes: 64, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }})(c))
	require.NoError(t, c.prepareBinParser())
	f := &binFlow{a: c.binParser}
	f.feed(0, h2TestFixtures(t)[0].wire, time.Now())
	f.close(TrafficFlowCloseReason_FIN)
	require.Zero(t, c.binParser.stats().BufferedBytes)
	require.Equal(t, "context-required", events[0].Status)
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, p := range []string{"http2", "mysql"} {
				steps := h2TestFixtures(t)
				if p == "mysql" {
					steps = mysqlTestFixtures(false)
				}
				es, _ := sessionTestFlow(t, p, steps, 3, true)
				assertSessionEvents(t, es, p, true)
			}
		}()
	}
	wg.Wait()
}

func TestLiveProtocolSharedParserIsolation(t *testing.T) {
	var mu sync.Mutex
	events := make(map[uint64][]*ProtocolEvent)
	c := NewDefaultConfig()
	require.NoError(t, WithBinParserConfig(BinParserConfig{OnEvent: func(e *ProtocolEvent) {
		mu.Lock()
		events[e.FlowID] = append(events[e.FlowID], e)
		mu.Unlock()
	}})(c))
	require.NoError(t, c.prepareBinParser())
	fixtures := [][]sessionStep{h2TestFixtures(t), mysqlTestFixtures(false)}
	var wg sync.WaitGroup
	for id := uint64(1); id <= 16; id++ {
		wg.Add(1)
		go func(id uint64) {
			defer wg.Done()
			f := &binFlow{a: c.binParser, id: id}
			for _, step := range fixtures[id%2] {
				for _, b := range step.wire {
					f.feed(step.dir, []byte{b}, time.Unix(1, 0))
				}
			}
			f.close(TrafficFlowCloseReason_FIN)
		}(id)
	}
	wg.Wait()
	for id := uint64(1); id <= 16; id++ {
		protocol, count := "http2", 12
		if id%2 == 1 {
			protocol, count = "mysql", 14
		}
		require.Len(t, events[id], count)
		assertSessionEvents(t, events[id], protocol, false)
	}
	require.Zero(t, c.binParser.stats().BufferedBytes)
	require.Nil(t, c.binParser.err.Load())
}

func TestLiveProtocolBufferExhaustionInvalidatesPeer(t *testing.T) {
	for _, grow := range []bool{false, true} {
		c := NewDefaultConfig()
		require.NoError(t, WithBinParserConfig(BinParserConfig{OnEvent: func(*ProtocolEvent) {}})(c))
		require.NoError(t, c.prepareBinParser())
		f := &binFlow{a: c.binParser}
		for _, step := range h2TestFixtures(t)[:4] {
			f.feed(step.dir, step.wire, time.Unix(1, 0))
		}
		wire := h2TestFrame(1, 4, 1, bytes.Repeat([]byte{0x82}, 900))
		if grow {
			f.feed(0, wire[:20], time.Unix(1, 0))
		}
		c.binParser.config.MaxBufferedBytes = int(c.binParser.stats().BufferedBytes)
		if grow {
			f.feed(0, wire[20:100], time.Unix(1, 0))
		} else {
			f.feed(0, wire[:20], time.Unix(1, 0))
		}
		require.True(t, f.directions[0].stopped)
		require.True(t, f.directions[1].stopped)
		require.Nil(t, f.h2)
		require.Zero(t, c.binParser.stats().BufferedBytes)
		f.close(TrafficFlowCloseReason_FIN)
	}
}

func FuzzLiveProtocolSessions(f *testing.F) {
	f.Add(byte(0), []byte{0, 0, 0, 4, 0, 0, 0, 0, 0})
	f.Add(byte(1), mysqlTestPacket(1, []byte{1}))
	f.Add(byte(0), []byte{0xff, 0xff, 0xff, 1, 4, 0, 0, 0, 1})
	f.Fuzz(func(t *testing.T, which byte, data []byte) {
		if len(data) > 65536 {
			return
		}
		steps := h2TestFixtures(t)[:4]
		p := "http2"
		dir := 0
		if which&1 != 0 {
			steps = mysqlTestFixtures(false)[:7]
			p = "mysql"
			dir = 1
		}
		steps = append(append([]sessionStep{}, steps...), sessionStep{dir, data})
		_, flow := sessionTestFlow(t, p, steps, 7, false)
		require.Zero(t, flow.a.stats().BufferedBytes)
	})
}

func BenchmarkLiveProtocolSessions(b *testing.B) {
	for _, p := range []string{"http2", "mysql"} {
		b.Run(p, func(b *testing.B) {
			steps := h2TestFixtures(b)
			if p == "mysql" {
				steps = mysqlTestFixtures(false)
			}
			wire := sessionTestPCAP(b, steps, 3306, 0, false, false)
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				events, _, err := binReplay(b, wire, 1)
				require.NoError(b, err)
				if len(events) == 0 {
					b.Fatal("no events")
				}
			}
		})
	}
}

func TestLiveHTTP2WindowReduction(t *testing.T) {
	steps := append([]sessionStep{}, h2TestFixtures(t)[:4]...)
	settings := make([]byte, 6)
	binary.BigEndian.PutUint16(settings, 4)
	binary.BigEndian.PutUint32(settings[2:], 2)
	steps = append(steps, sessionStep{1, h2TestFrame(4, 0, 0, settings)}, sessionStep{0, h2TestFrame(4, 1, 0, nil)}, sessionStep{0, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":method", "POST", ":scheme", "http", ":path", "/"))}, sessionStep{0, h2TestFrame(0, 1, 1, []byte("too much"))})
	es, _ := sessionTestFlow(t, "http2", steps, 0, false)
	require.Equal(t, "malformed", es[len(es)-1].Status)
	require.Contains(t, es[len(es)-1].Error, "window")
}

func TestLiveHTTP2PeerSettingsBeforeCoalescedPreface(t *testing.T) {
	var b bytes.Buffer
	encoder := hpack.NewEncoder(&b)
	encoder.SetMaxDynamicTableSizeLimit(8192)
	encoder.SetMaxDynamicTableSize(8192)
	for _, h := range []hpack.HeaderField{{Name: ":method", Value: "GET"}, {Name: ":scheme", Value: "http"}, {Name: ":path", Value: "/"}} {
		require.NoError(t, encoder.WriteField(h))
	}
	first := append([]byte(binH2Preface), h2TestFrame(4, 0, 0, nil)...)
	first = append(first, h2TestFrame(1, 5, 1, b.Bytes())...)
	steps := []sessionStep{{1, h2TestFrame(4, 0, 0, []byte{0, 1, 0, 0, 32, 0})}, {0, first}, {1, h2TestFrame(1, 5, 1, h2TestHeaders(t, ":status", "200"))}}
	for _, chunk := range []int{0, 1, 7} {
		es, _ := sessionTestFlow(t, "http2", steps, chunk, false)
		assertSessionEvents(t, es, "http2", false)
		require.Len(t, es, 4)
	}
}

func TestLiveProtocolTCPReorderAndRetransmit(t *testing.T) {
	for _, p := range []string{"http2", "mysql"} {
		steps := h2TestFixtures(t)
		if p == "mysql" {
			steps = mysqlTestFixtures(false)
		}
		seq := [2]uint32{1, 1}
		packets := []tcpStep{{syn: true}, {syn: true, reverse: true}}
		for _, s := range steps {
			cut := len(s.wire) / 2
			if cut > 0 {
				tail := tcpStep{seq: seq[s.dir] + uint32(cut), reverse: s.dir == 1, data: string(s.wire[cut:])}
				packets = append(packets, tail, tail, tcpStep{seq: seq[s.dir], reverse: s.dir == 1, data: string(s.wire[:cut])})
			} else {
				packets = append(packets, tcpStep{seq: seq[s.dir], reverse: s.dir == 1, data: string(s.wire)})
			}
			seq[s.dir] += uint32(len(s.wire))
		}
		packets = append(packets, tcpStep{seq: seq[0], fin: true}, tcpStep{seq: seq[1], fin: true, reverse: true})
		for _, workers := range []int{1, 2, 4} {
			es, stats, err := binReplay(t, binTestPcap(t, packets, 3306, false, false), workers)
			require.NoError(t, err)
			assertSessionEvents(t, es, p, false)
			require.Zero(t, stats.BufferedBytes)
			want := 12
			if p == "mysql" {
				want = 14
			}
			require.Len(t, es, want)
		}
	}
}

func TestLiveProtocolMissingHandshakeAndPendingResponse(t *testing.T) {
	for _, p := range []string{"http2", "mysql"} {
		steps := h2TestFixtures(t)
		start, end := 4, 5
		if p == "mysql" {
			steps = mysqlTestFixtures(false)
			start, end = 6, 7
		}
		es, _ := sessionTestFlow(t, p, steps[start:end], 0, false)
		for _, e := range es {
			require.NotEqual(t, "decoded", e.Status)
			require.Nil(t, e.Fields)
		}
		es, _ = sessionTestFlow(t, p, steps[:end], 0, false)
		require.Equal(t, "incomplete", es[len(es)-1].Status)
	}
}

func TestLiveMySQLTrackedTerminatorAndEmptyAuth(t *testing.T) {
	steps := mysqlTestFixtures(true, true)
	steps[3].wire = mysqlTestPacket(3, nil)
	es, _ := sessionTestFlow(t, "mysql", steps, 1, false)
	assertSessionEvents(t, es, "mysql", false)
	require.Equal(t, "MySQLTextResultSetDeprecatedTrackFields", es[7].Entry)
	// Two independent results in the same TCP chunk retain separate transaction snapshots.
	require.Equal(t, es[7].Session["Transaction ID"], es[8].Session["Transaction ID"])
}
